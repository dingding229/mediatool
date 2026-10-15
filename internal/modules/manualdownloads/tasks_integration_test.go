package manualdownloads

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestManualDownloadQueueHashDedupeRealProgressAndUncertainFence(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PG")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var databaseName string
	db.QueryRow(ctx, "SELECT current_database()").Scan(&databaseName)
	if databaseName != "media_tool_test" {
		t.Fatal(databaseName)
	}
	db.Exec(ctx, `TRUNCATE manual_download_tasks,integrations,jobs CASCADE`)
	defer db.Exec(ctx, `TRUNCATE manual_download_tasks,integrations,jobs CASCADE`)
	var mu sync.Mutex
	present := false
	unknown := false
	adds := 0
	hash := ""
	progress := 0.0
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/api/v2/auth/login":
			w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			if !present {
				w.Write([]byte("[]"))
				return
			}
			json.NewEncoder(w).Encode([]map[string]any{{"hash": hash, "name": "Owned task", "state": "downloading", "progress": progress, "save_path": "/downloads", "category": "media-tool", "size": 4096, "dlspeed": 1024}})
		case "/api/v2/torrents/add":
			adds++
			if strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
				r.ParseForm()
				if !strings.HasPrefix(r.Form.Get("urls"), "magnet:") {
					t.Error("magnet not submitted")
				}
			}
			if unknown {
				w.WriteHeader(502)
				return
			}
			present = true
			w.Write([]byte("Ok."))
		default:
			t.Errorf("unexpected request %s", r.URL)
			w.WriteHeader(404)
		}
	}))
	defer fake.Close()
	vault, _ := secrets.New(strings.Repeat("a", 64))
	repo := integrations.Repository{DB: db, Vault: vault}
	saved, err := repo.Save(ctx, "", integrations.Input{Kind: "qbittorrent", Name: "Owned manual fixture", Enabled: true, Settings: map[string]any{"endpoint": fake.URL, "downloadPath": "/downloads", "localPath": t.TempDir(), "category": "media-tool"}, Secrets: map[string]string{"username": "fixture", "password": "fixture"}})
	if err != nil {
		t.Fatal(err)
	}
	_ = saved
	manager := integrations.NewManager(repo, config.Config{})
	defer manager.Close()
	e := Engine{DB: db, Connections: manager}
	torrent := []byte("d4:infod6:lengthi1e4:name9:owned.mkv12:piece lengthi16384e6:pieces20:" + strings.Repeat("a", 20) + "ee")
	source, err := File(torrent, "Owned.torrent")
	if err != nil {
		t.Fatal(err)
	}
	hash = source.Meta.InfoHash
	id, err := e.Create(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	same, err := e.Create(ctx, source)
	if err != nil || same != id {
		t.Fatal("hash dedupe", err)
	}
	list, err := e.List(ctx)
	if err != nil || len(list) != 1 || list[0].Progress != nil {
		t.Fatal(list, err)
	}
	payload, _ := json.Marshal(map[string]string{"id": id})
	job := jobs.Job{Payload: payload}
	if err = e.Process(ctx, job); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	progress = .42
	mu.Unlock()
	if err = e.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	list, _ = e.List(ctx)
	if list[0].Progress == nil || *list[0].Progress != .42 || list[0].DownloadSpeed != 1024 {
		t.Fatal(list)
	}
	if err = e.Process(ctx, job); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	count := adds
	mu.Unlock()
	if count != 1 {
		t.Fatal("duplicate add", count)
	}
	// A malformed/unknown response must never authorize repeating the outbound add.
	magnet := "magnet:?xt=urn:btih:" + strings.Repeat("b", 40) + "&dn=OwnedMagnet"
	source, err = Link(ctx, magnet)
	if err != nil {
		t.Fatal(err)
	}
	id, err = e.Create(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	present = false
	unknown = true
	hash = source.Meta.InfoHash
	mu.Unlock()
	payload, _ = json.Marshal(map[string]string{"id": id})
	job.Payload = payload
	if err = e.Process(ctx, job); !errors.Is(err, jobs.ErrOperatorRequired) {
		t.Fatal("unknown response not fenced", err)
	}
	if err = e.Process(ctx, job); !errors.Is(err, jobs.ErrOperatorRequired) {
		t.Fatal(err)
	}
	mu.Lock()
	count = adds
	mu.Unlock()
	if count != 2 {
		t.Fatal("uncertain add replay", count)
	}
	var state string
	db.QueryRow(ctx, `SELECT state FROM manual_download_tasks WHERE id=$1`, id).Scan(&state)
	if state != "uncertain" {
		t.Fatal(state)
	}
	// This feature never fabricates a media binding or starts upload jobs.
	var pipeline int
	db.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE kind IN('transfer.upload','library.scan')`).Scan(&pipeline)
	if pipeline != 0 {
		t.Fatal(fmt.Sprint(pipeline))
	}
}
