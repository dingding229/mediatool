package artifacts

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
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
	defer root.Close()
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
	defer root.Remove(temp)
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
	defer directory.Close()
	return directory.Sync()
}
func Equal(rootDir, relative string, content []byte) (bool, error) {
	root, err := os.OpenRoot(rootDir)
	if err != nil {
		return false, err
	}
	defer root.Close()
	file, err := root.Open(relative)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, int64(len(content))+1))
	return string(raw) == string(content), err
}
