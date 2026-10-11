package filemanager

import (
	"context"
	"errors"
	"path"
	"sort"

	"media-tool/internal/adapters/clouddrive2"
	"media-tool/internal/modules/storage"
)

type Cloud struct {
	Client *clouddrive2.Adapter
	Root   string
}

func (c *Cloud) Close() error { return nil }
func (c *Cloud) full(p string) (string, error) {
	relative, err := Relative(p)
	if err != nil {
		return "", err
	}
	return path.Join(c.Root, relative), nil
}
func (c *Cloud) convert(f storage.File, relative string) File {
	return File{ID: f.ID, Name: f.Name, Path: relative, Size: f.Size, Directory: f.IsDirectory}
}
func (c *Cloud) Find(ctx context.Context, p string) (File, error) {
	full, err := c.full(p)
	if err != nil {
		return File{}, err
	}
	f, err := c.Client.Find(ctx, full)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return File{}, ErrMissing
		}
		return File{}, ErrUnavailable
	}
	return c.convert(f, p), nil
}
func (c *Cloud) List(ctx context.Context, p string) ([]File, error) {
	full, err := c.full(p)
	if err != nil {
		return nil, err
	}
	files, err := c.Client.List(ctx, full)
	if err != nil {
		if errors.Is(err, storage.ErrLimitExceeded) {
			return nil, ErrLimit
		}
		return nil, ErrUnavailable
	}
	if len(files) > 5000 {
		return nil, ErrLimit
	}
	out := []File{}
	for _, f := range files {
		if Name(f.Name) != nil || path.Clean(f.Path) != path.Join(full, f.Name) {
			return nil, ErrUnavailable
		}
		out = append(out, c.convert(f, path.Join(p, f.Name)))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Directory != out[j].Directory {
			return out[i].Directory
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}
func (c *Cloud) Mkdir(ctx context.Context, p string) error {
	full, err := c.full(p)
	if err != nil {
		return err
	}
	if Name(path.Base(full)) != nil {
		return ErrInvalid
	}
	return c.Client.MakeDirectory(ctx, path.Dir(full), path.Base(full))
}
func (c *Cloud) Relocate(ctx context.Context, source, target string) error {
	from, err := c.full(source)
	if err != nil {
		return err
	}
	to, err := c.full(target)
	if err != nil {
		return err
	}
	if path.Dir(from) == path.Dir(to) {
		return c.Client.RenamePath(ctx, from, path.Base(to))
	}
	if path.Base(from) != path.Base(to) {
		return ErrInvalid
	}
	return c.Client.MovePath(ctx, from, path.Dir(to))
}
