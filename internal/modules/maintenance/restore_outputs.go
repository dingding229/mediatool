package maintenance

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"syscall"

	deferutil "media-tool/internal/platform/cleanup"
)

const (
	outputFilled   = "filled"
	outputMatch    = "exists_match"
	outputMismatch = "exists_mismatch"
	outputFailed   = "failed"
)

type RestoreFileResult struct {
	Path      string `json:"path"`
	Status    string `json:"status"`
	Published bool   `json:"published,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

type restoreOutputIO struct {
	write func(*os.File, []byte) (int, error)
	sync  func(*os.File) error
}

func (e Engine) syncOutput(f *os.File) error {
	if e.outputIO != nil && e.outputIO.sync != nil {
		return e.outputIO.sync(f)
	}
	return f.Sync()
}

func (e Engine) syncOutputDir(dir *os.Root) error {
	fd, err := dir.Open(".")
	if err != nil {
		return err
	}
	defer deferutil.Close(fd, "read-only restore directory descriptor")
	return e.syncOutput(fd)
}

func openOutputArchive(archive string, proofs []FileProof) (*os.File, map[string]FileProof, error) {
	expected := make(map[string]FileProof, len(proofs))
	for _, proof := range proofs {
		if !safeRelative(proof.Path) || !Digest.MatchString(proof.Digest) || proof.Bytes < 0 || proof.Bytes > 64<<20 {
			return nil, nil, ErrUnsafe
		}
		if _, ok := expected[proof.Path]; ok {
			return nil, nil, ErrUnsafe
		}
		expected[proof.Path] = proof
	}
	f, err := os.OpenFile(archive, os.O_RDONLY|noFollowFlag(), 0)
	return f, expected, err
}

func readOutputArchive(ctx context.Context, file *os.File, expected map[string]FileProof, visit func(FileProof, []byte)) error {
	seen := make(map[string]bool, len(expected))
	tr := tar.NewReader(file)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		p, ok := expected[h.Name]
		if !ok || seen[h.Name] || h.Typeflag != tar.TypeReg || h.Size != p.Bytes {
			return ErrUnsafe
		}
		seen[h.Name] = true
		raw, err := io.ReadAll(io.LimitReader(tr, p.Bytes+1))
		if err != nil || int64(len(raw)) != p.Bytes || hash(raw) != p.Digest {
			return ErrUnsafe
		}
		if visit != nil {
			visit(p, raw)
		}
	}
	if len(seen) != len(expected) {
		return ErrUnsafe
	}
	return nil
}

func validateOutputArchive(ctx context.Context, archive string, proofs []FileProof) error {
	f, expected, err := openOutputArchive(archive, proofs)
	if err != nil {
		return err
	}
	defer deferutil.Close(f, "read-only output archive")
	return readOutputArchive(ctx, f, expected, nil)
}

func (e Engine) restoreOutputs(ctx context.Context, archive string, proofs []FileProof) ([]RestoreFileResult, error) {
	results := make([]RestoreFileResult, 0, len(proofs))
	f, expected, err := openOutputArchive(archive, proofs)
	if err != nil {
		return results, err
	}
	defer deferutil.Close(f, "read-only output archive")
	// Authenticate the entire archive before publishing any output. Recheck each
	// member on the write pass; the archive descriptor never changes underneath us.
	if err = readOutputArchive(ctx, f, expected, nil); err != nil {
		return results, err
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return results, err
	}
	root, err := os.OpenRoot(e.OutputRoot)
	if err != nil {
		return results, err
	}
	defer deferutil.Close(root, "restore output directory")
	uid, gid := -1, -1
	if os.Geteuid() == 0 {
		info, err := root.Stat(".")
		if err != nil {
			return results, err
		}
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			uid, gid = restoreUID(int(stat.Uid)), restoreGID(int(stat.Gid))
		}
	}
	failed := 0
	err = readOutputArchive(ctx, f, expected, func(p FileProof, raw []byte) {
		result := e.restoreOutput(root, p, raw, uid, gid)
		results = append(results, result)
		if result.Status == outputFailed {
			failed++
		}
	})
	if failed > 0 {
		err = errors.Join(err, fmt.Errorf("%d 个输出补齐失败，请查看逐文件回执", failed))
	}
	return results, err
}

// Open each parent under a pinned directory handle. Reject symlinks even when
// they point inside the root, and chown only directories this operation created.
func (e Engine) restoreOutputParent(root *os.Root, parent string, uid, gid int) (*os.Root, error) {
	dir, err := root.OpenRoot(".")
	if err != nil || parent == "." {
		return dir, err
	}
	for _, name := range strings.Split(parent, "/") {
		next, err := e.restoreOutputSubdir(dir, name, uid, gid)
		deferutil.Close(dir, "previous restore directory descriptor")
		if err != nil {
			return nil, err
		}
		dir = next
	}
	return dir, nil
}

func (e Engine) restoreOutputSubdir(dir *os.Root, name string, uid, gid int) (*os.Root, error) {
	info, err := dir.Lstat(name)
	created := false
	if errors.Is(err, os.ErrNotExist) {
		err = dir.Mkdir(name, 0750)
		created = err == nil
		if err == nil || errors.Is(err, os.ErrExist) {
			info, err = dir.Lstat(name)
		}
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, ErrUnsafe
	}
	next, err := dir.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	opened, err := next.Stat(".")
	if err == nil && !os.SameFile(info, opened) {
		err = ErrUnsafe
	}
	if err == nil && created {
		if uid >= 0 {
			var fd *os.File
			fd, err = next.Open(".")
			if err == nil {
				err = errors.Join(fd.Chown(uid, gid), fd.Close())
			}
		}
		if err == nil {
			err = e.syncOutputDir(dir)
		}
	}
	if err != nil {
		deferutil.Close(next, "rejected restore directory descriptor")
		return nil, err
	}
	return next, nil
}

func existingOutput(dir *os.Root, name string, p FileProof) (string, error) {
	info, err := dir.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", ErrUnsafe
	}
	if info.Size() != p.Bytes {
		return outputMismatch, nil
	}
	f, err := dir.OpenFile(name, os.O_RDONLY|noFollowFlag()|syscall.O_NONBLOCK, 0)
	if err != nil {
		return "", err
	}
	defer deferutil.Close(f, "read-only comparison with existing output")
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return "", ErrUnsafe
	}
	return outputContentStatus(f, p)
}

func outputContentStatus(f *os.File, p FileProof) (string, error) {
	h := shaHasher()
	n, err := io.Copy(h, io.LimitReader(f, p.Bytes+1))
	if err != nil {
		return "", err
	}
	if n != p.Bytes || fmt.Sprintf("%x", h.Sum(nil)) != p.Digest {
		return outputMismatch, nil
	}
	return outputMatch, nil
}

func (e Engine) restoreOutput(root *os.Root, p FileProof, raw []byte, uid, gid int) (r RestoreFileResult) {
	r.Path = p.Path
	dir, err := e.restoreOutputParent(root, path.Dir(p.Path), uid, gid)
	if err == nil {
		defer deferutil.Close(dir, "restore output parent descriptor")
		name := path.Base(p.Path)
		r.Status, err = existingOutput(dir, name, p)
		if err == nil && r.Status == "" {
			var raced bool
			r.Published, raced, err = e.publishOutput(dir, name, p, raw, uid, gid)
			if err == nil && raced {
				r.Status, err = existingOutput(dir, name, p)
				if err == nil && r.Status == "" {
					err = ErrConflict // The racing writer changed the name again.
				}
			} else if err == nil {
				r.Status = outputFilled
			}
		}
	}
	if err != nil {
		r.Status, r.Reason = outputFailed, err.Error()
	}
	return r
}

func (e Engine) publishOutput(dir *os.Root, name string, p FileProof, raw []byte, uid, gid int) (published, raced bool, result error) {
	nameTemp := ".restore-" + newID() + ".tmp"
	temp, err := dir.OpenFile(nameTemp, os.O_RDWR|os.O_CREATE|os.O_EXCL|noFollowFlag(), 0600)
	if err != nil {
		return false, false, err
	}
	open, linkedTemp := true, true
	defer func() {
		if open {
			result = errors.Join(result, temp.Close())
		}
		if linkedTemp {
			result = errors.Join(result, dir.Remove(nameTemp))
		}
	}()
	write := (*os.File).Write
	if e.outputIO != nil && e.outputIO.write != nil {
		write = e.outputIO.write
	}
	if n, err := write(temp, raw); err != nil || n != len(raw) {
		if err == nil {
			err = io.ErrShortWrite
		}
		return false, false, err
	}
	if _, err = temp.Seek(0, io.SeekStart); err != nil {
		return false, false, err
	}
	if status, err := outputContentStatus(temp, p); err != nil || status != outputMatch {
		return false, false, errors.Join(ErrUnsafe, err)
	}
	if uid >= 0 {
		if err = temp.Chown(uid, gid); err != nil {
			return false, false, err
		}
	}
	if err = e.syncOutput(temp); err != nil {
		return false, false, err
	}
	open = false
	if err = temp.Close(); err != nil {
		return false, false, err
	}
	// link is atomic and never replaces the target; both names are in the
	// same directory. Never unlink the final name, even if directory sync fails.
	if err = dir.Link(nameTemp, name); err != nil {
		if errors.Is(err, os.ErrExist) {
			return false, true, nil
		}
		return false, false, err
	}
	if err = dir.Remove(nameTemp); err != nil {
		return true, false, err
	}
	linkedTemp = false
	return true, false, e.syncOutputDir(dir)
}
