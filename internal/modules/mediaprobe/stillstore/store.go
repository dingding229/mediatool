// Package stillstore publishes bounded, immutable JPEG files. No worker, HTTP
// route or background worker is activated here. Runtime producers must use Cache.
package stillstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"media-tool/internal/modules/mediaprobe/stillqueue"
)

const DefaultRoot = "/data/native-media/stills"
const MaxJPEGBytes = int(stillqueue.MaxImageBytes)
const MaxPixels = 16 << 20
const ioBudget = 10 * time.Second

var (
	ErrInvalid     = errors.New("invalid still image")
	ErrUnsafe      = errors.New("unsafe still storage entry")
	ErrNotFound    = errors.New("still image not found")
	ErrConflict    = errors.New("immutable still image conflict")
	ErrUnavailable = errors.New("still storage unavailable")
)

type Store struct {
	mu   sync.RWMutex // Only coordinates Close with active operations, not publications.
	root *os.File
	step func(string) error // Unexported fault/barrier hook; nil outside package tests.
}

func Open(root string) (*Store, error) {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root || filepath.Base(root) != "stills" || filepath.Base(filepath.Dir(root)) != "native-media" || len(root) > 4096 || strings.ContainsRune(root, 0) {
		return nil, ErrUnsafe
	}
	dir, err := openRoot(root)
	if err != nil {
		return nil, err
	}
	return &Store{root: dir}, nil
}
func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.root == nil {
		return nil
	}
	err := s.root.Close()
	s.root = nil
	if err != nil {
		return ErrUnavailable
	}
	return nil
}
func closeFile(f *os.File, err *error) {
	if e := f.Close(); e != nil {
		*err = errors.Join(*err, ErrUnavailable)
	}
}
func (s *Store) atStep(stage string) error {
	if s.step != nil && s.step(stage) != nil {
		return ErrUnavailable
	}
	return nil
}

func splitPath(path string) (string, string, error) {
	if len(path) != 60 || path[2] != '/' {
		return "", "", ErrUnsafe
	}
	leaf := path[3:]
	expected, err := stillqueue.ImagePath(leaf[:36], leaf[37:53]+strings.Repeat("0", 48))
	if err != nil || expected != path {
		return "", "", ErrUnsafe
	}
	return path[:2], leaf, nil
}
func inspectJPEG(ctx context.Context, path string, raw []byte) (stillqueue.Image, error) {
	if err := ctx.Err(); err != nil {
		return stillqueue.Image{}, err
	}
	if len(raw) == 0 || len(raw) > MaxJPEGBytes {
		return stillqueue.Image{}, ErrInvalid
	}
	config, err := jpeg.DecodeConfig(bytes.NewReader(raw))
	if err != nil || config.Width < 1 || config.Width > 1280 || config.Height < 1 || config.Height > 16384 || int64(config.Width)*int64(config.Height) > MaxPixels {
		return stillqueue.Image{}, ErrInvalid
	}
	decoded, err := jpeg.Decode(bytes.NewReader(raw))
	if err != nil || decoded.Bounds().Dx() != config.Width || decoded.Bounds().Dy() != config.Height {
		return stillqueue.Image{}, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return stillqueue.Image{}, err
	}
	digest := sha256.Sum256(raw)
	return stillqueue.Image{RelativePath: path, Digest: hex.EncodeToString(digest[:]), Width: config.Width, Height: config.Height, SizeBytes: int64(len(raw))}, nil
}
func readJPEG(ctx context.Context, dir *os.File, leaf, path string) (raw []byte, image stillqueue.Image, err error) {
	file, err := openLeaf(dir, leaf)
	if err != nil {
		return nil, image, err
	}
	defer closeFile(file, &err)
	raw, err = io.ReadAll(io.LimitReader(file, stillqueue.MaxImageBytes+1))
	if err != nil {
		return nil, image, ErrUnavailable
	}
	image, err = inspectJPEG(ctx, path, raw)
	return raw, image, err
}

// Read never interprets a URL/path supplied by a client: only the canonical
// UUID/fingerprint leaf shape is accepted. Access recording is added in B2.
func (s *Store) Read(parent context.Context, path string) (raw []byte, image stillqueue.Image, err error) {
	shard, leaf, err := splitPath(path)
	if err != nil {
		return nil, image, err
	}
	if s == nil {
		return nil, image, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(parent, ioBudget)
	defer cancel()
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.root == nil {
		return nil, image, ErrUnavailable
	}
	defer func() {
		if err != nil {
			raw = nil
			image = stillqueue.Image{}
		}
	}()
	if err = ctx.Err(); err != nil {
		return nil, image, err
	}
	dir, err := openShard(s.root, shard, false)
	if err != nil {
		return nil, image, err
	}
	defer closeFile(dir, &err)
	return readJPEG(ctx, dir, leaf, path)
}

// Publish returns a receipt ONLY after file sync, no-replace rename and directory
// sync. C must then fence the database reference; a stale receipt is an orphan.
func (s *Store) Publish(parent context.Context, asset, fingerprint string, raw []byte) (stillqueue.Image, error) {
	return s.publish(parent, asset, fingerprint, raw, nil)
}
func (s *Store) publish(parent context.Context, asset, fingerprint string, raw []byte, admit func(context.Context, stillqueue.Image) error) (image stillqueue.Image, err error) {
	path, err := stillqueue.ImagePath(asset, fingerprint)
	if err != nil {
		return image, ErrInvalid
	}
	if s == nil {
		return image, ErrUnavailable
	}
	if len(raw) > MaxJPEGBytes {
		return image, ErrInvalid
	}
	raw = bytes.Clone(raw)
	ctx, cancel := context.WithTimeout(parent, ioBudget)
	defer cancel()
	wanted, err := inspectJPEG(ctx, path, raw)
	if err != nil {
		return image, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.root == nil {
		return image, ErrUnavailable
	}
	defer func() {
		if err != nil {
			image = stillqueue.Image{}
		}
	}()
	lock, err := lockRoot(ctx, s.root)
	if err != nil {
		return image, err
	}
	defer closeFile(lock, &err)
	if admit != nil {
		if err = admit(ctx, wanted); err != nil {
			return image, err
		}
	}
	shard, leaf, err := splitPath(path)
	if err != nil {
		return image, err
	}
	dir, err := openShard(s.root, shard, true)
	if err != nil {
		return image, err
	}
	defer closeFile(dir, &err)
	existing, readErr := existingJPEG(ctx, dir, leaf, path, wanted)
	if !errors.Is(readErr, ErrNotFound) {
		return existing, readErr
	}
	file, tmp, err := temporary(dir)
	if err != nil {
		return image, err
	}
	defer func() {
		if file != nil {
			closeFile(file, &err)
		}
		if tmp != "" {
			discard(dir, tmp, &err)
		}
	}()
	n, writeErr := file.Write(raw)
	if writeErr != nil || n != len(raw) {
		return image, ErrUnavailable
	}
	if file.Sync() != nil {
		return image, ErrUnavailable
	}
	if s.atStep("synced") != nil {
		return image, ErrUnavailable
	}
	closeErr := file.Close()
	file = nil
	if closeErr != nil {
		return image, ErrUnavailable
	}
	if err = ctx.Err(); err != nil {
		return image, err
	}
	err = renameExclusive(dir, tmp, leaf)
	if errors.Is(err, ErrConflict) {
		return existingJPEG(ctx, dir, leaf, path, wanted)
	}
	if err != nil {
		return image, err
	}
	tmp = ""
	if s.atStep("renamed") != nil || dir.Sync() != nil {
		return image, ErrUnavailable
	}
	if err = ctx.Err(); err != nil {
		return image, err
	}
	return wanted, nil
}
func existingJPEG(ctx context.Context, dir *os.File, leaf, path string, wanted stillqueue.Image) (stillqueue.Image, error) {
	_, existing, err := readJPEG(ctx, dir, leaf, path)
	if err != nil {
		return stillqueue.Image{}, err
	}
	if existing != wanted {
		return stillqueue.Image{}, ErrConflict
	}
	// Also completes directory durability if another writer crashed after rename.
	if dir.Sync() != nil {
		return stillqueue.Image{}, ErrUnavailable
	}
	return existing, nil
}
