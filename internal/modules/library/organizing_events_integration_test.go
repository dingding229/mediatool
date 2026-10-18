package library

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/metadata"
	dbutil "media-tool/internal/platform/database"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOrganizingSubtitleHistoryCommitRollbackAndStableSTRM(t *testing.T) {
	f := newArchiveFixture(t)
	ctx := f.ctx
	j := Journal{DB: f.db, Vault: f.e.Connections.Repository.Vault}
	count := func(kind, id string) int {
		t.Helper()
		var n int
		if e := f.db.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE event_type=$1 AND (payload->>'operationId'=$2 OR payload->>'recordId'=$2)`, kind, id).Scan(&n); e != nil {
			t.Fatal(e)
		}
		return n
	}
	tx, e := f.db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	intent, e := j.Stage(ctx, tx, f.root, "TV/new.zh.srt", f.asset, "asset:"+f.asset, "subtitle", []byte("owned subtitle"))
	if e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	sentinel := errors.New("reject final fact commit")
	guarded := dbutil.WithCommitGuard(ctx, func(c context.Context, tx pgx.Tx) error {
		var staged int
		if e := tx.QueryRow(c, `SELECT count(*)FROM outbox_events WHERE event_type='library.subtitles.copied' AND payload->>'operationId'=$1`, intent).Scan(&staged); e != nil {
			return e
		}
		if staged != 1 || count("library.subtitles.copied", intent) != 0 {
			t.Error("copy fact visible before commit")
		}
		return sentinel
	})
	if e = j.Apply(guarded, intent); !errors.Is(e, sentinel) {
		t.Fatal(e)
	}
	if count("library.subtitles.copied", intent) != 0 {
		t.Fatal("rollback published copy")
	}
	// Simulate a crash after filesystem write; recovery confirms the same digest.
	for range 2 {
		if e = j.Apply(ctx, intent); e != nil {
			t.Fatal(e)
		}
	}
	if count("library.subtitles.copied", intent) != 1 {
		t.Fatal("missing/duplicate actual copy")
	}
	before := count("library.subtitles.copied", intent)
	if e = CopySubtitlesOutputs(ctx, f.db, f.e.Connections.Repository.Vault, t.TempDir(), "absent.mkv", f.root, []string{"TV/new.strm"}, f.asset); e != nil {
		t.Fatal(e)
	}
	if count("library.subtitles.copied", intent) != before {
		t.Fatal("no subtitle emitted fact")
	}
	id := jobs.UUID()
	var updated time.Time
	if e = f.db.QueryRow(ctx, `INSERT INTO file_batches(id,digest,state,expires_at)VALUES($1,'fixture','partial',now()+interval '1 hour') RETURNING updated_at`, id).Scan(&updated); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { f.db.Exec(context.Background(), `DELETE FROM file_batches WHERE id=$1`, id) })
	items := []HistorySelection{{Kind: "batch", ID: id, UpdatedAt: updated}}
	guarded = dbutil.WithCommitGuard(ctx, func(c context.Context, tx pgx.Tx) error {
		var n int
		if e := tx.QueryRow(c, `SELECT count(*) FROM outbox_events WHERE event_type='library.history.hidden' AND payload->>'recordId'=$1`, id).Scan(&n); e != nil {
			return e
		}
		if n != 1 || count("library.history.hidden", id) != 0 {
			t.Error("hidden fact before commit")
		}
		return sentinel
	})
	if e = f.e.HideHistory(guarded, items, true); !errors.Is(e, sentinel) {
		t.Fatal(e)
	}
	if count("library.history.hidden", id) != 0 {
		t.Fatal("rollback published hidden")
	}
	for range 2 {
		if e = f.e.HideHistory(ctx, items, true); e != nil {
			t.Fatal(e)
		}
	}
	if count("library.history.hidden", id) != 1 {
		t.Fatal("hidden event not idempotent")
	}
	if raw, e := os.ReadFile(filepath.Join(f.root, f.next)); e != nil || string(raw) != string(f.raw) {
		t.Fatal("hiding history changed STRM", e)
	}
	// A fresh output's renamed path never changes the supplied stable resolver body.
	asset := jobs.UUID()
	if _, e = f.db.Exec(ctx, `INSERT INTO media_assets(id,cloud_file_id)SELECT $1,cloud_file_id FROM media_assets WHERE id=$2`, asset, f.asset); e != nil {
		t.Fatal(e)
	}
	body := []byte("https://resolver.fixture.invalid/api/v1/cloudStorage/strm302?apiKey=PRIVATE-FIXTURE\n")
	tx, e = f.db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	ids, e := StageOutputProjections(ctx, j, tx, f.root, asset, metadata.Item{ID: 1, MediaType: "movie", Title: "Owned"}, nil, []OutputProjection{{Path: "Renamed/owned.strm", WholeFilePlayback: true}}, body, "private-ref")
	if e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	for _, id := range ids {
		if e = j.Apply(ctx, id); e != nil {
			t.Fatal(e)
		}
	}
	actual, e := os.ReadFile(filepath.Join(f.root, "Renamed/owned.strm"))
	if e != nil || string(actual) != string(body) {
		t.Fatal("resolver bytes changed", e)
	}
	if !strings.Contains(string(actual), "/api/v1/cloudStorage/strm302?") {
		t.Fatal("not stable resolver")
	}
}
func TestOrganizingPathsRejectEscapeConflictAndOwner(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if e := os.Symlink(outside, filepath.Join(root, "escape")); e != nil {
		t.Fatal(e)
	}
	for _, p := range []string{"../bad.strm", "/tmp/bad.strm", "escape/bad.strm", "a/../../bad.strm"} {
		if PathsUnderRoot(root, []string{p}) {
			t.Fatal("escape accepted", p)
		}
	}
	if !PathsUnderRoot(root, []string{"new/safe.strm"}) {
		t.Fatal("missing safe directory refused")
	}
	f := newArchiveFixture(t)
	ctx := f.ctx
	p := []OutputProjection{{Path: f.next, WholeFilePlayback: true}}
	_, unchanged, e := f.e.PrepareOutputs(ctx, f.asset, f.asset, p)
	if e != nil || len(unchanged) != 1 || unchanged[0].Path != p[0].Path {
		t.Fatal("no-plugin behavior changed", e)
	}
	other := jobs.UUID()
	if _, e = f.db.Exec(ctx, `INSERT INTO media_assets(id,cloud_file_id)SELECT $1,cloud_file_id FROM media_assets WHERE id=$2`, other, f.asset); e != nil {
		t.Fatal(e)
	}
	if !f.e.proposalOwners(ctx, f.asset, []string{f.next}) || f.e.proposalOwners(ctx, other, []string{f.next}) {
		t.Fatal("prepare owner validation")
	}
	if e = os.WriteFile(filepath.Join(f.root, "unmanaged.strm"), []byte("unrelated"), 0600); e != nil {
		t.Fatal(e)
	}
	if f.e.proposalOwners(ctx, other, []string{"unmanaged.strm"}) {
		t.Fatal("unmanaged target was accepted")
	}
	tx, e := f.db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	_, e = StageOutputProjections(ctx, Journal{DB: f.db, Vault: f.e.Connections.Repository.Vault}, tx, f.root, other, metadata.Item{ID: 1, MediaType: "movie", Title: "Owned"}, nil, p, []byte("unchanged stable resolver\n"), "ref")
	if !errors.Is(e, ErrArtifactConflict) {
		t.Fatal("foreign owner overwritten", e)
	}
}
