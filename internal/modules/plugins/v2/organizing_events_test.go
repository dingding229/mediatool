package v2

import (
	"context"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/platform/outbox"
	"strings"
	"testing"
	"time"
)

func TestOrganizingBridgeCommitRollbackDedupAndLiveScopes(t *testing.T) {
	s, l := activeFixture(t)
	ctx := context.Background()
	types := []string{"library.organize.failed", "library.subtitles.copied", "library.subtitles.failed", "library.history.hidden", "download.files.deleted", "download.task.deleted"}
	metadata := contracts.Snapshot{Version: 2}
	for _, typ := range types {
		metadata.Descriptors = append(metadata.Descriptors, contracts.Descriptor{Kind: "event", ID: typ, Handler: contracts.HandlerID("event", typ), RequiredScopes: []string{"events.subscribe." + typ}})
	}
	if e := s.Publish(ctx, l, metadata); e != nil {
		t.Fatal(e)
	}
	if e := s.Renew(ctx, l, true); e != nil {
		t.Fatal(e)
	}
	aggregate := jobs.UUID()
	for _, typ := range types {
		fact := map[string]any{"operationId": aggregate, "assetId": aggregate, "recordId": aggregate, "taskId": aggregate, "kind": "transfer", "reasonCode": "attempts_exhausted", "filesystemDeleted": false, "filesDeleted": false, "count": 1, "accountId": aggregate, "subscriptionId": aggregate, "revision": 1, "accountRevision": 1, "enabled": true, "state": "succeeded", "attemptedAt": time.Now().UTC(), "mediaType": "movie", "tmdbId": 1, "cookie": "NEVER_BRIDGE_SECRET", "downloadHint": "https://private.invalid/token"}
		for _, commit := range []bool{false, true} {
			tx, e := s.DB.Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			if e = outbox.EmitVersionTx(ctx, tx, typ, aggregate, "1", fact); e != nil {
				t.Fatal(e)
			}
			// Retrying one immutable version in the same transaction emits once.
			if e = outbox.EmitVersionTx(ctx, tx, typ, aggregate, "1", fact); e != nil {
				t.Fatal(e)
			}
			if e = s.Bridge(ctx); e != nil {
				t.Fatal(e)
			}
			var n int
			if e = s.DB.QueryRow(ctx, `SELECT count(*) FROM plugin_v2_event_journal WHERE event_type=$1`, typ).Scan(&n); e != nil || n != 0 {
				t.Fatal("uncommitted event visible", typ, n, e)
			}
			if commit {
				e = tx.Commit(ctx)
			} else {
				e = tx.Rollback(ctx)
			}
			if e != nil {
				t.Fatal(e)
			}
		}
	}
	for range 2 {
		if e := s.Bridge(ctx); e != nil {
			t.Fatal(e)
		}
		if e := s.Deliver(ctx); e != nil {
			t.Fatal(e)
		}
	}
	var n int
	if e := s.DB.QueryRow(ctx, `SELECT count(*) FROM plugin_v2_runs WHERE kind='event'`).Scan(&n); e != nil || n != 6 {
		t.Fatal("missing/duplicate deliveries", n, e)
	}
	rows, e := s.DB.Query(ctx, `SELECT event_type,payload FROM plugin_v2_event_journal`)
	if e != nil {
		t.Fatal(e)
	}
	for rows.Next() {
		var typ string
		var raw []byte
		if e = rows.Scan(&typ, &raw); e != nil {
			t.Fatal(e)
		}
		if strings.Contains(string(raw), "NEVER_BRIDGE_SECRET") || strings.Contains(string(raw), "https://") {
			t.Fatal("bridge leaked untrusted extra fields")
		}
		var v any
		switch typ {
		case "library.organize.failed":
			v = &contracts.OrganizeFailure{}
		case "library.subtitles.copied", "library.subtitles.failed":
			v = &contracts.SubtitleOutcome{}
		case "library.history.hidden":
			v = &contracts.HistoryHidden{}
		default:
			v = &contracts.DownloadDeleted{}
		}
		if e = contracts.Decode(raw, v); e != nil {
			t.Fatal("unstable typed payload", e)
		}
	}
	if e = rows.Err(); e != nil {
		t.Fatal(e)
	}
	rows.Close()
	// A versioned fact ID is NOT the business operation ID.
	operation := jobs.UUID()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = outbox.EmitVersionTx(ctx, tx, "files.succeeded", operation, "version-1", map[string]string{"operationId": operation}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.Bridge(ctx); err != nil {
		t.Fatal(err)
	}
	var actual string
	if err = s.DB.QueryRow(ctx, `SELECT payload->>'operationId' FROM plugin_v2_event_journal WHERE event_type='library.organized'`).Scan(&actual); err != nil || actual != operation {
		t.Fatal("version key replaced business identity", actual, err)
	}
	// Direct removal of any granted subscription scope is effective immediately,
	// even without a revision bump; then a revision bump also invalidates snapshots.
	for phase := range 2 {
		for _, typ := range types {
			if phase == 0 {
				if _, e = s.DB.Exec(ctx, `DELETE FROM plugin_v2_grants WHERE instance_id=$1 AND scope=$2`, l.ID, "events.subscribe."+typ); e != nil {
					t.Fatal(e)
				}
			}
			tx, e := s.DB.Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			if e = outbox.EmitVersionTx(ctx, tx, typ, aggregate, string(rune('2'+phase)), map[string]any{"operationId": aggregate, "assetId": aggregate, "recordId": aggregate, "taskId": aggregate, "kind": "transfer", "reasonCode": "attempts_exhausted", "filesystemDeleted": false, "filesDeleted": false, "count": 1, "accountId": aggregate, "revision": phase + 2}); e != nil {
				t.Fatal(e)
			}
			if e = tx.Commit(ctx); e != nil {
				t.Fatal(e)
			}
		}
		if phase == 1 {
			if _, e = s.DB.Exec(ctx, `UPDATE plugin_v2_instances SET grant_revision=grant_revision+1 WHERE id=$1`, l.ID); e != nil {
				t.Fatal(e)
			}
		}
		if e = s.Bridge(ctx); e != nil {
			t.Fatal(e)
		}
		if e = s.Deliver(ctx); e != nil {
			t.Fatal(e)
		}
		if e = s.DB.QueryRow(ctx, `SELECT count(*) FROM plugin_v2_runs WHERE kind='event'`).Scan(&n); e != nil || n != 6 {
			t.Fatal("revoked scope delivered", n, e)
		}
	}
}
