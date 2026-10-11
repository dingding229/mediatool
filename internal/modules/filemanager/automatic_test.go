package filemanager

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateRecoveryAndLogPathsCannotBeBrowsedOrMutated(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"password", ".password.pending", ".password-reset.lock", ".env", "docker-compose.yml"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	os.Mkdir(filepath.Join(dir, "logs"), 0700)
	os.WriteFile(filepath.Join(dir, "movie.mkv"), []byte("owned"), 0600)
	local, err := OpenLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer local.Close()
	items, err := local.List(context.Background(), ".")
	if err != nil || len(items) != 1 || items[0].Name != "movie.mkv" {
		t.Fatal(items, err)
	}
	for _, name := range []string{"password", ".password.pending", "logs"} {
		if _, err = local.Find(context.Background(), name); err != ErrProtected {
			t.Fatal("private path readable", name, err)
		}
		if err = local.Relocate(context.Background(), "movie.mkv", name); err == nil {
			t.Fatal("private destination allowed", name)
		}
	}
}
