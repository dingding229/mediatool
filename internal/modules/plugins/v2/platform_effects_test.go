package v2

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/jackc/pgx/v5"
	"media-tool/internal/modules/jobs"
	"testing"
	"time"
)

func effectFixture(t *testing.T) (Store, Lease) {
	t.Helper()
	s, l := activeFixture(t)
	meta := contracts.Snapshot{Version: 2}
	for _, typ := range []string{"notifications.channel", "mediaserver.refresh"} {
		meta.Descriptors = append(meta.Descriptors, contracts.Descriptor{Kind: "module", ID: typ, Handler: contracts.HandlerID("module", typ), RequiredScopes: []string{"module." + typ}})
	}
	if e := s.Publish(context.Background(), l, meta); e != nil {
		t.Fatal(e)
	}
	if e := s.Renew(context.Background(), l, true); e != nil {
		t.Fatal(e)
	}
	return s, l
}
func effectReply(s Store, states ...string) <-chan error {
	done := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		for _, state := range states {
			handled := false
			for ctx.Err() == nil {
				var id string
				e := s.DB.QueryRow(ctx, `SELECT id::text FROM plugin_v2_runs WHERE kind='module' AND state='pending' ORDER BY created_at LIMIT 1`).Scan(&id)
				if errors.Is(e, pgx.ErrNoRows) {
					time.Sleep(time.Millisecond)
					continue
				}
				if e != nil {
					done <- e
					return
				}
				raw, _ := json.Marshal(contracts.EffectReceipt{State: state})
				cipher, e := s.seal("call-result", id, raw)
				if e == nil {
					_, e = s.DB.Exec(ctx, `UPDATE plugin_v2_runs SET state='succeeded',result_ciphertext=$2,finished_at=now() WHERE id=$1`, id, cipher)
				}
				if e != nil {
					done <- e
					return
				}
				handled = true
				break
			}
			if !handled {
				done <- ctx.Err()
				return
			}
		}
		done <- nil
	}()
	return done
}
func TestEffectReceiptNoDuplicateNoUnknownFallbackAndLiveScopes(t *testing.T) {
	s, l := effectFixture(t)
	ctx := context.Background()
	q := contracts.EffectRequest{RequestID: jobs.UUID(), ObjectID: jobs.UUID(), Kind: "notification"}
	done := effectReply(s, "ready", "sent")
	got, e := s.ExternalEffect(ctx, "notifications.channel", q)
	if e != nil || got.State != "sent" {
		t.Fatal(got, e)
	}
	if e = <-done; e != nil {
		t.Fatal(e)
	}
	for range 2 {
		got, e = s.ExternalEffect(ctx, "notifications.channel", q)
		if e != nil || got.State != "sent" {
			t.Fatal("duplicate", got, e)
		}
	}
	q.RequestID = jobs.UUID()
	done = effectReply(s, "ready", "unknown")
	got, e = s.ExternalEffect(ctx, "notifications.channel", q)
	if !errors.Is(e, ErrEffectUnknown) || got.State != "unknown" {
		t.Fatal(got, e)
	}
	if e = <-done; e != nil {
		t.Fatal(e)
	}
	if _, e = s.DB.Exec(ctx, `DELETE FROM plugin_v2_grants WHERE instance_id=$1 AND scope='module.notifications.channel'`, l.ID); e != nil {
		t.Fatal(e)
	}
	got, e = s.ExternalEffect(ctx, "notifications.channel", q)
	if !errors.Is(e, ErrEffectUnknown) || got.State != "unknown" {
		t.Fatal("revocation allowed a second sender", got, e)
	}
	q.RequestID = jobs.UUID()
	got, e = s.ExternalEffect(ctx, "notifications.channel", q)
	if e != nil || got.State != "not_started" {
		t.Fatal("missing scope must not dispatch", got, e)
	}
	var n int
	if e = s.DB.QueryRow(ctx, `SELECT count(*) FROM plugin_v2_runs WHERE kind='module'`).Scan(&n); e != nil || n != 4 {
		t.Fatal(n, e)
	}
}
func TestBuiltinRefreshReceiptFencesUnknownAndRepeatedJob(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	q := contracts.EffectRequest{RequestID: jobs.UUID(), ObjectID: jobs.UUID(), Kind: "asset"}
	calls := 0
	call := func(context.Context) error { calls++; return nil }
	if e := s.BuiltinEffect(ctx, "mediaserver.refresh", q, call); e != nil {
		t.Fatal(e)
	}
	if e := s.BuiltinEffect(ctx, "mediaserver.refresh", q, call); e != nil || calls != 1 {
		t.Fatal("duplicate refresh", calls, e)
	}
	q.RequestID = jobs.UUID()
	fail := func(context.Context) error { calls++; return errors.New("network outcome unknown") }
	if e := s.BuiltinEffect(ctx, "mediaserver.refresh", q, fail); !errors.Is(e, ErrEffectUnknown) {
		t.Fatal(e)
	}
	if e := s.BuiltinEffect(ctx, "mediaserver.refresh", q, call); !errors.Is(e, ErrEffectUnknown) || calls != 2 {
		t.Fatal("unknown refreshed again", calls, e)
	}
	got, e := s.ExternalEffect(ctx, "mediaserver.refresh", q)
	if !errors.Is(e, ErrEffectUnknown) || got.State != "unknown" {
		t.Fatal("absent provider replayed effect", got, e)
	}
}
