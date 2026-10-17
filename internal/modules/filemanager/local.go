package filemanager

import (
	"context"
	"errors"
	"fmt"
	"io"
	deferutil "media-tool/internal/platform/cleanup"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

type Local struct{ Root *os.Root }

func OpenLocal(root string) (*Local, error) {
	st, err := os.Lstat(root)
	if err != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return nil, ErrProtected
	}
	real, err := filepath.EvalSymlinks(root)
	if err != nil || !safeLocalRoot(real) {
		return nil, ErrProtected
	}
	r, err := os.OpenRoot(root)
	if err != nil {
		return nil, ErrUnavailable
	}
	return &Local{r}, nil
}
func (l *Local) Close() error { return l.Root.Close() }
func (l *Local) safe(raw string) error {
	clean, err := Relative(raw)
	if err != nil {
		return err
	}
	if privateLocalPath(clean) {
		return ErrProtected
	}
	if clean == "." {
		return nil
	}
	cursor := "."
	for _, part := range strings.Split(clean, "/") {
		cursor = path.Join(cursor, part)
		info, err := l.Root.Lstat(cursor)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return ErrMissing
			}
			return ErrUnavailable
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return ErrProtected
		}
	}
	return nil
}
func (l *Local) Find(_ context.Context, p string) (File, error) {
	if err := l.safe(p); err != nil {
		return File{}, err
	}
	fi, err := l.Root.Lstat(p)
	if err != nil {
		return File{}, ErrMissing
	}
	id := ""
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		id = fmt.Sprintf("%d:%d", st.Dev, st.Ino)
	}
	return File{ID: id, Name: fi.Name(), Path: p, Size: fi.Size(), Directory: fi.IsDir(), Modified: fi.ModTime().UnixNano(), Unsupported: !fi.IsDir() && !fi.Mode().IsRegular()}, nil
}
func (l *Local) List(ctx context.Context, p string) ([]File, error) {
	if err := l.safe(p); err != nil {
		return nil, err
	}
	folder, err := l.Root.Open(p)
	if err != nil {
		return nil, ErrMissing
	}
	defer deferutil.Close(folder, "Read/descriptor teardown only; primary operations handle their own I/O result")
	entries, err := folder.ReadDir(5001)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, ErrUnavailable
	}
	if len(entries) > 5000 {
		return nil, ErrLimit
	}
	out := []File{}
	for _, entry := range entries {
		if privateLocalPath(path.Join(p, entry.Name())) {
			continue
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		f, err := l.Find(ctx, path.Join(p, entry.Name()))
		if errors.Is(err, ErrProtected) {
			f = File{Name: entry.Name(), Path: path.Join(p, entry.Name()), Unsupported: true}
		} else if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Directory != out[j].Directory {
			return out[i].Directory
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}
func (l *Local) Mkdir(_ context.Context, p string) error {
	if privateLocalPath(p) {
		return ErrProtected
	}
	if err := l.safe(path.Dir(p)); err != nil {
		return err
	}
	if Name(path.Base(p)) != nil {
		return ErrInvalid
	}
	if err := l.Root.Mkdir(p, 0o750); err != nil {
		if errors.Is(err, os.ErrExist) {
			return ErrConflict
		}
		return ErrUnavailable
	}
	return nil
}
func (l *Local) Relocate(ctx context.Context, source, target string) error {
	if privateLocalPath(target) {
		return ErrProtected
	}
	if err := l.safe(source); err != nil {
		return err
	}
	if err := l.safe(path.Dir(target)); err != nil {
		return err
	}
	if Name(path.Base(target)) != nil {
		return ErrInvalid
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	from, err := l.Root.Open(path.Dir(source))
	if err != nil {
		return ErrUnavailable
	}
	defer deferutil.Close(from, "Read/descriptor teardown only; primary operations handle their own I/O result")
	to, err := l.Root.Open(path.Dir(target))
	if err != nil {
		return ErrUnavailable
	}
	defer deferutil.Close(to, "Read/descriptor teardown only; primary operations handle their own I/O result")
	if err = renameNoReplace(from, path.Base(source), to, path.Base(target)); err != nil {
		if errors.Is(err, os.ErrExist) {
			return ErrConflict
		}
		return ErrUnavailable
	}
	return nil
}
