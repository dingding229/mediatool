package httpapi

import (
	"encoding/json"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	v2 "media-tool/internal/modules/plugins/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestAcceptanceRealWorkerSeedingPreviewAndRetire(t *testing.T) {
	for _, execute := range []bool{false, true} {
		t.Run(map[bool]string{false: "dry-run", true: "retire"}[execute], func(t *testing.T) {
			ctx, s, d, in, m := acceptanceRuntime(t, "seeding-cleanup", json.RawMessage(`{}`))
			task, resource, run := jobs.UUID(), jobs.UUID(), jobs.UUID()
			hash := strings.Repeat("c", 40)
			var deleted atomic.Bool
			var calls atomic.Int32
			fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v2/auth/login":
					w.Write([]byte("Ok."))
				case "/api/v2/app/version":
					w.Write([]byte("v5.0"))
				case "/api/v2/torrents/info":
					if deleted.Load() {
						w.Write([]byte("[]"))
						return
					}
					json.NewEncoder(w).Encode([]map[string]any{{"hash": hash, "name": "Synthetic.Movie.2026.mkv", "state": "stalledUP", "progress": 1, "amount_left": 0, "completion_on": 1, "ratio": 3, "seeding_time": 7200, "save_path": "/downloads", "category": "media-tool", "tags": task, "size": 4}})
				case "/api/v2/torrents/delete":
					r.ParseForm()
					if r.Form.Get("deleteFiles") != "false" {
						t.Error("payload delete not allowed")
					}
					calls.Add(1)
					deleted.Store(true)
				default:
					w.WriteHeader(404)
				}
			}))
			defer fake.Close()
			q, e := m.Repository.Save(ctx, "", integrations.Input{Kind: "qbittorrent", Name: "synthetic downloader", Enabled: true, Settings: map[string]any{"endpoint": fake.URL, "downloadPath": "/downloads", "localPath": t.TempDir(), "category": "media-tool"}, Secrets: map[string]string{"username": "SYNTHETIC_USER", "password": "SYNTHETIC_PASSWORD"}})
			if e != nil {
				t.Fatal(e)
			}
			details, _ := json.Marshal(map[string]any{"endpoint": fake.URL, "downloadPath": "/downloads", "category": "media-tool", "searchRunId": run})
			queries := []struct {
				q    string
				args []any
			}{
				{`INSERT INTO resources(id,account_id,torrent_id,title,download_hint_ciphertext)VALUES($1,$2,'1','Synthetic',''::bytea)`, []any{resource, q.ID}},
				{`INSERT INTO search_runs(id,keyword,account_ids,state)VALUES($1,'Synthetic','[]','succeeded')`, []any{run}},
				{`INSERT INTO search_run_resources(run_id,resource_id,public_snapshot,download_hint_ciphertext,account_revision,definition_fingerprint,observed_at)VALUES($1,$2,'{"metadata":{"hr":false}}',''::bytea,1,'synthetic',now())`, []any{run, resource}},
				{`INSERT INTO download_tasks(id,resource_id,integration_id,media_type,tmdb_id,info_hash,download_state,details)VALUES($1,$2,$3,'movie',42,$4,'imported',$5)`, []any{task, resource, q.ID, hash, details}},
			}
			for _, x := range queries {
				if _, e = s.DB.Exec(ctx, x.q, x.args...); e != nil {
					t.Fatal(e)
				}
			}
			raw, _ := json.Marshal(map[string]any{"active": true, "execute": execute, "deleteFiles": false, "minRatio": "2", "minSeedingHours": "1", "batchSize": "2"})
			in, e = s.Apply(ctx, "admin", v2.Request{Kind: v2.Configure, InstanceID: in.ID, ExpectedRevision: in.Revision, Fingerprint: d.Fingerprint, OperationKey: jobs.UUID(), Confirm: true, Config: raw})
			if e != nil {
				t.Fatal(e)
			}
			startAcceptance(t, ctx, s, d, in)
			for range 2 {
				if e = runAcceptanceService(t, ctx, s, in.ID, "cleanup"); e != nil {
					t.Fatal(e)
				}
			}
			want := int32(0)
			if execute {
				want = 1
			}
			if calls.Load() != want {
				t.Fatal("duplicate or missing effect", calls.Load(), want)
			}
			if _, e = s.DB.Exec(ctx, `DELETE FROM plugin_v2_grants WHERE instance_id=$1 AND scope='downloads.delete'`, in.ID); e != nil {
				t.Fatal(e)
			}
			if e = runAcceptanceService(t, ctx, s, in.ID, "cleanup"); e == nil {
				t.Fatal("revoked scope accepted")
			}
			stopAcceptance(t, ctx, s, d, in.ID)
			t.Log("same app worker: native downloader preview/retire, default no payload deletion, receipt dedupe, revoked scope and reaper")
		})
	}
}
func TestAcceptanceRealWorkerWatchQuietCoreOrganizeAndDisable(t *testing.T) {
	ctx, s, d, in, m := acceptanceRuntime(t, "watch-organize", json.RawMessage(`{}`))
	root := t.TempDir()
	for _, name := range []string{"incoming", "organized"} {
		if e := os.Mkdir(filepath.Join(root, name), 0700); e != nil {
			t.Fatal(e)
		}
	}
	if e := os.MkdirAll(filepath.Join(root, "incoming", "Synthetic Show", "Season 01"), 0700); e != nil {
		t.Fatal(e)
	}
	source := filepath.Join(root, "incoming", "Synthetic Show", "Season 01", "Synthetic.Movie.2026.1080p.mkv")
	if e := os.WriteFile(source, []byte("tiny synthetic media"), 0600); e != nil {
		t.Fatal(e)
	}
	// Symlink evidence is listed unsupported and never submitted as a file plan.
	if e := os.Symlink(t.TempDir(), filepath.Join(root, "incoming", "escape-directory")); e != nil {
		t.Fatal(e)
	}
	strm := filepath.Join(root, "protected.strm")
	original := []byte("https://stable.fixture/resolve/fixed")
	if e := os.WriteFile(strm, original, 0600); e != nil {
		t.Fatal(e)
	}
	q, e := m.Repository.Save(ctx, "", integrations.Input{Kind: "local", Name: "synthetic directory", Enabled: true, Settings: map[string]any{"rootPath": root}})
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(map[string]any{"active": true, "rootId": q.ID, "directory": "incoming", "destination": "organized", "quietSeconds": "30"})
	in, e = s.Apply(ctx, "admin", v2.Request{Kind: v2.Configure, InstanceID: in.ID, ExpectedRevision: in.Revision, Fingerprint: d.Fingerprint, OperationKey: jobs.UUID(), Confirm: true, Config: raw})
	if e != nil {
		t.Fatal(e)
	}
	startAcceptance(t, ctx, s, d, in)
	if e = runAcceptanceService(t, ctx, s, in.ID, "scan"); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(source); e != nil {
		t.Fatal("moved before quiet window", e)
	}
	time.Sleep(31 * time.Second)
	for range 2 {
		if e = runAcceptanceService(t, ctx, s, in.ID, "scan"); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = os.Stat(source); !os.IsNotExist(e) {
		t.Fatal("source was not core-organized", e)
	}
	var plans int
	if e = s.DB.QueryRow(ctx, `SELECT count(*)FROM file_operations WHERE state='succeeded' AND plan->>'operation'='organize'`).Scan(&plans); e != nil || plans != 1 {
		t.Fatal("duplicate organize", plans, e)
	}
	if raw, e = os.ReadFile(strm); e != nil || string(raw) != string(original) {
		t.Fatal("STRM changed", e)
	}
	if _, e = s.DB.Exec(ctx, `DELETE FROM plugin_v2_grants WHERE instance_id=$1 AND scope='fs.media:write'`, in.ID); e != nil {
		t.Fatal(e)
	}
	if e = runAcceptanceService(t, ctx, s, in.ID, "scan"); e == nil {
		t.Fatal("revoked service scope accepted")
	}
	stopAcceptance(t, ctx, s, d, in.ID)
	t.Log("same app worker: quiet/size/mtime -> native organize preview/digest/apply; no symlink escape, duplicate submit, STRM changes or worker residue")
}
