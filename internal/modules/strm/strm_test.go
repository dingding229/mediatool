package strm

import (
	"media-tool/internal/modules/storage"
	"media-tool/internal/platform/mediaext"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func builder() Builder {
	return Builder{BaseURL: "https://media.example.com", APIKey: strings.Repeat("b", 64), AllowedRoots: []string{"/115/媒体"}}
}
func TestBuildEncodedStableURL(t *testing.T) {
	b := builder()
	ref := storage.FileRef{Path: "/115/媒体/幽灵公主 (1997)/电影 & 特别版#1.mkv", PickCode: "abc123"}
	raw, err := b.Build(ref)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if u.Path != Endpoint || u.Query().Get("path") != ref.Path || u.Query().Get("pickCode") != ref.PickCode || strings.Contains(raw, "幽灵") {
		t.Fatal(raw)
	}
}
func TestRejectTraversalAndRootSibling(t *testing.T) {
	for _, p := range []string{"/115/媒体/../private/a.mkv", "/115/媒体2/a.mkv", "/115/媒体//a.mkv", "/115/媒体/./a.mkv", "/115/媒体/a\n.mkv", "relative.mkv", "/115/媒体\\a.mkv"} {
		if err := builder().Validate(storage.FileRef{Path: p}); err == nil {
			t.Fatal("accepted", p)
		}
	}
}
func TestAtomicWrite(t *testing.T) {
	root := t.TempDir()
	link, _ := builder().Build(storage.FileRef{Path: "/115/媒体/a.mkv"})
	for i := 0; i < 2; i++ {
		if err := WriteAtomic(root, "Movies/a.strm", link); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(filepath.Join(root, "Movies/a.strm"))
	if err != nil || string(data) != link+"\n" {
		t.Fatal(err, string(data))
	}
	if err := WriteAtomic(root, "../escape.strm", link); err == nil {
		t.Fatal("escaped root")
	}
	if err := WriteAtomic(root, "a.strm", link+"\nsecond"); err == nil {
		t.Fatal("multiple lines accepted")
	}
	entries, _ := os.ReadDir(filepath.Join(root, "Movies"))
	if len(entries) != 1 {
		t.Fatal("temporary file leaked")
	}
}
func TestAtomicWriteRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skip(err)
	}
	if err := WriteAtomic(root, "escape/a.strm", "https://example.com/video"); err == nil {
		t.Fatal("followed external symlink")
	}
}

func TestEncodedTraversalIsDecodedOnceAndNeverBecomesAParentPath(t *testing.T) {
	b := Builder{BaseURL: "https://gateway.example", APIKey: "synthetic-only", AllowedRoots: []string{"/115"}}
	for _, raw := range []string{"path=%2F115%2F..%2Fsecret", "path=%2F115%2F%2e%2e%2Fsecret", "path=%2F115%2F.%2Fmovie.mkv", "path=%2F115%2F%5c..%5csecret", "path=%2F115%2Fmovie%00.mkv"} {
		q, e := url.ParseQuery(raw)
		if e != nil {
			continue
		}
		if _, e = b.Build(storage.FileRef{Path: q.Get("path")}); e == nil {
			t.Fatal("traversal accepted", raw)
		}
	}
	// Encoded percent text is a literal remote filename, not a second URL decode.
	literal := "/115/%2e%2e/movie.mkv"
	built, e := b.Build(storage.FileRef{Path: literal})
	if e != nil {
		t.Fatal(e)
	}
	u, e := url.Parse(built)
	if e != nil || u.Query().Get("path") != literal {
		t.Fatal("double decoding", built, e)
	}
}

func TestSTRMOnlyAcceptsSharedPlayableContainers(t *testing.T) {
	for _, ext := range mediaext.VideoExtensions() {
		if _, err := builder().Build(storage.FileRef{Path: "/115/媒体/Owned" + strings.ToUpper(ext)}); err != nil {
			t.Fatal(ext, err)
		}
	}
	for _, ext := range append(mediaext.SubtitleExtensions(), ".iso", ".ISO", ".nfo", ".strm", "") {
		if _, err := builder().Build(storage.FileRef{Path: "/115/媒体/Owned" + ext}); err == nil {
			t.Fatal("nonplayable STRM accepted", ext)
		}
	}
}
