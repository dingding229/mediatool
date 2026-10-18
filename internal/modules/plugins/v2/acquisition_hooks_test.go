package v2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/jackc/pgx/v5"
	"log/slog"
	"media-tool/internal/modules/jobs"
	dbutil "media-tool/internal/platform/database"
	"media-tool/internal/platform/trace"
	"testing"
	"time"
)

func policyFixture(t *testing.T) (Store, Lease) {
	t.Helper()
	s, l := activeFixture(t)
	metadata := contracts.Snapshot{Version: 2}
	for _, id := range []string{"resource.selection", "resource.download"} {
		metadata.Descriptors = append(metadata.Descriptors, contracts.Descriptor{Kind: "chain", ID: id, Handler: contracts.HandlerID("chain", id), RequiredScopes: []string{"events.chain." + id}})
	}
	if e := s.Publish(context.Background(), l, metadata); e != nil {
		t.Fatal(e)
	}
	if e := s.Renew(context.Background(), l, true); e != nil {
		t.Fatal(e)
	}
	return s, l
}
func policyReply(s Store, raw json.RawMessage, fail bool) <-chan error {
	done := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		for ctx.Err() == nil {
			var id string
			e := s.DB.QueryRow(ctx, `SELECT id::text FROM plugin_v2_runs WHERE kind='chain' AND state='pending' ORDER BY created_at LIMIT 1`).Scan(&id)
			if errors.Is(e, pgx.ErrNoRows) {
				time.Sleep(time.Millisecond)
				continue
			}
			if e != nil {
				done <- e
				return
			}
			state := RunSucceeded
			var cipher []byte
			if fail {
				state = RunFailed
			} else {
				cipher, e = s.seal("call-result", id, raw)
				if e != nil {
					done <- e
					return
				}
			}
			_, e = s.DB.Exec(ctx, `UPDATE plugin_v2_runs SET state=$2,result_ciphertext=$3,finished_at=now() WHERE id=$1`, id, state, cipher)
			done <- e
			return
		}
		done <- ctx.Err()
	}()
	return done
}
func TestResourcePoliciesNarrowFallbackTimeoutAndCommitFence(t *testing.T) {
	s, l := policyFixture(t)
	ctx := context.Background()
	a, b, c := jobs.UUID(), jobs.UUID(), jobs.UUID()
	q := contracts.ResourceSelectionRequest{RunID: c, Candidates: []contracts.ResourceCandidate{{ID: a, AccountID: c}, {ID: b, AccountID: c}}}
	for _, tc := range []struct {
		name string
		raw  string
		fail bool
		want int
	}{
		{"filter", fmt.Sprintf(`{"ids":[%q]}`, b), false, 1},
		{"unknown source", fmt.Sprintf(`{"ids":[%q]}`, c), false, 2},
		{"duplicate", fmt.Sprintf(`{"ids":[%q,%q]}`, a, a), false, 2},
		{"fields", fmt.Sprintf(`{"ids":[%q],"url":"https://private.invalid"}`, a), false, 2},
		{"error", `{}`, true, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			done := policyReply(s, json.RawMessage(tc.raw), tc.fail)
			guarded, ids, e := s.SelectResources(ctx, q)
			if e != nil || len(ids) != tc.want {
				t.Fatal(ids, e)
			}
			if e = <-done; e != nil {
				t.Fatal(e)
			}
			if e = dbutil.CheckPrepared(guarded, s.DB); e != nil {
				t.Fatal("read fence", e)
			}
		})
	}
	var logs bytes.Buffer
	logged := trace.With(ctx, slog.New(slog.NewJSONHandler(&logs, nil)))
	before := time.Now()
	_, ids, e := s.SelectResources(logged, q)
	if e != nil || len(ids) != 2 || time.Since(before) > time.Second || !bytes.Contains(logs.Bytes(), []byte("timeout_or_failed")) {
		t.Fatal("timeout did not safely fall back", ids, e, logs.String())
	}
	done := policyReply(s, json.RawMessage(`{"allow":false,"cleanupAfterImport":false}`), false)
	if _, _, e = s.PrepareDownload(ctx, contracts.ResourceDownloadRequest{Kind: "managed", ResourceID: a, DownloaderID: c, CleanupAfterImport: true}); !errors.Is(e, ErrResourceRejected) {
		t.Fatal("veto ignored", e)
	}
	if e = <-done; e != nil {
		t.Fatal(e)
	}
	done = policyReply(s, json.RawMessage(`{"allow":true,"cleanupAfterImport":true}`), false)
	if _, cleanup, e := s.PrepareDownload(ctx, contracts.ResourceDownloadRequest{Kind: "manual", ResourceID: a, DownloaderID: c}); e != nil || cleanup {
		t.Fatal("cleanup widened", cleanup, e)
	}
	if e = <-done; e != nil {
		t.Fatal(e)
	}
	// A prepared result cannot commit after either direct scope revocation or a
	// generation change, even with a staged business fact already in the tx.
	for _, mode := range []string{"scope", "generation"} {
		t.Run(mode, func(t *testing.T) {
			if mode == "generation" {
				if _, e = s.DB.Exec(ctx, `INSERT INTO plugin_v2_grants(instance_id,scope,fingerprint)VALUES($1,'events.chain.resource.selection',$2)`, l.ID, l.Fingerprint); e != nil {
					t.Fatal(e)
				}
			}
			done = policyReply(s, json.RawMessage(fmt.Sprintf(`{"ids":[%q]}`, a)), false)
			prepared, selected, e := s.SelectResources(ctx, q)
			if e != nil || len(selected) != 1 {
				t.Fatal(selected, e)
			}
			if e = <-done; e != nil {
				t.Fatal(e)
			}
			tx, e := s.DB.Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			defer rollback(ctx, tx)
			if _, e = tx.Exec(ctx, `INSERT INTO outbox_events(id,event_type,aggregate_id,payload)VALUES($1,'test.policy',$1,'{}')`, jobs.UUID()); e != nil {
				t.Fatal(e)
			}
			if mode == "scope" {
				_, e = s.DB.Exec(ctx, `DELETE FROM plugin_v2_grants WHERE instance_id=$1 AND scope='events.chain.resource.selection'`, l.ID)
			} else {
				_, e = s.DB.Exec(ctx, `UPDATE plugin_v2_instances SET generation=$2 WHERE id=$1`, l.ID, jobs.UUID())
			}
			if e != nil {
				t.Fatal(e)
			}
			if e = dbutil.Commit(prepared, tx); !errors.Is(e, ErrConflict) {
				t.Fatal("revocation crossed commit", e)
			}
			if e = tx.Rollback(ctx); e != nil {
				t.Fatal(e)
			}
			var n int
			if e = s.DB.QueryRow(ctx, `SELECT count(*)FROM outbox_events WHERE event_type='test.policy'`).Scan(&n); e != nil || n != 0 {
				t.Fatal("staged write escaped", n, e)
			}
			// New calls are denied/skipped, not an authorization cache hit.
			_, unchanged, e := s.SelectResources(ctx, q)
			if e != nil || len(unchanged) != 2 {
				t.Fatal(unchanged, e)
			}
		})
	}
}
func TestResourceNoPluginAndMailboxSchemaValidation(t *testing.T) {
	s := fixture(t)
	a, b := jobs.UUID(), jobs.UUID()
	q := contracts.ResourceSelectionRequest{RunID: a, Candidates: []contracts.ResourceCandidate{{ID: b, AccountID: a}}}
	ctx, ids, e := s.SelectResources(context.Background(), q)
	if e != nil || len(ids) != 1 || ids[0] != b {
		t.Fatal(ids, e)
	}
	if e = dbutil.CheckPrepared(ctx, nil); e != nil {
		t.Fatal("no-plugin path tried SQL", e)
	}
	if _, cleanup, e := s.PrepareDownload(context.Background(), contracts.ResourceDownloadRequest{Kind: "managed", ResourceID: a, DownloaderID: b, CleanupAfterImport: true}); e != nil || !cleanup {
		t.Fatal(cleanup, e)
	}
	raw, _ := json.Marshal(q)
	args := json.RawMessage(fmt.Sprintf(`{"type":"resource.selection","schemaVersion":1,"payload":%s}`, raw))
	if !validExtensionResult("chain", "resource.selection", args, json.RawMessage(fmt.Sprintf(`{"ids":[%q]}`, b))) {
		t.Fatal("valid result refused")
	}
	for _, r := range []string{fmt.Sprintf(`{"ids":[%q]}`, a), `{"ids":null}`, `{"ids":[],"ignoreHR":true}`} {
		if validExtensionResult("chain", "resource.selection", args, json.RawMessage(r)) {
			t.Fatal("invalid result accepted", r)
		}
	}
}

func TestDownloadPolicyRequiresExplicitTypedBooleans(t *testing.T) {
	for _, raw := range []string{`{}`, `{"allow":null,"cleanupAfterImport":false}`, `{"allow":true}`, `{"allow":true,"cleanupAfterImport":0}`, `{"allow":true,"cleanupAfterImport":false,"skipHR":true}`} {
		var v contracts.ResourceDownloadResult
		if decodeDownloadPolicy(json.RawMessage(raw), &v) == nil {
			t.Fatal("malformed result accepted", raw)
		}
	}
	var v contracts.ResourceDownloadResult
	if e := decodeDownloadPolicy(json.RawMessage(`{"allow":false,"cleanupAfterImport":false}`), &v); e != nil || v.Allow {
		t.Fatal(v, e)
	}
}
