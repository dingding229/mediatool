package logging

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestFileLogStoreFilterClearAndDynamicLevel(t *testing.T) {
	store, err := Open(t.TempDir(), "info")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var console bytes.Buffer
	logger, err := NewControlled(store, "info", &store.Level, "private-fixture-key")
	if err != nil {
		t.Fatal(err)
	}
	logger.Debug("not-yet-visible")
	logger.Info("event", "password", "do-not-show", "url", "https://cdn.test/a?sign=secret")
	logger.Warn("warning")
	rows, err := store.Read("info", 100)
	if err != nil || len(rows) != 1 || rows[0]["level"] != "INFO" {
		t.Fatal(rows, err)
	}
	raw := []byte(console.String())
	_ = raw
	if err = store.SetLevel("debug"); err != nil {
		t.Fatal(err)
	}
	logger.Debug("debug-event")
	rows, _ = store.Read("debug", 100)
	if len(rows) != 1 || rows[0]["level"] != "DEBUG" {
		t.Fatal(rows)
	}
	stats, _ := store.Stats()
	if stats.TotalBytes == 0 || stats.Level != "debug" {
		t.Fatal(stats)
	}
	if err = store.Clear(); err != nil {
		t.Fatal(err)
	}
	stats, _ = store.Stats()
	if stats.TotalBytes != 0 {
		t.Fatal(stats)
	}
	if _, err = store.Read("invalid", 100); err == nil {
		t.Fatal("level accepted")
	}
	logger.Info("after-clear")
	rows, _ = store.Read("", 100)
	if len(rows) != 1 {
		t.Fatal(rows)
	}
}
func TestLogRotationAndConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir, "debug")
	if err != nil {
		t.Fatal(err)
	}
	logger, _ := NewControlled(store, "debug", &store.Level)
	store.size = MaxLogFileBytes - 1
	logger.Info("rotate")
	if _, err = os.Stat(filepath.Join(dir, "archive-1.jsonl")); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 20 {
				logger.LogAttrs(nil, slog.LevelInfo, "concurrent", slog.String("key", "value"))
			}
		})
	}
	wg.Wait()
	store.Close()
	data, _ := os.ReadFile(filepath.Join(dir, "current.jsonl"))
	if strings.Contains(string(data), "[details omitted]") {
		t.Fatal("severity redacted")
	}
}
func TestLogRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "secret")
	os.WriteFile(target, []byte("private"), 0600)
	os.Symlink(target, filepath.Join(dir, "current.jsonl"))
	if _, err := Open(dir, "info"); err == nil {
		t.Fatal("link accepted")
	}
}

func TestLogRotationFailureReopensCurrentFile(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir, "info")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.size = MaxLogFileBytes
	if err = os.Mkdir(filepath.Join(dir, "archive-4.jsonl"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Write([]byte("rotation failure\n")); err == nil {
		t.Fatal("rotation unexpectedly succeeded")
	}
	if store.file == nil {
		t.Fatal("current log file was left closed")
	}
	if err = os.Remove(filepath.Join(dir, "archive-4.jsonl")); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Write([]byte("after recovery\n")); err != nil {
		t.Fatal(err)
	}
}

func TestLogClearFailureRecoversWithoutRestart(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, "info")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = os.Mkdir(filepath.Join(dir, "archive-2.jsonl"), 0700); err != nil {
		t.Fatal(err)
	}
	if err = s.Clear(); err == nil {
		t.Fatal("unsafe archive unexpectedly cleared")
	}
	if s.file == nil {
		t.Fatal("clear failure left file logging closed")
	}
	if _, err = s.Write([]byte("after failed clear\n")); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Write([]byte("after close\n")); err == nil {
		t.Fatal("explicit Close was reopened")
	}
}
