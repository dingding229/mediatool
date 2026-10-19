package v2

import (
	"bytes"
	"context"
	"fmt"
	"io"
	deferutil "media-tool/internal/platform/cleanup"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const stateDataMax int64 = 64 << 20

func directory(parent *os.Root, name string) (*os.Root, error) {
	st, e := parent.Lstat(name)
	if e != nil {
		return nil, e
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return nil, ErrInvalid
	}
	r, e := parent.OpenRoot(name)
	if e != nil {
		return nil, e
	}
	f, e := r.Open(".")
	if e != nil {
		closeState(r)
		return nil, e
	}
	actual, e := f.Stat()
	closeState(f)
	if e != nil || !os.SameFile(st, actual) {
		closeState(r)
		return nil, ErrInvalid
	}
	return r, nil
}
func (s Store) stateRoot(id string) (*os.Root, error) {
	if !validUUID(id) {
		return nil, ErrInvalid
	}
	if _, e := s.DataPath(id); e != nil {
		return nil, e
	}
	root, e := os.OpenRoot(s.Root)
	if e != nil {
		return nil, e
	}
	defer deferutil.Close(root, "root descriptor only")
	return directory(root, id)
}
func syncRoot(r *os.Root) error {
	f, e := r.Open(".")
	if e != nil {
		return e
	}
	defer deferutil.Close(f, "directory descriptor only")
	return f.Sync()
}
func identity(r *os.Root, name string) (string, error) {
	st, e := r.Lstat(name)
	if e != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return "", ErrInvalid
	}
	v, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		return "", ErrUnavailable
	}
	return fmt.Sprintf("%d:%d", v.Dev, v.Ino), nil
}
func executable(name string, b []byte) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".go", ".py", ".sh", ".js", ".mjs", ".wasm", ".so", ".dll", ".exe", ".dylib", ".zip", ".tar", ".gz", ".tgz", ".jar", ".class", ".a", ".o", ".ps1", ".bat", ".cmd", ".rb", ".pl", ".lua", ".html", ".htm", ".svg", ".php", ".ts", ".tsx", ".jsx", ".vue", ".c", ".h", ".cpp", ".rs", ".java", ".swift", ".sql", ".7z", ".rar", ".bz2", ".xz", ".zst":
		return true
	}
	if bytes.HasPrefix(b, []byte("#!")) || bytes.HasPrefix(b, []byte("\x7fELF")) || bytes.HasPrefix(b, []byte("MZ")) {
		return true
	}
	for _, magic := range [][]byte{{0xfe, 0xed, 0xfa, 0xce}, {0xce, 0xfa, 0xed, 0xfe}, {0xfe, 0xed, 0xfa, 0xcf}, {0xcf, 0xfa, 0xed, 0xfe}, {0xca, 0xfe, 0xba, 0xbe}} {
		if bytes.HasPrefix(b, magic) {
			return true
		}
	}
	return false
}

type stateSize struct {
	Bytes int64
	Files int
}

func copyState(ctx context.Context, src, dst *os.Root, depth int, total *stateSize) error {
	if depth > 16 {
		return ErrInvalid
	}
	d, e := src.Open(".")
	if e != nil {
		return e
	}
	defer deferutil.Close(d, "directory descriptor only")
	for {
		entries, e := d.ReadDir(100)
		if e != nil && e != io.EOF {
			return e
		}
		for _, entry := range entries {
			if e = ctx.Err(); e != nil {
				return e
			}
			total.Files++
			if total.Files > 4096 {
				return ErrInvalid
			}
			name := entry.Name()
			st, e := src.Lstat(name)
			if e != nil {
				return e
			}
			if st.Mode()&os.ModeSymlink != 0 {
				return ErrInvalid
			}
			if st.IsDir() {
				if dst != nil {
					if e = dst.Mkdir(name, 0700); e != nil {
						return e
					}
				}
				a, e := directory(src, name)
				if e != nil {
					return e
				}
				var b *os.Root
				if dst != nil {
					b, e = directory(dst, name)
				}
				if e != nil {
					closeState(a)
					return e
				}
				e = copyState(ctx, a, b, depth+1, total)
				closeState(a)
				if b != nil {
					closeState(b)
				}
				if e != nil {
					return e
				}
				continue
			}
			stat, ok := st.Sys().(*syscall.Stat_t)
			if !ok || !st.Mode().IsRegular() || stat.Nlink != 1 || st.Mode()&0111 != 0 || st.Size() > 16<<20 {
				return ErrInvalid
			}
			in, e := src.Open(name)
			if e != nil {
				return e
			}
			actual, e := in.Stat()
			if e != nil || !os.SameFile(st, actual) || actual.Sys().(*syscall.Stat_t).Nlink != 1 {
				closeState(in)
				return ErrInvalid
			}
			buf, e := io.ReadAll(io.LimitReader(in, (16<<20)+1))
			after, statErr := in.Stat()
			closeState(in)
			if statErr != nil || !os.SameFile(actual, after) || after.Sys().(*syscall.Stat_t).Nlink != 1 || after.Size() != actual.Size() || !after.ModTime().Equal(actual.ModTime()) {
				return ErrConflict
			}
			if e != nil {
				return e
			}
			total.Bytes += int64(len(buf))
			if len(buf) > 16<<20 || total.Bytes > stateDataMax || executable(name, buf) {
				return ErrInvalid
			}
			if dst == nil {
				continue
			}
			out, e := dst.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if e != nil {
				return e
			}
			_, e = out.Write(buf)
			if e == nil {
				e = out.Sync()
			}
			closeErr := out.Close()
			if e != nil {
				return e
			}
			if closeErr != nil {
				return closeErr
			}
		}
		if e == io.EOF {
			break
		}
	}
	if dst == nil {
		return nil
	}
	return syncRoot(dst)
}
func exchangeState(r *os.Root, a, b string) error {
	f, e := r.Open(".")
	if e != nil {
		return e
	}
	defer deferutil.Close(f, "directory descriptor only")
	if e = exchangeDirs(int(f.Fd()), a, b); e != nil {
		return e
	}
	return syncRoot(r)
}

func closeState(v io.Closer) {
	deferutil.Close(v, "private filesystem descriptor teardown; primary errors handled by caller")
}
