package v2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"log/slog"
	"media-tool/internal/modules/jobs"
	dbutil "media-tool/internal/platform/database"
	"media-tool/internal/platform/trace"
	"testing"
	"time"
)

func organizingFixture(t *testing.T) (Store, Lease) {
	t.Helper()
	s, l := activeFixture(t)
	ctx := context.Background()
	meta := contracts.Snapshot{Version: 2}
	for _, typ := range []string{"transfer.rename", "transfer.rename.build", "transfer.intercept", "transfer.overwrite.check", "storage.operation"} {
		meta.Descriptors = append(meta.Descriptors, contracts.Descriptor{Kind: "chain", ID: typ, Handler: contracts.HandlerID("chain", typ), RequiredScopes: []string{"events.chain." + typ}})
	}
	if e := s.Publish(ctx, l, meta); e != nil {
		t.Fatal(e)
	}
	if e := s.Renew(ctx, l, true); e != nil {
		t.Fatal(e)
	}
	return s, l
}
func TestOrganizingPoliciesNarrowTimeoutRevocationAndNoWorker(t *testing.T) {
	s, l := organizingFixture(t)
	ctx := context.Background()
	id := jobs.UUID()
	q := contracts.TransferPlan{OperationID: id, Paths: []string{"Movies/a.strm"}}
	validate := func(_ context.Context, paths []string) bool { return len(paths) == 1 && paths[0] != "escape/a.strm" }
	for _, typ := range []contracts.ChainEventType{contracts.ChainTransferRename, contracts.ChainTransferRenameBuild} {
		for _, tc := range []struct{ raw, want string }{
			{`{"paths":["Movies/b.strm"]}`, "Movies/b.strm"}, {`{"paths":["../outside.strm"]}`, "Movies/a.strm"}, {`{"paths":["/tmp/a.strm"]}`, "Movies/a.strm"}, {`{"paths":["escape/a.strm"]}`, "Movies/a.strm"}, {`{"paths":["Movies/a.strm"],"body":"http://evil"}`, "Movies/a.strm"},
		} {
			done := policyReply(s, json.RawMessage(tc.raw), false)
			guarded, paths, e := s.TransferPaths(ctx, typ, q, validate)
			if e != nil || paths[0] != tc.want {
				t.Fatal(typ, paths, e)
			}
			if e = <-done; e != nil {
				t.Fatal(e)
			}
			if e = dbutil.CheckPrepared(guarded, s.DB); e != nil {
				t.Fatal(e)
			}
		}
	}
	for _, typ := range []contracts.ChainEventType{contracts.ChainTransferIntercept, contracts.ChainTransferOverwriteCheck} {
		for _, decision := range []string{"reject", "skip", "force_allow"} {
			done := policyReply(s, json.RawMessage(fmt.Sprintf(`{"decision":%q}`, decision)), false)
			_, e := s.TransferDecision(ctx, typ, q)
			if (decision != "force_allow") != errors.Is(e, ErrOrganizeRejected) {
				t.Fatal(typ, decision, e)
			}
			if e = <-done; e != nil {
				t.Fatal(e)
			}
		}
	}
	for _, handle := range []string{jobs.UUID(), id} {
		done := policyReply(s, json.RawMessage(fmt.Sprintf(`{"handle":%q}`, handle)), false)
		_, selected, e := s.SelectStorage(ctx, contracts.StorageSelection{Handles: []string{id}, Selected: id})
		if e != nil || selected != id {
			t.Fatal(selected, e)
		}
		if e = <-done; e != nil {
			t.Fatal(e)
		}
	}
	var logs bytes.Buffer
	logged := trace.With(ctx, slog.New(slog.NewJSONHandler(&logs, nil)))
	start := time.Now()
	_, paths, e := s.TransferPaths(logged, contracts.ChainTransferRename, q, validate)
	if e != nil || paths[0] != q.Paths[0] || time.Since(start) > time.Second || !bytes.Contains(logs.Bytes(), []byte("timeout_or_failed")) {
		t.Fatal("timeout fallback", paths, e)
	}
	// Each scope is checked at invocation and again before a prepared result commits.
	for _, typ := range []string{"transfer.rename", "transfer.rename.build", "transfer.intercept", "transfer.overwrite.check", "storage.operation"} {
		snap, d, e := s.Descriptor(ctx, l.ID, "chain", typ)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = s.DB.Exec(ctx, `DELETE FROM plugin_v2_grants WHERE instance_id=$1 AND scope=$2`, l.ID, "events.chain."+typ); e != nil {
			t.Fatal(e)
		}
		if _, _, e = s.Descriptor(ctx, l.ID, "chain", typ); e == nil {
			t.Fatal("revoked scope cached")
		}
		tx, e := s.DB.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		if e = s.fencePolicy(ctx, tx, snap, d); !errors.Is(e, ErrConflict) {
			t.Fatal("stale write", e)
		}
		if e = tx.Rollback(ctx); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = s.DB.Exec(ctx, `UPDATE plugin_v2_instances SET runtime_state='backoff' WHERE id=$1`, l.ID); e != nil {
		t.Fatal(e)
	}
	_, paths, e = s.TransferPaths(ctx, contracts.ChainTransferRename, q, validate)
	if e != nil || paths[0] != q.Paths[0] {
		t.Fatal("absent worker changed builtin", paths, e)
	}
}
