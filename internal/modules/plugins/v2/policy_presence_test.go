package v2

import (
	"context"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/jackc/pgx/v5"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type policyQueryCount struct{ queries atomic.Int64 }

func (c *policyQueryCount) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	if strings.Contains(d.SQL, "plugins:policy-hooks") {
		c.queries.Add(1)
	}
	return ctx
}
func (*policyQueryCount) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}
func TestAbsentConversionPoliciesAvoidPerFileSQLAndEpochFences(t *testing.T) {
	trace := &policyQueryCount{}
	s := fixtureTracer(t, trace)
	q := contracts.MediaConversion{Candidates: []contracts.MediaIdentity{}}
	for range 1000 {
		if _, _, e := s.ConvertMedia(context.Background(), q); e != nil {
			t.Fatal(e)
		}
	}
	if n := trace.queries.Load(); n != 2 {
		t.Fatal("zero-plugin per-file SQL", n)
	}
	key := "chain/media.recognize.convert"
	epoch, _ := absentPolicy(s.DB, key)
	invalidateNameHooks(s.DB)
	rememberAbsentPolicy(s.DB, key, epoch, time.Now().Add(time.Hour))
	if _, hit := absentPolicy(s.DB, key); hit {
		t.Fatal("stale read poisoned absence")
	}
}
