package v2

import (
	"context"
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/platform/outbox"
	"strings"
	"testing"
)

func TestDiscoveryFactsCommitRollbackTypedProjectionAndLiveRevoke(t *testing.T) {
	s, l := activeFixture(t)
	ctx := context.Background()
	types := []string{"metadata.scraped", "subscription.coverage.completed"}
	meta := contracts.Snapshot{Version: 2}
	for _, typ := range types {
		meta.Descriptors = append(meta.Descriptors, contracts.Descriptor{Kind: "event", ID: typ, Handler: contracts.HandlerID("event", typ), RequiredScopes: []string{"events.subscribe." + typ}})
	}
	if e := s.Publish(ctx, l, meta); e != nil {
		t.Fatal(e)
	}
	if e := s.Renew(ctx, l, true); e != nil {
		t.Fatal(e)
	}
	id := jobs.UUID()
	for _, typ := range types {
		for _, commit := range []bool{false, true} {
			tx, e := s.DB.Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			fact := map[string]any{"objectId": id, "kind": "asset", "tmdbId": 1, "mediaType": "series", "revision": "1", "subscriptionId": id, "season": 1, "expected": []int{1}, "owned": []int{1}, "complete": true, "state": "complete", "evidence": "tmdb_season_exposable_assets", "observedAt": "2026-10-18T00:00:00Z", "token": "NEVER_BRIDGE_SECRET", "posterUrl": "https://private.invalid/secret", "streamUrl": "https://cdn.invalid/token"}
			if typ == "subscription.coverage.completed" {
				fact["revision"] = 1
			}
			if e = outbox.EmitVersionTx(ctx, tx, typ, id, "1", fact); e != nil {
				t.Fatal(e)
			}
			if e = s.Bridge(ctx); e != nil {
				t.Fatal(e)
			}
			var n int
			if e = s.DB.QueryRow(ctx, `SELECT count(*) FROM plugin_v2_event_journal WHERE event_type=$1`, typ).Scan(&n); e != nil || n != 0 {
				t.Fatal("precommit broadcast", n, e)
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
	if e := s.DB.QueryRow(ctx, `SELECT count(*) FROM plugin_v2_runs WHERE kind='event'`).Scan(&n); e != nil || n != 2 {
		t.Fatal(n, e)
	}
	rows, e := s.DB.Query(ctx, `SELECT event_type,payload FROM plugin_v2_event_journal`)
	if e != nil {
		t.Fatal(e)
	}
	defer rows.Close()
	for rows.Next() {
		var typ string
		var raw []byte
		if e = rows.Scan(&typ, &raw); e != nil {
			t.Fatal(e)
		}
		if strings.Contains(string(raw), "SECRET") || strings.Contains(string(raw), "https:") {
			t.Fatal("secret escaped whitelist")
		}
		var target any = &contracts.MetadataScraped{}
		if typ == types[1] {
			target = &contracts.CoverageObservation{}
		}
		if e = contracts.Decode(raw, target); e != nil {
			t.Fatal(e)
		}
	}
	if e = rows.Err(); e != nil {
		t.Fatal(e)
	}
	rows.Close()
	for _, typ := range types {
		if _, e = s.DB.Exec(ctx, `DELETE FROM plugin_v2_grants WHERE instance_id=$1 AND scope=$2`, l.ID, "events.subscribe."+typ); e != nil {
			t.Fatal(e)
		}
		tx, e := s.DB.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		if e = outbox.EmitVersionTx(ctx, tx, typ, id, "2", json.RawMessage(`{}`)); e != nil {
			t.Fatal(e)
		}
		if e = tx.Commit(ctx); e != nil {
			t.Fatal(e)
		}
	}
	if e = s.Bridge(ctx); e != nil {
		t.Fatal(e)
	}
	if e = s.Deliver(ctx); e != nil {
		t.Fatal(e)
	}
	if e = s.DB.QueryRow(ctx, `SELECT count(*) FROM plugin_v2_runs WHERE kind='event'`).Scan(&n); e != nil || n != 2 {
		t.Fatal("revoked broadcast delivered", n, e)
	}
}
