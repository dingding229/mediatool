package strm

import (
	"context"
	"errors"
	"io"
	"os"
	"path"
	"strings"
	"unicode/utf8"
)

var ErrUnsafeBody = errors.New("invalid owned STRM body")
var ErrReadUnavailable = errors.New("owned STRM unavailable")

// ReadOwned is shared by background preprocessing and native playback. It never
// creates/repairs files; Root confines traversal, Lstat/SameFile reject swaps.
func ReadOwned(ctx context.Context, dir, relative string) (body []byte, err error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if len(relative) > 2048 || relative == "" || relative == "." || relative == ".." || path.IsAbs(relative) || path.Clean(relative) != relative || strings.HasPrefix(relative, "../") || strings.ContainsAny(relative, "\\\x00") || !strings.HasSuffix(strings.ToLower(relative), ".strm") {
		return nil, ErrUnsafeBody
	}
	sanitize := func(e error) error {
		if e == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(e, os.ErrNotExist) {
			return os.ErrNotExist
		}
		return ErrReadUnavailable
	}
	root, e := os.OpenRoot(dir)
	if e != nil {
		return nil, sanitize(e)
	}
	defer func() {
		if e := root.Close(); err == nil && e != nil {
			err = sanitize(e)
		}
	}()
	info, e := root.Lstat(relative)
	if e != nil {
		return nil, sanitize(e)
	}
	if !info.Mode().IsRegular() {
		return nil, ErrUnsafeBody
	}
	f, e := root.OpenFile(relative, os.O_RDONLY|readNonblock, 0)
	if e != nil {
		return nil, sanitize(e)
	}
	defer func() {
		if e := f.Close(); err == nil && e != nil {
			err = sanitize(e)
		}
	}()
	actual, e := f.Stat()
	if e != nil {
		return nil, sanitize(e)
	}
	if !actual.Mode().IsRegular() || !os.SameFile(info, actual) {
		return nil, ErrUnsafeBody
	}
	body, e = io.ReadAll(io.LimitReader(f, 8193))
	if e != nil {
		return nil, sanitize(e)
	}
	if len(body) > 8192 || !utf8.Valid(body) {
		return nil, ErrUnsafeBody
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return body, nil
}
