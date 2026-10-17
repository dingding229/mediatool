package acquisition

import (
	"context"
	"encoding/json"
	"errors"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/search"
	"media-tool/internal/modules/subscriptionpolicy"
	deferutil "media-tool/internal/platform/cleanup"
)

func (e Engine) guardSubscriptionAdd(ctx context.Context, d taskDetails, resourceID string) (func(), error) {
	if d.SubscriptionRunID == "" {
		return func() {}, nil
	}
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	fail := func() (func(), error) {
		deferutil.Rollback(ctx, tx)
		return nil, errors.Join(jobs.ErrOperatorRequired, errors.New("订阅已归档、停用、规则或H&R证明已变更，请重新评估"))
	}
	var enabled, archived bool
	var revision, runRevision int64
	var rulesRaw, snapshot []byte
	err = tx.QueryRow(ctx, `SELECT s.enabled,s.archived_at IS NOT NULL,s.revision,r.revision,s.rules FROM subscription_runs r JOIN subscriptions s ON s.id=r.subscription_id WHERE r.id=$1 FOR SHARE OF s`, d.SubscriptionRunID).Scan(&enabled, &archived, &revision, &runRevision, &rulesRaw)
	if err != nil || !enabled || archived || revision != runRevision {
		return fail()
	}
	err = tx.QueryRow(ctx, `SELECT public_snapshot FROM search_run_resources WHERE run_id=$1 AND resource_id=$2`, d.SearchRunID, resourceID).Scan(&snapshot)
	if err != nil {
		return fail()
	}
	var candidate search.Resource
	var rules subscriptionpolicy.Rules
	if json.Unmarshal(snapshot, &candidate) != nil || json.Unmarshal(rulesRaw, &rules) != nil {
		return fail()
	}
	if ok, _ := subscriptionpolicy.Filter(candidate, d.Media, rules); !ok {
		return fail()
	}
	return func() { deferutil.Rollback(context.Background(), tx) }, nil
}
