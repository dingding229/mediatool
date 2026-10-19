package acquisition

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	dbutil "media-tool/internal/platform/database"
	"media-tool/internal/platform/pluginactor"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestNativeAssistantOwnedReadbackPolicyAndUnknown(t *testing.T) {
	for _, mode := range []string{"valid", "auto", "hr", "protected", "unknown", "revoked", "unsafe-category"} {
		t.Run(mode, func(t *testing.T) {
			f := newSourceFixture(t)
			ctx := pluginactor.With(f.ctx, jobs.UUID(), "downloads.control", nil)
			db := f.engine.DB
			var mu sync.Mutex
			tags, category, state := f.taskID, "media-tool", "stalledUP"
			dl, ul, seed, inactive := int64(0), int64(0), int64(-1), int64(-1)
			ratio := float64(-1)
			auto := mode == "auto"
			posts := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				switch r.URL.Path {
				case "/api/v2/torrents/info":
					shown := tags
					if mode == "protected" {
						shown += ",plugin:another-instance"
					}
					json.NewEncoder(w).Encode([]map[string]any{{"hash": f.hash, "name": "Synthetic", "state": state, "progress": 1, "completion_on": 1, "save_path": "/downloads", "category": category, "tags": shown, "dl_limit": dl, "up_limit": ul, "ratio_limit": ratio, "max_ratio": ratio, "seeding_time_limit": seed, "max_seeding_time": seed, "inactive_seeding_time_limit": inactive, "auto_tmm": auto}})
				case "/api/v2/torrents/categories":
					json.NewEncoder(w).Encode(map[string]any{"safe": map[string]any{"savePath": "/downloads"}, "unsafe": map[string]any{"savePath": "/outside"}})
				case "/api/v2/torrents/addTags":
					r.ParseForm()
					posts++
					tags += "," + r.Form.Get("tags")
				case "/api/v2/torrents/setCategory":
					r.ParseForm()
					posts++
					category = r.Form.Get("category")
				case "/api/v2/torrents/setDownloadLimit":
					r.ParseForm()
					posts++
					if mode != "unknown" {
						dl, _ = strconv.ParseInt(r.Form.Get("limit"), 10, 64)
					}
				case "/api/v2/torrents/setUploadLimit":
					r.ParseForm()
					posts++
					ul, _ = strconv.ParseInt(r.Form.Get("limit"), 10, 64)
				case "/api/v2/torrents/setShareLimits":
					r.ParseForm()
					posts++
					ratio, _ = strconv.ParseFloat(r.Form.Get("ratioLimit"), 64)
					seed, _ = strconv.ParseInt(r.Form.Get("seedingTimeLimit"), 10, 64)
				case "/api/v2/torrents/recheck":
					posts++
					state = "checkingUP"
				default:
					t.Error("unexpected fake request", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			var cid string
			if e := db.QueryRow(ctx, `SELECT integration_id::text FROM download_tasks WHERE id=$1`, f.taskID).Scan(&cid); e != nil {
				t.Fatal(e)
			}
			old, e := f.engine.Connections.Repository.Get(ctx, cid)
			if e != nil {
				t.Fatal(e)
			}
			_, e = f.engine.Connections.Repository.Save(ctx, cid, integrations.Input{Kind: "qbittorrent", Name: "synthetic", Enabled: true, Revision: old.Record.Revision, Settings: map[string]any{"endpoint": server.URL, "downloadPath": "/downloads", "localPath": t.TempDir(), "category": "media-tool"}, Secrets: map[string]string{"username": "FAKE_USER", "password": "FAKE_SECRET"}})
			if e != nil {
				t.Fatal(e)
			}
			if _, e = db.Exec(ctx, `UPDATE download_tasks SET info_hash=$2,download_state='imported',details=details||jsonb_build_object('endpoint',$3::text,'category','media-tool') WHERE id=$1`, f.taskID, f.hash, server.URL); e != nil {
				t.Fatal(e)
			}
			if mode == "hr" {
				if _, e = db.Exec(ctx, `UPDATE search_run_resources SET public_snapshot=jsonb_set(public_snapshot,'{metadata,hr}','true') WHERE run_id=$1`, f.runID); e != nil {
					t.Fatal(e)
				}
			}
			cap := int64(1024)
			r := float64(2)
			input := Assistance{ID: f.taskID, Tag: "verified", Category: "safe", DownloadLimit: &cap, RatioLimit: &r}
			if mode == "unsafe-category" {
				input.Category = "unsafe"
			}
			admissions := 0
			admit := func() error {
				admissions++
				if mode == "revoked" && admissions >= 3 {
					return errors.New("revoked fixture")
				}
				return nil
			}
			svc := DownloaderService{Engine: f.engine}
			e = svc.Configure(ctx, input, admit)
			if mode == "valid" {
				if e != nil {
					t.Fatal(e)
				}
				var saved string
				if e = db.QueryRow(ctx, `SELECT details->>'category' FROM download_tasks WHERE id=$1`, f.taskID).Scan(&saved); e != nil || saved != "safe" {
					t.Fatal(e, saved)
				}
				if e = svc.Recheck(ctx, f.taskID, func() error { return nil }); e != nil {
					t.Fatal(e)
				}
			} else if e == nil {
				t.Fatal("unsafe configuration accepted", mode)
			}
			mu.Lock()
			count := posts
			finalTags := tags
			mu.Unlock()
			if mode == "auto" || mode == "hr" || mode == "protected" || mode == "unsafe-category" {
				if count != 0 {
					t.Fatal("unsafe side effects", mode, count)
				}
			}
			if mode == "unknown" || mode == "revoked" {
				var pending bool
				if e = db.QueryRow(ctx, `SELECT details?'pluginAssistantIntent' FROM download_tasks WHERE id=$1`, f.taskID).Scan(&pending); e != nil || !pending {
					t.Fatal("unknown intent lost", e)
				}
				if e = svc.Configure(ctx, input, func() error { return nil }); e == nil {
					t.Fatal("unknown replay")
				}
			}
			if mode == "valid" && !strings.Contains(finalTags, "plugin:"+pluginactor.InstanceID(ctx)) {
				t.Fatal("missing protection marker")
			}
		})
	}
}
func TestNativeAssistantFinalProjectionIsFenced(t *testing.T) {
	f := newSourceFixture(t)
	ctx := dbutil.WithCommitGuard(f.ctx, func(context.Context, pgx.Tx) error { return errors.New("revoked") })
	if _, e := dbutil.Exec(ctx, f.engine.DB, `UPDATE download_tasks SET details=details||'{"pluginAssistantIntent":"fixture"}' WHERE id=$1`, f.taskID); e == nil {
		t.Fatal("revoked write committed")
	}
}
