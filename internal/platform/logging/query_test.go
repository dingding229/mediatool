package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeQueryRecord(t *testing.T, s *Store, at time.Time, level, msg, module string) {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"time": at.Format(time.RFC3339Nano), "level": level, "msg": msg, "module": module})
	raw = append(raw, '\n')
	if _, err := s.Write(raw); err != nil {
		t.Fatal(err)
	}
}
func TestLogQueryFiltersAndStableCutoff(t *testing.T) {
	s, err := Open(t.TempDir(), "debug")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	start := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	for i := range 7 {
		writeQueryRecord(t, s, start.Add(time.Duration(i)*time.Second), "INFO", fmt.Sprintf("owned-%d", i), "search")
	}
	writeQueryRecord(t, s, start.Add(7*time.Second), "WARN", "provider unavailable", "open115")
	q := Query{Page: 1, PageSize: 3, Before: start.Add(8 * time.Second)}
	out, err := s.Query(context.Background(), q)
	if err != nil || out.Total != 8 || len(out.Items) != 3 || out.Items[0]["msg"] != "provider unavailable" {
		t.Fatal(out, err)
	}
	q.Before = out.Before
	q.Page = 2
	writeQueryRecord(t, s, start.Add(9*time.Second), "ERROR", "new-after-snapshot", "search")
	second, err := s.Query(context.Background(), q)
	if err != nil || second.Total != 8 || second.Items[0]["msg"] != "owned-4" {
		t.Fatal(second, err)
	}
	q.Page = 1
	q.Level = "warning"
	q.PageSize = 100
	warning, err := s.Query(context.Background(), q)
	if err != nil || warning.Total != 1 {
		t.Fatal(warning, err)
	}
	q.Level = ""
	q.Module = "search"
	q.Keyword = "OWNED"
	q.From = start.Add(2 * time.Second)
	q.To = start.Add(3 * time.Second)
	matches, err := s.Query(context.Background(), q)
	if err != nil || matches.Total != 2 || matches.Items[0]["msg"] != "owned-3" || strings.Join(matches.Modules, ",") != "open115,search" {
		t.Fatal(matches, err)
	}
	q.Page = 10000
	empty, err := s.Query(context.Background(), q)
	if err != nil || empty.Total != 2 || len(empty.Items) != 0 {
		t.Fatal(empty, err)
	}
}
func TestLogQueryValidationCancellationAndUnsafeSource(t *testing.T) {
	s, err := Open(t.TempDir(), "info")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	valid := Query{Page: 1, PageSize: 100}
	for _, q := range []Query{{Page: 0, PageSize: 100}, {Page: 1, PageSize: 201}, {Page: 10001, PageSize: 50}, {Page: 1, PageSize: 50, Level: "verbose"}, {Page: 1, PageSize: 50, Keyword: strings.Repeat("词", 201)}, {Page: 1, PageSize: 50, Module: strings.Repeat("a", 121)}, {Page: 1, PageSize: 50, From: time.Now(), To: time.Now().Add(-time.Hour)}} {
		if _, err := s.Query(context.Background(), q); err == nil {
			t.Fatal("accepted invalid", q)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Query(ctx, valid); err == nil {
		t.Fatal("cancel ignored")
	}
	target := filepath.Join(t.TempDir(), "secret")
	os.WriteFile(target, []byte("private"), 0600)
	if err := s.root.Symlink(target, "archive-1.jsonl"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Query(context.Background(), valid); err == nil {
		t.Fatal("symlink accepted")
	}
}
func TestLogQueryBoundedWindowAcrossArchives(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, "debug")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	start := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	old, _ := json.Marshal(map[string]any{"time": start.Format(time.RFC3339), "level": "INFO", "msg": "archived fixture", "source": map[string]any{"file": "internal/adapters/tmdb/client.go", "line": 1}})
	if err := os.WriteFile(filepath.Join(dir, "archive-1.jsonl"), append(old, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := s.Query(context.Background(), Query{Page: 1, PageSize: 50})
	if err != nil || out.Total != 1 || out.Items[0]["module"] != "tmdb" {
		t.Fatal(out, err)
	}
	var data bytes.Buffer
	for i := range LogQueryMaxRecords + 5 {
		raw, _ := json.Marshal(map[string]any{"time": start.Format(time.RFC3339), "level": "INFO", "msg": i})
		data.Write(raw)
		data.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(dir, "current.jsonl"), data.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	out, err = s.Query(context.Background(), Query{Page: 1, PageSize: 50})
	if err != nil || out.Total != LogQueryMaxRecords || !out.Truncated || out.Items[0]["msg"] != float64(LogQueryMaxRecords+4) {
		t.Fatal(out.Total, out.Truncated, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "current.jsonl"), append(bytes.Repeat([]byte("junk\n"), int(LogQueryFileBytes)/5+2), old...), 0600); err != nil {
		t.Fatal(err)
	}
	out, err = s.Query(context.Background(), Query{Page: 1, PageSize: 50})
	if err != nil || !out.Truncated {
		t.Fatal(out, err)
	}
}
func TestLogQueryRedactionAndSafeSource(t *testing.T) {
	s, err := Open(t.TempDir(), "info")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	logger, err := New(s, "info", "owned-secret-value")
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("owned-secret-value https://provider.example/a?api_key=private", "password", "must-not-leak")
	out, err := s.Query(context.Background(), Query{Page: 1, PageSize: 50})
	if err != nil || out.Total != 1 {
		t.Fatal(out, err)
	}
	raw, _ := json.Marshal(out)
	for _, secret := range []string{"owned-secret-value", "must-not-leak", "provider.example", "api_key=private"} {
		if bytes.Contains(raw, []byte(secret)) {
			t.Fatal("leak", secret)
		}
	}
	source, ok := out.Items[0]["source"].(map[string]any)
	if !ok || source["file"] != "internal/platform/logging/query_test.go" || source["line"].(float64) <= 0 {
		t.Fatal(source)
	}
	if out.Items[0]["module"] != "logging" {
		t.Fatal(out.Items[0])
	}
}

func TestInstanceFilterIsExactAndCombinedWithModule(t *testing.T) {
	s, e := Open(t.TempDir(), "info")
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	for _, line := range []string{`{"time":"2026-10-17T00:00:00Z","level":"INFO","module":"plugins","instance":"first","msg":"one"}`, `{"time":"2026-10-17T00:00:00Z","level":"INFO","module":"plugins","instance":"first-extra","msg":"two"}`, `{"time":"2026-10-17T00:00:00Z","level":"INFO","module":"jobs","instance":"first","msg":"three"}`} {
		if _, e = s.Write([]byte(line + "\n")); e != nil {
			t.Fatal(e)
		}
	}
	out, e := s.Query(context.Background(), Query{Page: 1, PageSize: 50, Module: "plugins", Instance: "first"})
	if e != nil || out.Total != 1 || out.Items[0]["msg"] != "one" {
		t.Fatal(out, e)
	}
}
