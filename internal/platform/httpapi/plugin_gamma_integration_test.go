package httpapi

import (
	"encoding/json"
	"io"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	v2 "media-tool/internal/modules/plugins/v2"
	"media-tool/internal/modules/sites"
	"media-tool/internal/platform/auth"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type gammaTransport func(*http.Request) (*http.Response, error)

func (f gammaTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestAcceptanceRealWorkerWishlistSyntheticSourcesAndManualChoice(t *testing.T) {
	for _, source := range []string{"tmdb-list", "douban-wish"} {
		t.Run(source, func(t *testing.T) {
			ctx, s, d, in, m := acceptanceRuntime(t, "wishlist-subscribe", json.RawMessage(`{}`))
			var sources atomic.Int32
			m.OutboundHTTP = &http.Client{Transport: gammaTransport(func(r *http.Request) (*http.Response, error) {
				body := ""
				if r.URL.Host == "movie.douban.com" {
					sources.Add(1)
					if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
						t.Error("douban inherited credentials")
					}
					body = `<div class="grid-view"><div class="item"><li class="title"><a href="https://movie.douban.com/subject/123/">Synthetic</a></li><li class="intro">2026</li></div></div>`
				} else if r.URL.Host == "api.themoviedb.org" {
					if r.Header.Get("Authorization") != "Bearer SYNTHETIC_TMDB_TOKEN" {
						t.Error("host token missing")
					}
					switch {
					case strings.Contains(r.URL.Path, "/list/"):
						sources.Add(1)
						body = `{"items":[{"id":101,"media_type":"movie","title":"Synthetic","release_date":"2026-01-01"}],"item_count":1}`
					case strings.Contains(r.URL.Path, "/search/movie"):
						body = `{"results":[{"id":101,"title":"Synthetic","release_date":"2026-01-01"},{"id":102,"title":"Synthetic","release_date":"2026-01-01"}]}`
					case strings.Contains(r.URL.Path, "/search/tv"):
						body = `{"results":[]}`
					case strings.Contains(r.URL.Path, "/movie/"):
						body = `{"id":101,"title":"Synthetic","release_date":"2026-01-01"}`
					default:
						t.Error("unexpected synthetic request", r.URL.Path)
					}
				} else {
					t.Error("real network forbidden", r.URL.Host)
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Set-Cookie": {"SYNTHETIC_COOKIE"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			if _, e := m.Repository.Save(ctx, "", integrations.Input{Kind: "tmdb", Name: "synthetic", Enabled: true, Secrets: map[string]string{"token": "SYNTHETIC_TMDB_TOKEN"}}); e != nil {
				t.Fatal(e)
			}
			if e := (sites.Repository{DB: s.DB}).LoadGoTemplates(ctx, []string{"hdsky"}); e != nil {
				t.Fatal(e)
			}
			account, e := m.Repository.Save(ctx, "", integrations.Input{Kind: "ptsite", Name: "synthetic", Enabled: true, Settings: map[string]any{"definitionId": "hdsky", "endpoint": "https://synthetic.invalid", "userId": "1"}, Secrets: map[string]string{"cookie": "SYNTHETIC_PT_COOKIE"}})
			if e != nil {
				t.Fatal(e)
			}
			raw, _ := json.Marshal(map[string]any{"active": true, "create": true, "source": source, "id": "42", "accountIds": account.ID, "intervalMinutes": "30", "perRun": "10"})
			in, e = s.Apply(ctx, "admin", v2.Request{Kind: v2.Configure, InstanceID: in.ID, ExpectedRevision: in.Revision, Fingerprint: d.Fingerprint, OperationKey: jobs.UUID(), Confirm: true, Config: raw})
			if e != nil {
				t.Fatal(e)
			}
			startAcceptance(t, ctx, s, d, in)
			for range 2 {
				if e = runAcceptanceService(t, ctx, s, in.ID, "sync"); e != nil {
					t.Fatal(e)
				}
			}
			if sources.Load() != 1 {
				t.Fatal("source interval ignored", sources.Load())
			}
			var count int
			if e = s.DB.QueryRow(ctx, `SELECT count(*) FROM subscriptions`).Scan(&count); e != nil {
				t.Fatal(e)
			}
			if source == "douban-wish" {
				if count != 0 {
					t.Fatal("ambiguous source guessed identity")
				}
				view, e := s.Inspect(ctx, in.ID, "")
				if e != nil {
					t.Fatal(e)
				}
				found := false
				for _, v := range view.Entries {
					if v.Key == "wishlist-state" {
						found = strings.Contains(string(v.Value), `"pending":[{`) && strings.Contains(string(v.Value), `douban:123`)
					}
				}
				if !found {
					t.Fatal("missing manual candidates")
				}
				raw = json.RawMessage(`{"data":{"sourceId":"douban:123","tmdbId":101,"mediaType":"movie"}}`)
				if _, e = s.Invoke(ctx, in.ID, "action", "resolve", raw, jobs.UUID()); e != nil {
					t.Fatal(e)
				}
			} else if count != 1 {
				t.Fatal(count)
			}
			if e = s.DB.QueryRow(ctx, `SELECT count(*) FROM subscriptions WHERE NOT enabled AND identity->>'id'='101'`).Scan(&count); e != nil || count != 1 {
				t.Fatal("missing disabled exact subscription", count, e)
			}
			var queued int
			if e = s.DB.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE kind='subscription.run'`).Scan(&queued); e != nil || queued != 0 {
				t.Fatal("unexpected acquisition side effect", queued, e)
			}
			if _, e = s.DB.Exec(ctx, `DELETE FROM plugin_v2_grants WHERE instance_id=$1 AND scope='subscriptions.write'`, in.ID); e != nil {
				t.Fatal(e)
			}
			if e = runAcceptanceService(t, ctx, s, in.ID, "sync"); e == nil {
				t.Fatal("revocation ignored")
			}
			stopAcceptance(t, ctx, s, d, in.ID)
			t.Log("same app worker: synthetic TMDB/public Douban -> exact/pending choice -> native idempotent disabled subscription, no search/download, revocation/reaper")
		})
	}
}
func TestAcceptanceRealWorkerDownloaderAssistantAndAdminStatus(t *testing.T) {
	ctx, s, d, in, m := acceptanceRuntime(t, "torrent-assistant", json.RawMessage(`{}`))
	task, resource := jobs.UUID(), jobs.UUID()
	hash := strings.Repeat("d", 40)
	var mu sync.Mutex
	tags := task
	var mutations atomic.Int32
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/api/v2/torrents/info":
			json.NewEncoder(w).Encode([]map[string]any{{"hash": hash, "name": "Synthetic", "state": "error", "progress": .5, "save_path": "/downloads", "category": "media-tool", "tags": tags, "dl_limit": 0, "up_limit": 0, "auto_tmm": false}})
		case "/api/v2/torrents/addTags":
			r.ParseForm()
			tags += "," + r.Form.Get("tags")
			mutations.Add(1)
		default:
			t.Error("unexpected downloader mutation", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer fake.Close()
	q, e := m.Repository.Save(ctx, "", integrations.Input{Kind: "qbittorrent", Name: "synthetic", Enabled: true, Settings: map[string]any{"endpoint": fake.URL, "downloadPath": "/downloads", "localPath": t.TempDir(), "category": "media-tool"}, Secrets: map[string]string{"username": "SYNTHETIC", "password": "SYNTHETIC_PASSWORD"}})
	if e != nil {
		t.Fatal(e)
	}
	details, _ := json.Marshal(map[string]any{"endpoint": fake.URL, "downloadPath": "/downloads", "category": "media-tool"})
	for _, v := range []struct {
		sql  string
		args []any
	}{{`INSERT INTO resources(id,account_id,torrent_id,title,download_hint_ciphertext)VALUES($1,$2,'1','Synthetic',''::bytea)`, []any{resource, q.ID}}, {`INSERT INTO download_tasks(id,resource_id,integration_id,media_type,tmdb_id,info_hash,download_state,details)VALUES($1,$2,$3,'movie',101,$4,'downloading',$5)`, []any{task, resource, q.ID, hash, details}}} {
		if _, e = s.DB.Exec(ctx, v.sql, v.args...); e != nil {
			t.Fatal(e)
		}
	}
	raw, _ := json.Marshal(map[string]any{"active": true, "write": true, "recheck": false, "tag": "verified", "batch": "5", "stallMinutes": "30"})
	in, e = s.Apply(ctx, "admin", v2.Request{Kind: v2.Configure, InstanceID: in.ID, ExpectedRevision: in.Revision, Fingerprint: d.Fingerprint, OperationKey: jobs.UUID(), Confirm: true, Config: raw})
	if e != nil {
		t.Fatal(e)
	}
	startAcceptance(t, ctx, s, d, in)
	for range 2 {
		if e = runAcceptanceService(t, ctx, s, in.ID, "assist"); e != nil {
			t.Fatal(e)
		}
	}
	if mutations.Load() != 1 {
		t.Fatal("mutation repeated", mutations.Load())
	}
	mu.Lock()
	owned := strings.Contains(tags, task) && strings.Contains(tags, "plugin:"+in.ID) && strings.Contains(tags, "verified")
	mu.Unlock()
	if !owned {
		t.Fatal("owner or marker missing")
	}
	c := cfg()
	c.PluginDataRoot = s.Root
	server := httptest.NewServer(New(c, Dependencies{DB: s.DB, PluginInstances: s, AuthSessions: testSessionReader{}}))
	defer server.Close()
	token, e := (auth.Auth{Key: c.SessionKey}).Issue(time.Now())
	if e != nil {
		t.Fatal(e)
	}
	req, e := http.NewRequestWithContext(ctx, "GET", server.URL+"/api/v1/plugin/"+in.ID+"/status", nil)
	if e != nil {
		t.Fatal(e)
	}
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	resp, e := server.Client().Do(req)
	if e != nil {
		t.Fatal(e)
	}
	body, e := io.ReadAll(resp.Body)
	resp.Body.Close()
	if e != nil || resp.StatusCode != 200 || !strings.Contains(string(body), "configured") {
		t.Fatal(resp.StatusCode, string(body), e)
	}
	if strings.Contains(string(body), "SYNTHETIC_PASSWORD") {
		t.Fatal("credential leak")
	}
	if _, e = s.DB.Exec(ctx, `DELETE FROM plugin_v2_grants WHERE instance_id=$1 AND scope='downloads.control'`, in.ID); e != nil {
		t.Fatal(e)
	}
	if e = runAcceptanceService(t, ctx, s, in.ID, "assist"); e == nil {
		t.Fatal("revoked assistant allowed")
	}
	stopAcceptance(t, ctx, s, d, in.ID)
	t.Log("same app worker: synthetic downloader append tag, owner preserved, receipt dedupe, real admin HTTP status, revocation/reaper")
}
