package v2

import (
	"context"
	"errors"
	"fmt"
	"time"
)

func (s Store) recoverExtensionRuns(ctx context.Context) error {
	_, e := s.DB.Exec(ctx, `WITH resumed AS(SELECT r.id,s.id AS snapshot_id,s.generation FROM plugin_v2_runs r JOIN plugin_v2_extensions x ON x.instance_id=r.instance_id JOIN plugin_v2_snapshots s ON s.id=x.snapshot_id JOIN plugin_v2_instances i ON i.id=r.instance_id WHERE r.state='pending' AND r.generation<>s.generation AND r.grant_revision=s.grant_revision AND i.desired_state='enabled' AND i.runtime_state='running' AND i.generation=s.generation AND EXISTS(SELECT 1 FROM jsonb_array_elements(s.descriptors->'descriptors') d WHERE d->>'handler'=r.handler) LIMIT 200) UPDATE plugin_v2_runs r SET snapshot_id=p.snapshot_id,generation=p.generation FROM resumed p WHERE r.id=p.id`)
	if e != nil {
		return e
	}
	_, e = s.DB.Exec(ctx, `WITH expired AS(SELECT r.id FROM plugin_v2_runs r JOIN plugin_v2_instances i ON i.id=r.instance_id WHERE r.state IN('pending','running') AND (r.deadline_at<=now() OR (r.state='running' AND r.lease_until<=now()) OR (r.state='running' AND i.generation IS DISTINCT FROM r.generation) OR i.grant_revision<>r.grant_revision OR i.desired_state<>'enabled') ORDER BY r.created_at,r.id FOR UPDATE OF r SKIP LOCKED LIMIT 200) UPDATE plugin_v2_runs r SET state=CASE WHEN r.state='running' THEN 'unknown' ELSE 'canceled' END,reason=CASE WHEN r.deadline_at<=now() THEN 'deadline' WHEN r.lease_until<=now() THEN 'worker_lost' ELSE 'revoked' END,finished_at=now() FROM expired WHERE r.id=expired.id`)
	if e != nil {
		return e
	}
	_, e = s.DB.Exec(ctx, `WITH stale AS(SELECT j.id FROM jobs j WHERE j.kind='plugin.v2.service' AND j.state IN('pending','retry') AND NOT EXISTS(SELECT 1 FROM plugin_v2_extensions x JOIN plugin_v2_snapshots s ON s.id=x.snapshot_id JOIN plugin_v2_instances i ON i.id=x.instance_id WHERE x.instance_id::text=j.payload->>'instanceId' AND s.id::text=j.payload->>'snapshotId' AND i.desired_state='enabled' AND i.generation=s.generation AND i.grant_revision=s.grant_revision) ORDER BY j.created_at LIMIT 100) UPDATE jobs j SET state='cancelled',updated_at=now() FROM stale WHERE j.id=stale.id`)
	if e != nil {
		return e
	}
	return nil
}

// Independent maintenance domains always get a turn, even under journal
// pressure. Errors retain their domain instead of masking earlier failures.
func (s Store) MaintainExtensions(ctx context.Context) error {
	var failures []error
	for _, step := range []struct {
		name string
		run  func(context.Context) error
	}{
		{"state", s.RecoverState}, {"runs", s.recoverExtensionRuns}, {"journal_gc", s.CollectJournal},
		{"bridge", s.Bridge}, {"deliver", s.Deliver}, {"schedule", s.Schedule},
	} {
		bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
		e := step.run(bounded)
		cancel()
		if e != nil {
			failures = append(failures, fmt.Errorf("%s: %w", step.name, e))
		}
	}
	return errors.Join(failures...)
}
