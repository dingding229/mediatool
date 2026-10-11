package strm

import (
	"media-tool/internal/modules/storage"
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
