package filemanager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"media-tool/internal/modules/metadata"
	"media-tool/internal/modules/releasename"
	"path"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	ErrReadOnly    = errors.New("directory is read-only")
	ErrInvalid     = errors.New("invalid file operation")
	ErrChanged     = errors.New("file or configuration changed; preview again")
	ErrConflict    = errors.New("destination already exists or is reserved")
	ErrBusy        = errors.New("a scan, transfer or file operation is in progress")
	ErrProtected   = errors.New("managed outputs or seeding source are protected")
	ErrUnavailable = errors.New("file service unavailable")
	ErrLimit       = errors.New("file scope exceeds configured safety limit")
	ErrMissing     = errors.New("file not found")
)

const TrashDirectory = ".media-tool-trash"

type File struct {
	Capabilities *Capabilities `json:"capabilities,omitempty"`
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Path         string        `json:"path"`
	Size         int64         `json:"size"`
	Directory    bool          `json:"directory"`
	Modified     int64         `json:"modified,omitempty"`
	Unsupported  bool          `json:"unsupported,omitempty"`
}
type Backend interface {
	Find(context.Context, string) (File, error)
	List(context.Context, string) ([]File, error)
	Mkdir(context.Context, string) error
	Relocate(context.Context, string, string) error
	Close() error
}
type Input struct {
	ExpectedModified *int64 `json:"expectedModified,omitempty"`
	ExpectedFileID   string `json:"expectedFileId"`
	ExpectedSize     *int64 `json:"expectedSize"`
	ConnectionID     string `json:"connectionId"`
	Operation        string `json:"operation"`
	Source           string `json:"source"`
	Destination      string `json:"destination"`
	Name             string `json:"name"`
	Template         string `json:"template"`
	Title            string `json:"title"`
	Year             string `json:"year"`
	MediaType        string `json:"mediaType"`
	Category         string `json:"category"`
	Season           int    `json:"season"`
	Episode          int    `json:"episode"`
	Resolution       string `json:"resolution"`
	IncludeSidecars  bool   `json:"includeSidecars"`
}
type Item struct {
	Source File   `json:"source"`
	Target string `json:"target"`
}
type Step struct {
	Action string `json:"action"`
	Source File   `json:"source"`
	Target string `json:"target"`
}
type Plan struct {
	Media             metadata.Item `json:"media"`
	Category          string        `json:"category"`
	ID                string        `json:"id"`
	ConnectionID      string        `json:"connectionId"`
	Revision          int64         `json:"revision"`
	BackendKind       string        `json:"backendKind"`
	Root              string        `json:"root"`
	OriginFingerprint string        `json:"originFingerprint"`
	Operation         string        `json:"operation"`
	Items             []Item        `json:"items"`
	Steps             []Step        `json:"steps"`
	Digest            string        `json:"digest"`
	ExpiresAt         time.Time     `json:"expiresAt"`
}
type Result struct {
	ID             string `json:"id"`
	State          string `json:"state"`
	CompletedSteps int    `json:"completedSteps"`
	Message        string `json:"message"`
	Plan           Plan   `json:"plan"`
}
type Root struct {
	Capabilities    Capabilities `json:"capabilities"`
	ConnectionState string       `json:"connectionState"`
	ID              string       `json:"id"`
	Name            string       `json:"name"`
	Kind            string       `json:"kind"`
	Path            string       `json:"path"`
	Revision        int64        `json:"revision"`
	Origin          string       `json:"-"`
}

func Relative(raw string) (string, error) {
	if raw == "" {
		return ".", nil
	}
	if strings.Count(raw, "/") > 32 || len(raw) > 2048 || strings.HasPrefix(raw, "/") || strings.Contains(raw, "\\") || path.Clean(raw) != raw || raw == ".." || strings.HasPrefix(raw, "../") || !utf8.ValidString(raw) {
		return "", ErrInvalid
	}
	for _, r := range raw {
		if unicode.IsControl(r) {
			return "", ErrInvalid
		}
	}
	return raw, nil
}
func Name(raw string) error {
	if raw == "" || raw == "." || raw == ".." || len(raw) > 255 || strings.ContainsAny(raw, "/\\") || strings.TrimSpace(raw) != raw || !utf8.ValidString(raw) {
		return ErrInvalid
	}
	for _, r := range raw {
		if unicode.IsControl(r) {
			return ErrInvalid
		}
	}
	return nil
}
func Same(a, b File) bool {
	return !a.Unsupported && !b.Unsupported && a.ID != "" && a.ID == b.ID && a.Directory == b.Directory && (a.Directory || a.Size == b.Size) && (a.Modified == 0 || b.Modified == a.Modified)
}
func fingerprint(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

var tokenPattern = regexp.MustCompile(`\{([a-zA-Z_]+)\}`)

func FileName(in Input, source File) (string, error) {
	if in.Name != "" && in.Template != "" {
		return "", ErrInvalid
	}
	name := in.Name
	if in.Template != "" {
		if len(in.Template) > 512 {
			return "", ErrInvalid
		}
		extension := path.Ext(source.Name)
		original := strings.TrimSuffix(source.Name, extension)
		hint := releasename.Parse(source.Name)
		vars := map[string]string{"source": hint.Source, "codec": hint.Codec, "group": hint.Group, "original": original, "title": in.Title, "year": in.Year, "ext": extension, "resolution": in.Resolution}
		if in.Season < 0 || in.Season > 999 || in.Episode < 0 || in.Episode > 9999 {
			return "", ErrInvalid
		}
		vars["videoFormat"] = in.Resolution
		vars["season_episode"] = fmt.Sprintf("S%02dE%02d", in.Season, in.Episode)
		vars["season"] = pad(in.Season)
		vars["episode"] = pad(in.Episode)
		valid := true
		name = tokenPattern.ReplaceAllStringFunc(in.Template, func(match string) string {
			v, ok := vars[match[1:len(match)-1]]
			if !ok || v == "" {
				valid = false
			}
			return v
		})
		if !valid || strings.ContainsAny(name, "{}") {
			return "", ErrInvalid
		}
	}
	if name == "" {
		name = source.Name
	}
	if !source.Directory {
		extension := path.Ext(source.Name)
		mediaExtensions := map[string]bool{".mkv": true, ".mp4": true, ".m4v": true, ".rmvb": true, ".vob": true, ".iso": true, ".avi": true, ".ts": true, ".m2ts": true, ".mov": true, ".wmv": true, ".flv": true, ".webm": true, ".mpg": true, ".mpeg": true}
		if mediaExtensions[strings.ToLower(extension)] && !strings.EqualFold(path.Ext(name), extension) {
			if mediaExtensions[strings.ToLower(path.Ext(name))] {
				return "", ErrInvalid
			} // rename is not format conversion.
			name += extension
		} else if path.Ext(name) == "" {
			name += extension
		}
	}
	if err := Name(name); err != nil {
		return "", err
	}
	return name, nil
}
func pad(n int) string { return fmt.Sprintf("%02d", n) }
