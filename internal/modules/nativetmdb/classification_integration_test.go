package nativetmdb

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/adapters/tmdb"
	"media-tool/internal/modules/jobs"
)

type fixtureConnection struct{ client tmdb.Client }

func (f fixtureConnection) TMDB(context.Context) (tmdb.Client, error) { return f.client, nil }
func classificationFixture(t *testing.T) (context.Context, *pgxpool.Pool, string) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("media_tool_test required")
	}
	cfg, e := pgxpool.ParseConfig(dsn)
	if e != nil || cfg.ConnConfig.Database != "media_tool_test" {
		t.Fatal("isolated DB required")
	}
	ctx := context.Background()
	base, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer base.Close()
	schema := "tmdb_class_" + strings.ReplaceAll(jobs.UUID(), "-", "")
	q := pgx.Identifier{schema}.Sanitize()
	if _, e = base.Exec(ctx, `CREATE SCHEMA `+q); e != nil {
		t.Fatal(e)
	}
	for _, table := range []string{"storage_connections", "cloud_files", "media_assets", "media_asset_bindings", "media_items", "strm_entries", "native_tmdb_details", "jobs", "outbox_events"} {
		if _, e = base.Exec(ctx, `CREATE TABLE `+pgx.Identifier{schema, table}.Sanitize()+`(LIKE public.`+pgx.Identifier{table}.Sanitize()+` INCLUDING ALL)`); e != nil {
			t.Fatal(e)
		}
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	db, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if _, e := db.Exec(ctx, `DROP SCHEMA `+q+` CASCADE`); e != nil {
			t.Error(e)
		}
		db.Close()
	})
	exec := func(s string, args ...any) {
		t.Helper()
		if _, e := db.Exec(ctx, s, args...); e != nil {
			t.Fatal(e)
		}
	}
	storage, file, asset, media, entry, detail := jobs.UUID(), jobs.UUID(), jobs.UUID(), jobs.UUID(), jobs.UUID(), jobs.UUID()
	exec(`INSERT INTO storage_connections(id,name,provider)VALUES($1,'fixture','clouddrive2')`, storage)
	exec(`INSERT INTO cloud_files(id,storage_id,full_path,size_bytes)VALUES($1,$2,'/fixture/movie.mkv',1)`, file, storage)
	exec(`INSERT INTO media_assets(id,cloud_file_id)VALUES($1,$2)`, asset, file)
	exec(`INSERT INTO media_items(id,media_type,tmdb_id,title,metadata)VALUES($1,'movie',700,'保留入库题名','{}')`, media)
	exec(`INSERT INTO media_asset_bindings(asset_id,media_id)VALUES($1,$2)`, asset, media)
	exec(`INSERT INTO strm_entries(id,asset_id,output_path,key_reference)VALUES($1,$2,'电影/原目录/原结果.strm','fixture')`, entry, asset)
	exec(`INSERT INTO native_tmdb_details(id,tmdb_type,tmdb_id,language,payload,state,fetched_at)VALUES($1,'movie',700,'zh-CN','{"name":"保留快照题名","overview":"保留简介","communityRating":7.7,"posterPath":"/original.jpg"}','ready','2026-10-20T00:00:00Z')`, detail)
	return ctx, db, detail
}
func TestClassificationRefreshOnlyWritesFactsAndRunsOnce(t *testing.T) {
	ctx, db, id := classificationFixture(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "GET" || r.URL.Path != "/movie/700" {
			t.Error("invalid provider operation")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":700,"title":"不得覆盖的新题名","overview":"不得覆盖的新简介","vote_average":1,"original_language":"zh","production_countries":[{"iso_3166_1":"TW"}],"genres":[{"id":18,"name":"Drama"}]}`))
	}))
	defer server.Close()
	e := Engine{DB: db, Connections: fixtureConnection{tmdb.Client{Language: "zh-CN", BaseURL: server.URL, HTTP: server.Client()}}}
	target := Target{Type: "movie", TMDBID: 700}
	var old []byte
	var fetched time.Time
	if err := db.QueryRow(ctx, `SELECT payload,fetched_at FROM native_tmdb_details WHERE id=$1`, id).Scan(&old, &fetched); err != nil {
		t.Fatal(err)
	}
	out, err := e.RefreshClassification(ctx, target)
	if err != nil || !out.Changed {
		t.Fatal(out, err)
	}
	var same bool
	var raw []byte
	var afterFetched time.Time
	if err = db.QueryRow(ctx, `SELECT payload-'classification'=$2::jsonb,payload,fetched_at FROM native_tmdb_details WHERE id=$1`, id, old).Scan(&same, &raw, &afterFetched); err != nil {
		t.Fatal(err)
	}
	if !same || !fetched.Equal(afterFetched) {
		t.Fatal("non-classification snapshot fields changed")
	}
	var v map[string]any
	if err = json.Unmarshal(raw, &v); err != nil || v["classification"] == nil {
		t.Fatal(string(raw), err)
	}
	out, err = e.RefreshClassification(ctx, target)
	if err != nil || out.Changed || calls.Load() != 1 {
		t.Fatal("not idempotent", out, err, calls.Load())
	}
	for _, table := range []string{"jobs", "outbox_events"} {
		var count int
		if err = db.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&count); err != nil || count != 0 {
			t.Fatal("repair emitted jobs/events", table, count, err)
		}
	}
	var title, path string
	if err = db.QueryRow(ctx, `SELECT title FROM media_items`).Scan(&title); err != nil || title != "保留入库题名" {
		t.Fatal(title, err)
	}
	if err = db.QueryRow(ctx, `SELECT output_path FROM strm_entries`).Scan(&path); err != nil || path != "电影/原目录/原结果.strm" {
		t.Fatal(path, err)
	}
	// Existing protocol/metadata refreshes also save facts, not just the repair.
	if _, err = db.Exec(ctx, `UPDATE native_tmdb_details SET payload=payload-'classification',state='pending',next_attempt_at=now() WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	jobRaw, _ := json.Marshal(map[string]string{"detailId": id})
	if err = e.Process(ctx, jobs.Job{Payload: jobRaw}); err != nil {
		t.Fatal(err)
	}
	var version int
	if err = db.QueryRow(ctx, `SELECT (payload#>>'{classification,version}')::int FROM native_tmdb_details WHERE id=$1`, id).Scan(&version); err != nil || version != 1 {
		t.Fatal(version, err)
	}
}
func TestClassificationRefreshFailureConflictAndInvisibleIdentityDoNotOverwrite(t *testing.T) {
	for _, mode := range []string{"provider-error", "concurrent-edit", "asset-disappeared", "identity-replaced"} {
		t.Run(mode, func(t *testing.T) {
			ctx, db, id := classificationFixture(t)
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if mode == "provider-error" {
					w.WriteHeader(404)
					return
				}
				if mode == "concurrent-edit" {
					if _, e := db.Exec(ctx, `UPDATE native_tmdb_details SET payload=jsonb_set(payload,'{overview}','"另一个已提交的版本"') WHERE id=$1`, id); e != nil {
						t.Error(e)
					}
				}
				if mode == "identity-replaced" {
					if _, e := db.Exec(ctx, `UPDATE native_tmdb_details SET tmdb_id=701 WHERE id=$1`, id); e != nil {
						t.Error(e)
					}
					if _, e := db.Exec(ctx, `UPDATE media_items SET tmdb_id=701`); e != nil {
						t.Error(e)
					}
				}
				if mode == "asset-disappeared" {
					if _, e := db.Exec(ctx, `UPDATE cloud_files SET state='missing'`); e != nil {
						t.Error(e)
					}
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":700,"title":"不得覆盖","original_language":"zh","origin_country":["TW"]}`))
			}))
			defer server.Close()
			e := Engine{DB: db, Connections: fixtureConnection{tmdb.Client{Language: "zh-CN", BaseURL: server.URL, HTTP: server.Client()}}}
			out, err := e.RefreshClassification(ctx, Target{Type: "movie", TMDBID: 700})
			if err == nil || out.Changed {
				t.Fatal(out, err)
			}
			if mode != "provider-error" && !errors.Is(err, ErrClassificationConflict) {
				t.Fatal(err)
			}
			var unchanged bool
			if e := db.QueryRow(ctx, `SELECT NOT(payload?'classification') FROM native_tmdb_details WHERE id=$1`, id).Scan(&unchanged); e != nil || !unchanged {
				t.Fatal("failure wrote facts", e)
			}
			before := calls.Load()
			if _, err = e.RefreshClassification(ctx, Target{Type: "movie", TMDBID: 999}); !errors.Is(err, ErrInvalid) || calls.Load() != before {
				t.Fatal("non-library identity fetched", err)
			}
		})
	}
}
