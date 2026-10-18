package filemanager

import (
	"context"
	deferutil "media-tool/internal/platform/cleanup"
)

// Stat applies the same configured-root, private-directory and no-symlink rules
// as List. It never resolves a download URL or reads a file's contents.
func (e Engine) Stat(ctx context.Context, id, relative string) (File, error) {
	p, err := Relative(relative)
	if err != nil {
		return File{}, err
	}
	r, err := e.root(ctx, id)
	if err != nil {
		return File{}, err
	}
	b, err := e.backend(ctx, r)
	if err != nil {
		return File{}, err
	}
	defer deferutil.Close(b, "Read/descriptor teardown only")
	if r.Kind == "local" && e.privateBrowsePath(r, p) {
		return File{}, ErrProtected
	}
	c, err := e.capability(ctx, r, p, b, nil)
	if err != nil {
		return File{}, err
	}
	if !c.Read {
		return File{}, ErrProtected
	}
	return b.Find(ctx, p)
}
