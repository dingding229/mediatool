package strm

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"media-tool/internal/modules/storage"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"
)

var pickCodePattern = regexp.MustCompile(`^[A-Za-z0-9]{1,64}$`)

const Endpoint = "/api/v1/cloudStorage/strm302"

type Builder struct {
	BaseURL, APIKey string
	AllowedRoots    []string
}

func (b Builder) Validate(ref storage.FileRef) error {
	if ref.Path == "" || !strings.HasPrefix(ref.Path, "/") || strings.Contains(ref.Path, "\\") {
		return errors.New("path must be an absolute CloudDrive2 path")
	}
	for _, r := range ref.Path {
		if r < 32 || r == 127 {
			return errors.New("control characters are not allowed")
		}
	}
	for _, segment := range strings.Split(ref.Path, "/") {
		if segment == ".." {
			return errors.New("path traversal is not allowed")
		}
	}
	cleaned := path.Clean(ref.Path)
	if cleaned != ref.Path {
		return errors.New("path must be canonical without duplicate slashes or dot segments")
	}
	allowed := false
	for _, root := range b.AllowedRoots {
		root = path.Clean(root)
		if root == "/" || cleaned == root || strings.HasPrefix(cleaned, root+"/") {
			allowed = true
			break
		}
	}
	if !allowed {
		return errors.New("path is outside allowed storage roots")
	}
	if ref.PickCode != "" && !pickCodePattern.MatchString(ref.PickCode) {
		return errors.New("pickCode must be 1-64 alphanumeric characters")
	}
	return nil
}
func (b Builder) Build(ref storage.FileRef) (string, error) {
	if err := b.Validate(ref); err != nil {
		return "", err
	}
	u, err := url.Parse(b.BaseURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("invalid public origin")
	}
	if b.APIKey == "" {
		return "", errors.New("STRM key is missing")
	}
	u.Path = Endpoint
	u.RawPath = ""
	q := url.Values{"apiKey": {b.APIKey}, "path": {ref.Path}}
	if ref.PickCode != "" {
		q.Set("pickCode", ref.PickCode)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// WriteAtomic only writes .strm files below an existing trusted root. os.Root
// confines symlink traversal; rename replaces a complete file atomically.
func WriteAtomic(rootDir, relative, link string) error {
	if relative == "" || strings.HasPrefix(relative, "/") || strings.Contains(relative, "\\") || path.Clean(relative) != relative || strings.HasPrefix(relative, "../") || !strings.HasSuffix(strings.ToLower(relative), ".strm") {
		return errors.New("expected a safe relative .strm path")
	}
	if strings.ContainsAny(link, "\r\n") {
		return errors.New("STRM must contain one URL")
	}
	u, err := url.Parse(link)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return errors.New("STRM must contain an HTTP URL")
	}
	root, err := os.OpenRoot(rootDir)
	if err != nil {
		return err
	}
	defer root.Close()
	dir := path.Dir(relative)
	if err = root.MkdirAll(dir, 0750); err != nil {
		return err
	}
	nonce := make([]byte, 12)
	if _, err = rand.Read(nonce); err != nil {
		return err
	}
	tmp := path.Join(dir, ".strm-"+hex.EncodeToString(nonce)+".tmp")
	f, err := root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0640)
	if err != nil {
		return err
	}
	defer root.Remove(tmp)
	_, writeErr := fmt.Fprintln(f, link)
	if writeErr == nil {
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return root.Rename(tmp, relative)
}
