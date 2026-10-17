package artifacts

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	deferutil "media-tool/internal/platform/cleanup"
	"os"
	"path"
	"strings"
)

func Write(rootDir, relative string, content []byte) error {
	if relative == "" || strings.HasPrefix(relative, "/") || path.Clean(relative) != relative || strings.HasPrefix(relative, "../") || strings.ContainsAny(relative, "\\\x00") {
		return errors.New("unsafe artifact path")
	}
	root, err := os.OpenRoot(rootDir)
	if err != nil {
		return err
	}
	defer deferutil.Close(root, "Read/descriptor teardown only; primary operations handle their own I/O result")
	if err = root.MkdirAll(path.Dir(relative), 0750); err != nil {
		return err
	}
	nonce := make([]byte, 12)
	if _, err = rand.Read(nonce); err != nil {
		return err
	}
	temp := path.Join(path.Dir(relative), ".artifact-"+hex.EncodeToString(nonce))
	file, err := root.OpenFile(temp, os.O_EXCL|os.O_CREATE|os.O_WRONLY, 0640)
	if err != nil {
		return err
	}
	defer deferutil.Do(func() error { return root.Remove(temp) }, "Temporary cleanup or final unlock only; resource close also releases locks and the primary result is authoritative")
	_, err = file.Write(content)
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = root.Rename(temp, relative); err != nil {
		return err
	}
	directory, err := root.Open(path.Dir(relative))
	if err != nil {
		return err
	}
	defer deferutil.Close(directory, "Read/descriptor teardown only; primary operations handle their own I/O result")
	return directory.Sync()
}
func Equal(rootDir, relative string, content []byte) (bool, error) {
	root, err := os.OpenRoot(rootDir)
	if err != nil {
		return false, err
	}
	defer deferutil.Close(root, "Read/descriptor teardown only; primary operations handle their own I/O result")
	file, err := root.Open(relative)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer deferutil.Close(file, "Read/descriptor teardown only; primary operations handle their own I/O result")
	raw, err := io.ReadAll(io.LimitReader(file, int64(len(content))+1))
	return string(raw) == string(content), err
}

// Create publishes a missing artifact atomically without replacing an existing
// file, even if it appeared after the approved inspection. Same-directory hard
// linking fails closed on filesystems without support.
func Create(rootDir, relative string, content []byte) error {
	if relative == "" || strings.HasPrefix(relative, "/") || path.Clean(relative) != relative || strings.HasPrefix(relative, "../") || strings.ContainsAny(relative, "\\\x00") {
		return errors.New("unsafe artifact path")
	}
	root, err := os.OpenRoot(rootDir)
	if err != nil {
		return err
	}
	defer deferutil.Close(root, "Read/descriptor teardown only; primary operations handle their own I/O result")
	if err = root.MkdirAll(path.Dir(relative), 0750); err != nil {
		return err
	}
	nonce := make([]byte, 12)
	if _, err = rand.Read(nonce); err != nil {
		return err
	}
	temp := path.Join(path.Dir(relative), ".restore-"+hex.EncodeToString(nonce))
	f, err := root.OpenFile(temp, os.O_EXCL|os.O_CREATE|os.O_WRONLY, 0640)
	if err != nil {
		return err
	}
	defer deferutil.Do(func() error { return root.Remove(temp) }, "Temporary cleanup or final unlock only; resource close also releases locks and the primary result is authoritative")
	_, err = f.Write(content)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = root.Link(temp, relative); err != nil {
		return err
	}
	directory, err := root.Open(path.Dir(relative))
	if err != nil {
		return err
	}
	defer deferutil.Close(directory, "Read/descriptor teardown only; primary operations handle their own I/O result")
	return directory.Sync()
}
