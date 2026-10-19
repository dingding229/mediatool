package filemanager

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalBoundariesNoReplaceAndEmptyDirectory(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	ctx := context.Background()
	l, err := OpenLocal(root)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if files, err := l.List(ctx, "."); err != nil || len(files) != 0 {
		t.Fatal("empty directory not readable", err)
	}
	os.WriteFile(filepath.Join(root, "source.mkv"), []byte("owned"), 0o600)
	os.WriteFile(filepath.Join(root, "existing.mkv"), []byte("preserve"), 0o600)
	os.WriteFile(filepath.Join(outside, "private"), []byte("DO-NOT-READ"), 0o600)
	os.Symlink(outside, filepath.Join(root, "escape"))
	for _, p := range []string{"../private", "/etc/passwd", "escape/private", "escape"} {
		if _, err := l.Find(ctx, p); err == nil {
			t.Fatal("path escaped root", p)
		}
	}
	if err = l.Relocate(ctx, "source.mkv", "existing.mkv"); !errors.Is(err, ErrConflict) {
		t.Fatal("destination overwritten", err)
	}
	if data, _ := os.ReadFile(filepath.Join(root, "existing.mkv")); string(data) != "preserve" {
		t.Fatal("existing bytes changed")
	}
	if err = l.Relocate(ctx, "source.mkv", "中文电影 (1997).mkv"); err != nil {
		t.Fatal(err)
	}
	if _, err = l.Find(ctx, "source.mkv"); !errors.Is(err, ErrMissing) {
		t.Fatal("source not moved")
	}
	files, err := l.List(ctx, ".")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range files {
		if f.Name == "escape" {
			found = f.Unsupported
		}
	}
	if !found {
		t.Fatal("symlink policy missing")
	}
}
func TestFileNamesTemplatesAreSinglePassAndBounded(t *testing.T) {
	source := File{Name: "Owned.mkv"}
	for _, tc := range []struct {
		in   Input
		want string
	}{
		{Input{Name: "自定义"}, "自定义.mkv"},
		{Input{Name: "Movie.1080p"}, "Movie.1080p.mkv"},
		{Input{Template: "{title} ({year}) - {resolution}{ext}", Title: "幽灵公主", Year: "1997", Resolution: "1080p"}, "幽灵公主 (1997) - 1080p.mkv"},
		{Input{Template: "{original} - S{season}E{episode}{ext}", Season: 1, Episode: 3}, "Owned - S01E03.mkv"},
	} {
		got, err := FileName(tc.in, source)
		if err != nil || got != tc.want {
			t.Fatal(got, err)
		}
	}
	for _, in := range []Input{{Name: "unexpected.mp4"}, {Name: "../other"}, {Template: "{unknown}{ext}"}, {Template: "{title}", Title: "{year}"}, {Template: "{year}{ext}"}, {Name: strings.Repeat("a", 256)}, {Name: "bad\x00"}, {Name: "custom", Template: "{original}"}} {
		if _, err := FileName(in, source); err == nil {
			t.Fatal("invalid name/template accepted")
		}
	}
}
func TestOrganizePreviewCreatesNoFilesAndIncludesSidecars(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	l, _ := OpenLocal(root)
	defer l.Close()
	for _, name := range []string{"Owned.mkv", "Owned.zh-CN.srt", "Other.srt"} {
		os.WriteFile(filepath.Join(root, name), []byte("fixture"), 0o600)
	}
	p, err := BuildPlan(ctx, l, Root{ID: "owned", Kind: "local", Path: root, Revision: 1}, Input{Operation: "organize", Source: "Owned.mkv", MediaType: "movie", Title: "Owned", Year: "2026", Category: "动画", Template: "{title} ({year}){ext}", IncludeSidecars: true}, "owned-operation")
	if err != nil || len(p.Items) != 2 || p.Items[0].Target != "电影/动画/Owned (2026)/Owned (2026).mkv" {
		t.Fatal("preview invalid", p, err)
	}
	if _, err = os.Stat(filepath.Join(root, "电影")); !os.IsNotExist(err) {
		t.Fatal("preview changed filesystem")
	}
	if p.Digest == "" || len(p.Steps) != 5 {
		t.Fatal("directories/sidecar steps missing", p.Steps)
	}
	if _, err = BuildPlan(ctx, l, Root{Kind: "local"}, Input{Operation: "move", Source: ".", Destination: "folder"}, "id"); !errors.Is(err, ErrProtected) {
		t.Fatal("root move accepted")
	}
}

func TestDirectRenamePreviewKeepsSubtitleNamesTogether(t *testing.T) {
	ctx := context.Background()
	directory := t.TempDir()
	l, _ := OpenLocal(directory)
	defer l.Close()
	for _, name := range []string{"Owned.mkv", "Owned.zh.srt"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte("owned"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	p, err := BuildPlan(ctx, l, Root{Kind: "local", Path: directory}, Input{Operation: "rename", Source: "Owned.mkv", Name: "New.1080p", IncludeSidecars: true}, "owned")
	if err != nil || len(p.Items) != 2 || p.Items[0].Target != "New.1080p.mkv" || p.Items[1].Target != "New.1080p.zh.srt" {
		t.Fatal("rename detached subtitles", p, err)
	}
}

func TestLocalRejectsHardlinkEscapeAndSymlinkSubdirectories(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if e := os.WriteFile(filepath.Join(outside, "secret.mkv"), []byte("outside sentinel"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.MkdirAll(filepath.Join(root, "Show", "Season 01"), 0700); e != nil {
		t.Fatal(e)
	}
	link := "Show/Season 01/secret.mkv"
	if e := os.Link(filepath.Join(outside, "secret.mkv"), filepath.Join(root, filepath.FromSlash(link))); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(outside, filepath.Join(root, "Show", "escape")); e != nil {
		t.Fatal(e)
	}
	local, e := OpenLocal(root)
	if e != nil {
		t.Fatal(e)
	}
	defer local.Close()
	f, e := local.Find(context.Background(), link)
	if e != nil || !f.Unsupported {
		t.Fatal("hard link offered", f, e)
	}
	if e = local.Relocate(context.Background(), link, "moved.mkv"); e == nil {
		t.Fatal("hard link moved")
	}
	if _, e = local.List(context.Background(), "Show/escape"); e == nil {
		t.Fatal("symlink directory traversed")
	}
	raw, e := os.ReadFile(filepath.Join(outside, "secret.mkv"))
	if e != nil || string(raw) != "outside sentinel" {
		t.Fatal(e, string(raw))
	}
}
