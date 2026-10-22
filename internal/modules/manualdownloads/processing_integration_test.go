package manualdownloads

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"media-tool/internal/modules/acquisition"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
)

func TestManualImportOptInIdentityUncertaintyAndManagedAdoption(t *testing.T) {
	for _, mode := range []string{"automatic", "ambiguous", "no-year", "legacy", "frozen-mapping"} {
		t.Run(mode, func(t *testing.T) {
			dsn := os.Getenv("TEST_DATABASE_URL")
			if dsn == "" {
				t.Skip("isolated PG")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			db, err := database.Open(ctx, dsn)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			var name string
			if err = db.QueryRow(ctx, `SELECT current_database()`).Scan(&name); err != nil || name != "media_tool_test" {
				t.Fatal("wrong test database", name, err)
			}
			if _, err = db.Exec(ctx, `TRUNCATE integrations,jobs,manual_download_tasks CASCADE`); err != nil {
				t.Fatal(err)
			}
			defer db.Exec(context.Background(), `TRUNCATE integrations,jobs,manual_download_tasks CASCADE`)
			local := t.TempDir()
			filename := "Owned.Film.2026.mkv"
			if mode == "no-year" {
				filename = "unknown.mkv"
			}
			if err = os.WriteFile(filepath.Join(local, filename), []byte("x"), 0600); err != nil {
				t.Fatal(err)
			}
			source, err := File([]byte(fmt.Sprintf("d4:infod6:lengthi1e4:name%d:%s12:piece lengthi16384e6:pieces20:%see", len(filename), filename, strings.Repeat("a", 20))), "owned.torrent")
			if err != nil {
				t.Fatal(err)
			}
			var mu sync.Mutex
			present := false
			adds := 0
			deleted := 0
			qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				switch r.URL.Path {
				case "/api/v2/auth/login":
					io.WriteString(w, "Ok.")
				case "/api/v2/app/version":
					io.WriteString(w, "v5.0.4")
				case "/api/v2/torrents/add":
					present = true
					adds++
					io.WriteString(w, "Ok.")
				case "/api/v2/torrents/info":
					if !present {
						io.WriteString(w, "[]")
						return
					}
					json.NewEncoder(w).Encode([]map[string]any{{"hash": source.Meta.InfoHash, "name": filename, "state": "stalledUP", "progress": 1, "amount_left": 0, "completion_on": 1, "save_path": "/downloads", "category": "media-tool", "size": 1}})
				case "/api/v2/torrents/files":
					json.NewEncoder(w).Encode([]map[string]any{{"index": 0, "name": filename, "priority": 1, "progress": 1, "size": 1}})
				case "/api/v2/torrents/trackers":
					io.WriteString(w, "[]")
				case "/api/v2/torrents/delete":
					deleted++
					t.Error("source deleted before verified import")
				default:
					t.Errorf("unexpected QB path %s", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer qb.Close()
			vault, err := secrets.New(strings.Repeat("a", 64))
			if err != nil {
				t.Fatal(err)
			}
			repo := integrations.Repository{DB: db, Vault: vault}
			qbRecord, err := repo.Save(ctx, "", integrations.Input{Kind: "qbittorrent", Name: "Owned manual test", Enabled: true, Settings: map[string]any{"endpoint": qb.URL, "downloadPath": "/downloads", "localPath": local, "category": "media-tool"}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = repo.Save(ctx, "", integrations.Input{Kind: "tmdb", Name: "Owned identity fixture", Enabled: true, Settings: map[string]any{"language": "zh-CN"}, Secrets: map[string]string{"token": "synthetic-fixture"}}); err != nil {
				t.Fatal(err)
			}
			manager := integrations.NewManager(repo, config.Config{})
			defer manager.Close()
			manager.OutboundHTTP = &http.Client{Transport: torrentTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host != "api.themoviedb.org" {
					t.Fatal("unexpected provider host", r.URL.Host)
				}
				item := `{"id":99,"title":"Owned Film","original_title":"Owned Film","release_date":"2026-01-01"}`
				raw := item
				if strings.Contains(r.URL.Path, "/search/") {
					raw = `{"results":[` + item
					if mode == "ambiguous" {
						raw += `,` + strings.Replace(item, `"id":99`, `"id":100`, 1)
					}
					raw += `]}`
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(raw)), Request: r}, nil
			})}
			var previousSettings []byte
			if err = db.QueryRow(ctx, `SELECT value FROM application_settings WHERE singleton`).Scan(&previousSettings); err != nil {
				t.Fatal(err)
			}
			defer db.Exec(context.Background(), `UPDATE application_settings SET value=$1 WHERE singleton`, previousSettings)
			if _, err = db.Exec(ctx, `UPDATE application_settings SET value=jsonb_set(value,'{cleanupAfterImport}','true') WHERE singleton`); err != nil {
				t.Fatal(err)
			}
			e := Engine{DB: db, Connections: manager}
			var id string
			if mode == "legacy" {
				id, err = e.Create(ctx, source)
			} else {
				id, err = e.CreateWithImport(ctx, source)
			}
			if err != nil {
				t.Fatal(err)
			}
			sourceProof, err := e.importSource(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if (sourceProof.options["autoImport"] == "true") != (mode != "legacy") {
				t.Fatal("wrong initial opt-in")
			}
			if mode != "legacy" && sourceProof.options["cleanupAfterImport"] != "true" {
				t.Fatal("new-task cleanup intent not frozen")
			}
			if _, err = db.Exec(ctx, `UPDATE application_settings SET value=jsonb_set(value,'{cleanupAfterImport}','false') WHERE singleton`); err != nil {
				t.Fatal(err)
			}
			queue := jobs.Repository{DB: db}
			job, err := queue.Claim(ctx, "manual-test", time.Minute)
			if err != nil || job == nil || job.Kind != "download.manual.add" {
				t.Fatal(job, err)
			}
			if err = e.Process(ctx, *job); err != nil {
				t.Fatal(err)
			}
			if err = queue.Finish(ctx, *job, nil); err != nil {
				t.Fatal(err)
			}
			if err = e.Sync(ctx); err != nil {
				t.Fatal(err)
			}
			if err = e.Sync(ctx); err != nil {
				t.Fatal(err)
			}
			if mode == "legacy" {
				var count int
				db.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE kind='download.manual.import'`).Scan(&count)
				if count != 0 {
					t.Fatal("legacy auto-backfill")
				}
				if err = e.ConfirmImport(ctx, id, "movie", 99, false); err == nil {
					t.Fatal("missing consent accepted")
				}
				if err = e.ConfirmImport(ctx, id, "movie", 99, true); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "frozen-mapping" {
				if _, err = db.Exec(ctx, `UPDATE integrations SET settings=jsonb_set(settings,'{localPath}',to_jsonb($2::text)),revision=revision+1 WHERE id=$1`, qbRecord.ID, t.TempDir()); err != nil {
					t.Fatal(err)
				}
			}
			job, err = queue.Claim(ctx, "manual-test", time.Minute)
			if err != nil || job == nil || job.Kind != "download.manual.import" {
				t.Fatal(job, err)
			}
			result := e.ProcessImport(ctx, *job)
			if err = queue.Finish(ctx, *job, result); err != nil {
				t.Fatal(err)
			}
			succeeds := mode == "automatic" || mode == "legacy"
			if succeeds && result != nil {
				t.Fatal(result)
			}
			if !succeeds && result == nil {
				t.Fatal("uncertain source imported")
			}
			if mode == "ambiguous" || mode == "no-year" {
				if !errors.Is(result, jobs.ErrOperatorRequired) {
					t.Fatal("uncertainty must stop for operator", result)
				}
			}
			var count int
			if err = db.QueryRow(ctx, `SELECT count(*) FROM download_tasks WHERE id=$1`, id).Scan(&count); err != nil || count != map[bool]int{true: 1, false: 0}[succeeds] {
				t.Fatal("wrong managed count", count, err)
			}
			if succeeds {
				if err = e.ProcessImport(ctx, *job); err != nil {
					t.Fatal("not idempotent", err)
				}
				a := acquisition.Engine{DB: db, Connections: manager}
				if err = a.Reconcile(ctx); err != nil {
					t.Fatal(err)
				}
				tasks, err := a.List(ctx)
				if err != nil || len(tasks) != 1 || tasks[0].ResourceID != "" || tasks[0].State != "local_complete" {
					t.Fatal("did not reuse managed reconciliation", tasks, err)
				}
			}
			if mode == "automatic" {
				a := acquisition.Engine{DB: db, Connections: manager}
				var frozen bool
				if err = db.QueryRow(ctx, `SELECT details->>'cleanupAfterImport'='true' FROM download_tasks WHERE id=$1`, id).Scan(&frozen); err != nil || !frozen {
					t.Fatal("later setting changed old intent", frozen, err)
				}
				if err = a.ApproveCleanup(ctx, id, true, true); err != nil {
					t.Fatal(err)
				}
				raw, _ := json.Marshal(map[string]string{"taskId": id})
				if err = a.Cleanup(ctx, jobs.Job{Payload: raw}); err == nil || !strings.Contains(err.Error(), "入库尚未完成") {
					t.Fatal("cleanup passed before verified import", err)
				}
			}
			if b, err := os.ReadFile(filepath.Join(local, filename)); err != nil || string(b) != "x" {
				t.Fatal("source mutated", err)
			}
			mu.Lock()
			if adds != 1 || deleted != 0 {
				t.Error("wrong external side effects", adds, deleted)
			}
			mu.Unlock()
		})
	}
}
