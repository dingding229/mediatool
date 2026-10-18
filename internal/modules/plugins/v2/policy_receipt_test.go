package v2

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"testing"
	"time"
)

// A cold worker may legally fall back at 200ms. Exercise known no-op fixtures
// within one shared 15s readiness budget until the exact handler has a real
// succeeded receipt. Neither production deadlines nor execution assertions are
// relaxed: an absent/revoked/broken handler can never satisfy this check.
func proveRealPolicy(t *testing.T, s Store, id string, typ contracts.ChainEventType, until time.Time, invoke func() bool) {
	t.Helper()
	ctx := context.Background()
	handler := contracts.HandlerID("chain", string(typ))
	for time.Now().Before(until) {
		accepted := invoke()
		var executed bool
		if e := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM plugin_v2_runs WHERE instance_id=$1 AND kind='chain' AND handler=$2 AND state='succeeded')`, id, handler).Scan(&executed); e != nil {
			t.Fatal(e)
		}
		if accepted && executed {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	var states json.RawMessage
	e := s.DB.QueryRow(ctx, `SELECT coalesce(jsonb_agg(jsonb_build_object('state',state,'reason',reason,'count',n)),'[]'::jsonb) FROM (SELECT state,reason,count(*) n FROM plugin_v2_runs WHERE instance_id=$1 AND handler=$2 GROUP BY state,reason) x`, id, handler).Scan(&states)
	t.Fatalf("policy %s has no accepted success receipt within readiness budget; states=%s error=%v", typ, states, e)
}
func policyTimedOut(err error) bool {
	var rpc *contracts.Error
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || errors.As(err, &rpc) && rpc.Code == contracts.ResultUnknown
}
