package subscriptions

import (
	"context"
	"errors"
	deferutil "media-tool/internal/platform/cleanup"

	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/search"
)

var ErrEvaluationRefresh = errors.New("订阅、站点或候选快照已变化；请重新预演或发起新的评估，已有下载不会重放")

func (e Engine) needsRefresh(ctx context.Context, runID string) error {
	_, err := e.DB.Exec(ctx, `UPDATE subscription_runs SET state='needs_refresh',collection_state='needs_refresh',finished_at=now()WHERE id=$1`, runID)
	if err != nil {
		return err
	}
	return errors.Join(jobs.ErrOperatorRequired, ErrEvaluationRefresh)
}

// Reset only pre-decision staging. A failed multi-site HTTP attempt never
// contributes old entries/counts/seen markers to the next fresh collection.
func (e Engine) beginCollection(ctx context.Context, record Record, runID, searchRunID string) (bool, error) {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer deferutil.Rollback(ctx, tx)
	var lockedRun string
	if err = tx.QueryRow(ctx, `SELECT id::text FROM search_runs WHERE id=$1 FOR UPDATE`, searchRunID).Scan(&lockedRun); err != nil {
		return false, err
	}
	var phase string
	if err = tx.QueryRow(ctx, `SELECT collection_state FROM subscription_runs WHERE id=$1 FOR UPDATE`, runID).Scan(&phase); err != nil {
		return false, err
	}
	if phase == "needs_refresh" {
		return false, errors.Join(jobs.ErrOperatorRequired, ErrEvaluationRefresh)
	}
	if phase == "collected" {
		return false, nil
	}
	var effects bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM subscription_decisions WHERE run_id=$1) OR EXISTS(SELECT 1 FROM download_tasks d WHERE d.details->>'searchRunId'=$2 OR (d.resource_id IN(SELECT resource_id FROM search_run_resources WHERE run_id=$2::uuid) AND d.download_state NOT IN('failed','cancelled')))`, runID, searchRunID).Scan(&effects); err != nil {
		return false, err
	}
	if effects {
		if _, err = tx.Exec(ctx, `UPDATE subscription_runs SET state='needs_refresh',collection_state='needs_refresh',finished_at=now()WHERE id=$1`, runID); err != nil {
			return false, err
		}
		if err = tx.Commit(ctx); err != nil {
			return false, err
		}
		return false, errors.Join(jobs.ErrOperatorRequired, ErrEvaluationRefresh)
	}
	for _, statement := range []string{
		`DELETE FROM subscription_rss_run_items WHERE run_id=$1`,
		`DELETE FROM subscription_rss_run_accounts WHERE run_id=$1`,
	} {
		if _, err = tx.Exec(ctx, statement, runID); err != nil {
			return false, err
		}
	}
	if _, err = tx.Exec(ctx, `DELETE FROM search_run_resources WHERE run_id=$1`, searchRunID); err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `UPDATE search_site_results SET state='pending',message='',result_count=0,source_revision=NULL,definition_fingerprint=NULL,observed_at=NULL WHERE run_id=$1`, searchRunID); err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `UPDATE search_runs SET state='pending',finished_at=NULL WHERE id=$1`, searchRunID); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

func (e Engine) completeCollection(ctx context.Context, record Record, runID, searchRunID string) error {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer deferutil.Rollback(ctx, tx)
	var revision int64
	var enabled bool
	if err = tx.QueryRow(ctx, `SELECT revision,enabled FROM subscriptions WHERE id=$1 FOR SHARE`, record.ID).Scan(&revision, &enabled); err != nil {
		return err
	}
	var preview bool
	if err = tx.QueryRow(ctx, `SELECT preview FROM subscription_runs WHERE id=$1`, runID).Scan(&preview); err != nil {
		return err
	}
	if revision != record.Revision || (!preview && !enabled) {
		return ErrEvaluationRefresh
	}
	if record.Source != "search" && !preview {
		var accounts int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM subscription_rss_run_accounts WHERE run_id=$1`, runID).Scan(&accounts); err != nil || accounts != len(record.Rules.AccountIDs) {
			if err != nil {
				return err
			}
			return errors.New("RSS集合尚未完整收集；未更新订阅基线")
		}
	}
	var lockedRun string
	if err = tx.QueryRow(ctx, `SELECT id::text FROM search_runs WHERE id=$1 FOR UPDATE`, searchRunID).Scan(&lockedRun); err != nil {
		return err
	}
	searcher := search.Engine{DB: e.DB, Vault: e.Connections.Repository.Vault, Connections: e.Connections}
	if err = searcher.ValidateRunSourcesTx(ctx, tx, searchRunID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE subscription_runs SET collection_state='collected',collected_at=now()WHERE id=$1 AND collection_state='collecting'`, runID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (e Engine) validateEvaluation(ctx context.Context, record Record, run RunRecord) error {
	var revision int64
	var enabled bool
	if err := e.DB.QueryRow(ctx, `SELECT revision,enabled FROM subscriptions WHERE id=$1`, record.ID).Scan(&revision, &enabled); err != nil {
		return err
	}
	if revision != record.Revision || (!run.Preview && !enabled) {
		return e.needsRefresh(ctx, run.ID)
	}
	return nil
}

func snapshotRequiresRefresh(err error) bool {
	return errors.Is(err, search.ErrSnapshotUnavailable) || errors.Is(err, search.ErrSnapshotChanged) || errors.Is(err, search.ErrSnapshotExpired)
}
