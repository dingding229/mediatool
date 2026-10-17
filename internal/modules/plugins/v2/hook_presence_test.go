package v2

import (
	"context"
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/jobs"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type hookQueryCount struct{ queries atomic.Int64 }

func (c *hookQueryCount) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	if strings.Contains(d.SQL, "plugins:name-hooks") {
		c.queries.Add(1)
	}
	return ctx
}
func (*hookQueryCount) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}
func TestAbsentNameHooksAvoidPerItemSQL(t *testing.T) {
	trace := &hookQueryCount{}
	s := fixtureTracer(t, trace)
	ctx := context.Background()
	for range 200 {
		if v := s.RecognizeName(ctx, "Owned.2026.mkv"); v.Title != "Owned" {
			t.Fatal(v)
		}
	}
	if n := trace.queries.Load(); n != 1 {
		t.Fatal("zero-plugin hot path queried per item", n)
	}
	start := time.Now()
	before := trace.queries.Load()
	for range 10000 {
		s.RecognizeName(ctx, "Owned.2026.1080p.WEB-DL.H264.mkv")
	}
	elapsed := time.Since(start)
	queries := trace.queries.Load() - before
	if queries > int64(elapsed/hookAbsenceTTL)+1 {
		t.Fatal("absence SQL not TTL-bounded", queries, elapsed)
	}
	t.Logf("10000-name empty-hook scan elapsed=%s extraSQL=%d (previously 20000 SQL)", elapsed, queries)
}
func TestNameHookAbsenceInvalidatesAtPublicationAndRunningTransition(t *testing.T) {
	trace := &hookQueryCount{}
	s := fixtureTracer(t, trace)
	ctx := context.Background()
	defs, _ := Catalog()
	d := defs[0]
	if _, e := s.nameHooks(ctx); e != nil {
		t.Fatal(e)
	}
	in, e := s.Apply(ctx, "admin", Request{Kind: Install, OperationKey: jobs.UUID(), PluginID: d.Manifest.ID, Fingerprint: d.Fingerprint, Scopes: scopes(d), Config: json.RawMessage(`{}`), Confirm: true})
	if e != nil {
		t.Fatal(e)
	}
	if _, hit := absentHooks(s.DB); hit {
		t.Fatal("install did not invalidate")
	}
	in, e = s.Apply(ctx, "admin", Request{Kind: Enable, OperationKey: jobs.UUID(), InstanceID: in.ID, Fingerprint: d.Fingerprint, ExpectedRevision: in.Revision, Confirm: true})
	if e != nil {
		t.Fatal(e)
	}
	l, e := s.Claim(ctx, jobs.UUID())
	if e != nil {
		t.Fatal(e)
	}
	descriptor := contracts.Descriptor{Kind: "chain", ID: "name.recognize", Handler: contracts.HandlerID("chain", "name.recognize"), RequiredScopes: []string{"events.chain.name.recognize"}}
	if e = s.Publish(ctx, l, contracts.Snapshot{Version: 2, Descriptors: []contracts.Descriptor{descriptor}}); e != nil {
		t.Fatal(e)
	}
	if _, hit := absentHooks(s.DB); hit {
		t.Fatal("publication did not invalidate")
	}
	// Query in the publication->running gap must not poison the running snapshot.
	if _, e = s.nameHooks(ctx); e != nil {
		t.Fatal(e)
	}
	if e = s.Renew(ctx, l, true); e != nil {
		t.Fatal(e)
	}
	before := trace.queries.Load()
	list, e := s.nameHooks(ctx)
	if trace.queries.Load()-before != 1 {
		t.Fatal("positive hooks must load once, not per kind")
	}
	if e != nil || len(list["chain"]) != 1 {
		t.Fatal("running transition cached as absent", list, e)
	}
	if _, e = s.DB.Exec(ctx, `DELETE FROM plugin_v2_grants WHERE instance_id=$1 AND scope='events.chain.name.recognize'`, l.ID); e != nil {
		t.Fatal(e)
	}
	if _, _, e = s.Descriptor(ctx, l.ID, "chain", "name.recognize"); e == nil {
		t.Fatal("authorization cached")
	}
	current, e := s.List(ctx, 0, 50)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Apply(ctx, "admin", Request{Kind: Disable, OperationKey: jobs.UUID(), InstanceID: in.ID, Fingerprint: d.Fingerprint, ExpectedRevision: current[0].Revision, Confirm: true}); e != nil {
		t.Fatal(e)
	}
	list, e = s.nameHooks(ctx)
	if e != nil || len(list["chain"]) != 0 {
		t.Fatal("disabled hook retained", list, e)
	}
	if e = s.Refresh(ctx); e != nil {
		t.Fatal(e)
	}
	if _, hit := absentHooks(s.DB); hit {
		t.Fatal("refresh did not invalidate")
	}
}
func TestNameHookAbsenceEpochAndPoolIsolation(t *testing.T) {
	a, b := &pgxpool.Pool{}, &pgxpool.Pool{}
	ticket, _ := absentHooks(a)
	invalidateNameHooks(a)
	rememberAbsentHooks(a, ticket, time.Now().Add(hookAbsenceTTL))
	if _, hit := absentHooks(a); hit {
		t.Fatal("late query refilled invalidated absence")
	}
	ticket, _ = absentHooks(a)
	rememberAbsentHooks(a, ticket, time.Now().Add(hookAbsenceTTL))
	if _, hit := absentHooks(a); !hit {
		t.Fatal("absence not cached")
	}
	if _, hit := absentHooks(b); hit {
		t.Fatal("cache leaked between pools")
	}
	// Entry eviction cannot recycle an epoch and accept an old in-flight SELECT.
	old := a
	for range 33 {
		absentHooks(&pgxpool.Pool{})
	}
	absentHooks(old)
	rememberAbsentHooks(old, ticket, time.Now().Add(hookAbsenceTTL))
	if _, hit := absentHooks(old); hit {
		t.Fatal("eviction recycled an epoch")
	}
}

func TestNameHookAbsenceNeverCachesQueryFailure(t *testing.T) {
	s := fixture(t)
	invalidateNameHooks(s.DB)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := s.nameHooks(ctx); e == nil {
		t.Fatal("canceled query succeeded")
	}
	if _, hit := absentHooks(s.DB); hit {
		t.Fatal("query failure cached as no hooks")
	}
}
func TestNameHookAbsenceConfigUpdateInvalidates(t *testing.T) {
	s, l := activeFixture(t)
	ticket, _ := absentHooks(s.DB)
	rememberAbsentHooks(s.DB, ticket, time.Now().Add(hookAbsenceTTL))
	revision := int64(1)
	raw, e := json.Marshal(contracts.HostCall{Method: "plugin.config.update", Args: json.RawMessage(`{"value":{"changed":true}}`), ExpectedRevision: &revision})
	if e != nil {
		t.Fatal(e)
	}
	if _, e := s.Host(l)(context.Background(), raw); e != nil {
		t.Fatal(e)
	}
	if _, hit := absentHooks(s.DB); hit {
		t.Fatal("config generation did not invalidate absence")
	}
}

func TestNameHookAbsenceDoesNotExtendOldQueryTTL(t *testing.T) {
	p := &pgxpool.Pool{}
	ticket, _ := absentHooks(p)
	rememberAbsentHooks(p, ticket, time.Now().Add(-time.Second))
	if _, hit := absentHooks(p); hit {
		t.Fatal("late SELECT extended TTL from completion time")
	}
}
