package onlineupdate

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"media-tool/internal/modules/updatecheck"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestFreshReleaseHubMismatchIsAuditedBeforeDocker(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("media_tool_test")
	}
	c, e := pgxpool.ParseConfig(dsn)
	if e != nil || c.ConnConfig.Database != "media_tool_test" || os.Getenv("MEDIA_TOOL_TEST_LOCKED") != "1" {
		t.Fatal("serialized test DB only")
	}
	base, e := pgxpool.NewWithConfig(t.Context(), c)
	if e != nil {
		t.Fatal(e)
	}
	defer base.Close()
	id, _ := newID()
	schema := "f8b_" + id
	if _, e = base.Exec(t.Context(), `CREATE SCHEMA `+pgx.Identifier{schema}.Sanitize()); e != nil {
		t.Fatal(e)
	}
	defer base.Exec(context.Background(), `DROP SCHEMA `+pgx.Identifier{schema}.Sanitize()+` CASCADE`)
	c.ConnConfig.RuntimeParams["search_path"] = schema
	db, e := pgxpool.NewWithConfig(t.Context(), c)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if _, e = db.Exec(t.Context(), `CREATE TABLE audit_logs(actor text,action text,details jsonb)`); e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(t.Context(), `CREATE TABLE app_settings(LIKE public.app_settings INCLUDING ALL);INSERT INTO app_settings(key,value)VALUES('online_update.execution','false'::jsonb)`); e != nil {
		t.Fatal(e)
	}
	checker := updatecheck.New(db, nil)
	m := updatecheck.Metadata{Version: "0.28.1", Commit: strings.Repeat("a", 40), Digest: "sha256:" + strings.Repeat("b", 64), MinimumSchema: 79, Schema: 79, Platform: "linux/amd64", Repository: updatecheck.Repository}
	body, e := updatecheck.ReleaseBody("fixture", m)
	if e != nil {
		t.Fatal(e)
	}
	calls := 0
	bad := false
	checker.Client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		var v any
		if r.URL.String() == "https://api.github.com/repos/dingding229/mediatool/releases/latest" {
			v = map[string]any{"tag_name": "v0.28.1", "body": body, "published_at": time.Now().Add(-time.Hour)}
		} else if r.URL.String() == "https://hub.docker.com/v2/namespaces/dingding229/repositories/mediatool/tags/v0.28.1" {
			digest := m.Digest
			if bad {
				digest = "sha256:" + strings.Repeat("c", 64)
			}
			v = map[string]any{"name": "v0.28.1", "digest": digest, "images": []any{map[string]any{"architecture": "amd64", "os": "linux", "digest": m.Digest}}}
		} else {
			t.Fatalf("unapproved URL %s", r.URL)
		}
		b, _ := json.Marshal(v)
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(b)))}, nil
	})
	if _, e = checker.VerifyFresh(t.Context()); e != nil {
		t.Fatal(e)
	}
	bad = true
	s := New(db, checker)
	s.Root = t.TempDir()
	if _, e = s.Start(t.Context(), "fixture-admin", "0.28.1", m.Digest); e == nil {
		t.Fatal("mutable tag mismatch accepted")
	}
	if calls != 4 {
		t.Fatal("execution reused check cache", calls)
	}
	var actor, action, reason string
	if e = db.QueryRow(t.Context(), `SELECT actor,action,details->>'reason' FROM audit_logs`).Scan(&actor, &action, &reason); e != nil || actor != "fixture-admin" || action != "online_update.rejected" || reason != "release_hub_mismatch_or_unavailable" {
		t.Fatal(e, actor, action, reason)
	}
}
