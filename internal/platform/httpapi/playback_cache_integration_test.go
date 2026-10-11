package httpapi

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/linkcache"
	"media-tool/internal/modules/storage"
	"media-tool/internal/platform/cache"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
)

func TestRuntimePlaybackCacheFollowsOfficialSelectionAndCredentialRevision(t *testing.T) {
	dsn, redisURL := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_REDIS_URL")
	if dsn == "" || redisURL == "" {
		t.Skip("isolated PostgreSQL/Redis required")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	if err = db.QueryRow(ctx, `SELECT current_database()`).Scan(&name); err != nil || name != "media_tool_test" {
		t.Fatal("isolated media_tool_test required")
	}
	rc, err := cache.Open(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	vault, _ := secrets.New(strings.Repeat("c", 64))
	m := integrations.NewManager(integrations.Repository{DB: db, Vault: vault}, config.Config{})
	defer m.Close()
	// No requests to real CD2/115; these connections only partition local fixture keys.
	cloud, err := m.Repository.Save(ctx, "", integrations.Input{Kind: "clouddrive2", Name: "cache revision fixture", Enabled: true, Settings: map[string]any{"endpoint": "http://localhost:19798", "cloudRoot": "/115/CacheRevisionFixture"}, Secrets: map[string]string{"token": "fixture-cloud-token"}})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DELETE FROM integrations WHERE id=$1`, cloud.ID)
	official, err := m.Repository.Save(ctx, "", integrations.Input{Kind: "open115", Name: "cache official fixture", Settings: map[string]any{"appId": "12345", "cloudMountPrefix": "/115"}, Secrets: map[string]string{"accessToken": "fixture-access", "refreshToken": "fixture-refresh"}})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DELETE FROM integrations WHERE id=$1`, official.ID)
	a := &API{Deps: Dependencies{DB: db, Integrations: m}}
	var calls atomic.Int32
	source := resolverFunc(func(context.Context, storage.FileRef) (storage.DownloadLink, error) {
		return storage.DownloadLink{URL: fmt.Sprintf("https://cdn.test.fixture/%d", calls.Add(1)), CacheUntil: time.Now().Add(time.Minute)}, nil
	})
	c := &linkcache.Cached{Source: source, Redis: rc, Version: a.playbackCacheVersion}
	ref := storage.FileRef{Path: "/115/CacheRevisionFixture/owned-" + cloud.ID + ".mkv", UserAgent: "fixture", ClientIP: "203.0.113.10"}
	first, err := c.Resolve(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	again, err := c.Resolve(ctx, ref)
	if err != nil || again.URL != first.URL || calls.Load() != 1 {
		t.Fatal("initial cache miss", err)
	}
	c.Invalidate(ctx, ref)
	first, err = c.Resolve(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	official, err = m.Repository.Save(ctx, official.ID, integrations.Input{Kind: official.Kind, Name: official.Name, Enabled: true, Settings: official.Settings, Revision: official.Revision})
	if err != nil {
		t.Fatal(err)
	}
	enabled, err := c.Resolve(ctx, ref)
	if err != nil || enabled.URL == first.URL {
		t.Fatal("official enable reused CD2 cache", err)
	}
	version, err := a.playbackCacheVersion(ctx, ref)
	if err != nil || !strings.HasPrefix(version, "open115:"+official.ID+":") {
		t.Fatal("selected provider revision not wired", err)
	}
	c.Invalidate(ctx, ref)
	enabled, err = c.Resolve(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	official, err = m.Repository.Save(ctx, official.ID, integrations.Input{Kind: official.Kind, Name: official.Name, Enabled: true, Settings: official.Settings, Revision: official.Revision, Secrets: map[string]string{"accessToken": "fixture-new-access"}})
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := c.Resolve(ctx, ref)
	if err != nil || rotated.URL == enabled.URL {
		t.Fatal("official credential revision reused cache", err)
	}
	c.Invalidate(ctx, ref)
	_, err = m.Repository.Save(ctx, official.ID, integrations.Input{Kind: official.Kind, Name: official.Name, Enabled: false, Settings: official.Settings, Revision: official.Revision})
	if err != nil {
		t.Fatal(err)
	}
	disabled, err := c.Resolve(ctx, ref)
	if err != nil || disabled.URL != first.URL {
		t.Fatal("official disable retained official URL", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = c.Resolve(cancelled, ref); err == nil {
		t.Fatal("failed revision read reused cached delivery")
	}
	c.Invalidate(ctx, ref)
	if env, err := (&API{}).playbackCacheVersion(ctx, ref); err != nil || env != "env" {
		t.Fatal("environment fallback changed")
	}
}
