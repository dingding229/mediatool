package migrations_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/mediaprobe/linksource"
	"media-tool/internal/modules/playbackgrants"
	"media-tool/internal/modules/storage"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/secrets"
)

type probeTransport func(*http.Request) (*http.Response, error)

func (f probeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type probeLinkFixture struct {
	movieFixture
	asset                     movieAsset
	m                         *integrations.Manager
	root, output, body, cloud string
	logs                      *bytes.Buffer
}

func newProbeLinkFixture(t *testing.T) probeLinkFixture {
	t.Helper()
	f := newMovieFixture(t)
	// This fixture invokes current integration repository code, not just the
	// schema-58 catalog. Apply the real release schema in its disposable schema.
	if _, err := f.provider.UpTo(f.ctx, 93); err != nil {
		t.Fatal("migrate probe fixture to schema 93:", err)
	}
	if version, err := f.provider.GetDBVersion(f.ctx); err != nil || version != 93 {
		t.Fatal("probe fixture schema", version, err)
	}
	media := f.media(t, 220001, "Synthetic probe", "2026", `{}`)
	a := f.asset(t, media, "g:test-a", time.Now(), "mkv")
	cloud := "/115/synthetic.mkv"
	f.exec(t, `UPDATE cloud_files SET full_path=$2,pick_code='syntheticpickcode',size_bytes=4096 WHERE id=$1`, a.file, cloud)
	vault, err := secrets.New(strings.Repeat("c", 64))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	cfg := config.Config{PublicURL: "https://resolver.example", STRMOutputDir: root, STRMAllowedRoots: []string{"/115"}, PlaybackAllowedHosts: []string{"8.8.8.8"}}
	m := integrations.NewManager(integrations.Repository{DB: f.pool, Vault: vault}, cfg)
	t.Cleanup(m.Close)
	if _, err := m.Repository.Save(f.ctx, "", integrations.Input{Kind: "open115", Name: "Synthetic probe client", Enabled: true, Settings: map[string]any{"appId": "12345", "cloudMountPrefix": "/115"}, Secrets: map[string]string{"accessToken": "SYNTHETIC_ACCESS", "refreshToken": "SYNTHETIC_REFRESH"}}); err != nil {
		t.Fatal(err)
	}
	grant := playbackgrants.Service{DB: f.pool, Vault: vault, PublicURL: cfg.PublicURL}
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(f.ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Error(err)
		}
	}()
	body, _, err := grant.Prepare(f.ctx, tx, a.asset, cloud)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	body += "\n"
	output := "Synthetic/" + a.asset + ".strm"
	sum := sha256.Sum256([]byte(body))
	f.exec(t, `INSERT INTO managed_media_artifacts(relative_path,asset_id,artifact_kind,content_digest,owner_scope) VALUES($1,$2,'strm',$3,$4)`, output, a.asset, sum[:], "asset:"+a.asset)
	if err := os.Mkdir(filepath.Join(root, "Synthetic"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, output), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return probeLinkFixture{f, a, m, root, output, body, cloud, &bytes.Buffer{}}
}
func (f probeLinkFixture) source(t *testing.T) *linksource.Source {
	t.Helper()
	s, err := linksource.NewForManager(f.m, linksource.Config{UserAgent: "SyntheticProbe/22"}, slog.New(slog.NewJSONHandler(f.logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func probeFailure(t *testing.T, err error, class linksource.Class, reason linksource.Reason) {
	t.Helper()
	var failure *linksource.Failure
	if !errors.As(err, &failure) || failure.Class != class || failure.Reason != reason {
		t.Fatalf("want %s/%s got %v", class, reason, err)
	}
}
func TestNativeProbeLinkFactoryUsesActualSTRMGrantAndUncachedFake115(t *testing.T) {
	f := newProbeLinkFixture(t)
	calls := 0
	// All outbound requests terminate HERE: never a real 115/CDN connection.
	f.m.OutboundHTTP = &http.Client{Transport: probeTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("Authorization") != "Bearer SYNTHETIC_ACCESS" {
			t.Error("missing synthetic access token")
		}
		if err := r.ParseMultipartForm(8192); err != nil {
			t.Error(err)
		}
		defer func() {
			if r.MultipartForm != nil {
				if err := r.MultipartForm.RemoveAll(); err != nil {
					t.Error(err)
				}
			}
		}()
		body := ""
		switch r.URL.Path {
		case "/open/folder/get_info":
			if r.FormValue("path") != "/synthetic.mkv" {
				t.Error("wrong verified path")
			}
			body = `{"state":true,"code":0,"data":{"file_id":"22","file_category":"1","file_name":"synthetic.mkv","size_byte":4096,"pick_code":"syntheticpickcode"}}`
		case "/open/ufile/downurl":
			if r.UserAgent() != "SyntheticProbe/22" {
				t.Error("UA mismatch at 115")
			}
			body = `{"state":true,"code":0,"data":{"22":{"file_name":"synthetic.mkv","file_size":4096,"pick_code":"syntheticpickcode","url":{"url":"https://8.8.8.8/synthetic?SIGN=NEVER_PERSIST"}}}}`
		default:
			t.Error("unexpected upstream endpoint")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	s := f.source(t)
	jobsBefore := f.integer(t, `SELECT count(*) FROM jobs`)
	for range 2 {
		ticket, err := s.Fetch(f.ctx, f.asset.asset)
		if err != nil {
			t.Fatal(err)
		}
		link, ua, err := ticket.Claim()
		if err != nil || ua != "SyntheticProbe/22" || link != "https://8.8.8.8/synthetic?SIGN=NEVER_PERSIST" {
			t.Fatal("consumer UA differs from 115 request")
		}
	}
	if calls != 4 {
		t.Fatal("cached link or duplicate upstream request", calls)
	}
	if f.integer(t, `SELECT count(*) FROM media_probe_results`) != 0 || f.integer(t, `SELECT count(*) FROM jobs`) != jobsBefore {
		t.Fatal("T22 created tasks/state")
	}
	stored, err := os.ReadFile(filepath.Join(f.root, f.output))
	if err != nil || string(stored) != f.body {
		t.Fatal("original STRM changed")
	}
	for _, secret := range []string{"NEVER_PERSIST", "apiKey=", "SYNTHETIC_ACCESS", "SYNTHETIC_REFRESH", f.cloud, "syntheticpickcode"} {
		if strings.Contains(f.logs.String(), secret) {
			t.Fatal("log leak")
		}
	}
	if !strings.Contains(f.logs.String(), "official_http_download") {
		t.Fatal("R1-F observation not reused")
	}
}
func TestNativeProbeLinkExposablePredicateAndMismatch(t *testing.T) {
	f := newProbeLinkFixture(t)
	// Forbidden to perform any real networking even if a negative test regresses.
	calls := 0
	f.m.OutboundHTTP = &http.Client{Transport: probeTransport(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("unexpected fake transport call")
	})}
	repo := linksource.SQLRepository{DB: f.pool}
	s := f.source(t)
	for _, state := range []string{"missing", "recycled"} {
		f.exec(t, `UPDATE cloud_files SET state=$2 WHERE id=$1`, f.asset.file, state)
		_, err := repo.Load(f.ctx, f.asset.asset)
		probeFailure(t, err, linksource.Permanent, linksource.NotExposable)
	}
	f.exec(t, `UPDATE cloud_files SET state='present' WHERE id=$1`, f.asset.file)
	f.exec(t, `UPDATE media_asset_bindings SET state='pending' WHERE asset_id=$1`, f.asset.asset)
	_, err := repo.Load(f.ctx, f.asset.asset)
	probeFailure(t, err, linksource.Permanent, linksource.NotExposable)
	f.exec(t, `UPDATE media_asset_bindings SET state='ready' WHERE asset_id=$1`, f.asset.asset)
	f.exec(t, `UPDATE strm_entries SET output_path='invalid.txt' WHERE asset_id=$1`, f.asset.asset)
	_, err = repo.Load(f.ctx, f.asset.asset)
	probeFailure(t, err, linksource.Permanent, linksource.NotExposable)
	f.exec(t, `UPDATE strm_entries SET output_path=$2 WHERE asset_id=$1`, f.asset.asset, f.output)
	e, err := repo.Load(f.ctx, f.asset.asset)
	if err != nil || e.AssetID != f.asset.asset || e.Path != f.cloud || e.Output != f.output || len(e.Digest) != 32 {
		t.Fatal("evidence load")
	}
	f.exec(t, `UPDATE cloud_files SET full_path='/115/replaced.mkv' WHERE id=$1`, f.asset.file)
	_, err = s.Fetch(f.ctx, f.asset.asset)
	probeFailure(t, err, linksource.Manual, linksource.Mismatch)
	f.exec(t, `UPDATE cloud_files SET full_path=$2 WHERE id=$1`, f.asset.file, f.cloud)
	if err := os.WriteFile(filepath.Join(f.root, f.output), []byte(f.body+"changed"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err = s.Fetch(f.ctx, f.asset.asset)
	probeFailure(t, err, linksource.Manual, linksource.Mismatch)
	if err := os.Remove(filepath.Join(f.root, f.output)); err != nil {
		t.Fatal(err)
	}
	_, err = s.Fetch(f.ctx, f.asset.asset)
	probeFailure(t, err, linksource.Permanent, linksource.FileMissing)
	if err := os.WriteFile(filepath.Join(f.root, f.output), []byte(f.body), 0600); err != nil {
		t.Fatal(err)
	}
	f.exec(t, `UPDATE playback_grants SET revoked_at=now() WHERE asset_id=$1`, f.asset.asset)
	_, err = s.Fetch(f.ctx, f.asset.asset)
	probeFailure(t, err, linksource.Manual, linksource.Authorization)
	f.exec(t, `UPDATE playback_grants SET revoked_at=NULL WHERE asset_id=$1`, f.asset.asset)
	f.exec(t, `DELETE FROM managed_media_artifacts WHERE asset_id=$1`, f.asset.asset)
	_, err = s.Fetch(f.ctx, f.asset.asset)
	probeFailure(t, err, linksource.Manual, linksource.Mismatch)
	f.exec(t, `DELETE FROM strm_entries WHERE asset_id=$1`, f.asset.asset)
	_, err = s.Fetch(f.ctx, f.asset.asset)
	probeFailure(t, err, linksource.Permanent, linksource.NotExposable)
	if calls != 0 {
		t.Fatal("invalid asset reached upstream")
	}
	// No credentials embedded in the small classified errors.
	if strings.Contains(err.Error(), "apiKey") || strings.Contains(err.Error(), f.cloud) {
		t.Fatal("error leak")
	}
}
func TestNativeProbeGrantDatabaseFailureIsRetryableNotRevoked(t *testing.T) {
	f := newProbeLinkFixture(t)
	service := playbackgrants.Service{DB: f.pool}
	ctx, cancel := context.WithCancel(f.ctx)
	cancel()
	if _, err := service.Authorize(ctx, f.asset.asset, "synthetic"); !errors.Is(err, context.Canceled) {
		t.Fatal("cancel mistaken for revoked grant", err)
	}
	f.pool.Close()
	if _, err := service.Authorize(f.ctx, f.asset.asset, "synthetic"); !errors.Is(err, storage.ErrUnavailable) || errors.Is(err, playbackgrants.ErrDenied) {
		t.Fatal("DB outage mistaken for revoked grant", err)
	}
}
