package linksource

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootReaderSafeReadAndRejections(t *testing.T) {
	root := t.TempDir()
	reader := RootReader{Root: root}
	ctx := context.Background()
	body := []byte("https://resolver.example/stable?apiKey=NEVER_LOG_GRANT\n")
	for name, raw := range map[string][]byte{"ok.strm": body, "big.strm": []byte(strings.Repeat("x", 8193)), "bad.strm": {255}} {
		if err := os.WriteFile(filepath.Join(root, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(root, "dir.strm"), 0700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.strm")
	if err := os.WriteFile(outside, body, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link.strm")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(outside), filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	got, err := reader.Read(ctx, "ok.strm")
	if err != nil || string(got) != string(body) {
		t.Fatal("read failed")
	}
	for _, name := range []string{"../outside.strm", "/etc/passwd", "a/../ok.strm", "a\\ok.strm", "", ".", "..", "bad.txt", "big.strm", "bad.strm", "dir.strm", "link.strm"} {
		t.Run(name, func(t *testing.T) {
			_, err := reader.Read(ctx, name)
			assertFailure(t, err, Manual, Mismatch)
			noSecrets(t, err.Error())
		})
	}
	for _, name := range []string{"missing.strm", "escape/outside.strm"} {
		_, err := reader.Read(ctx, name)
		if err == nil {
			t.Fatal("missing/escape allowed")
		}
		noSecrets(t, err.Error())
	}
	_, err = reader.Read(ctx, "missing.strm")
	assertFailure(t, err, Permanent, FileMissing)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = reader.Read(canceled, "ok.strm")
	assertFailure(t, err, Retryable, Timeout)
	_, err = (RootReader{Root: filepath.Join(root, "absent")}).Read(ctx, "ok.strm")
	assertFailure(t, err, Permanent, FileMissing)
}
