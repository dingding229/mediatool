package v2

import (
	"context"
)

func (s Store) MaintainExtensions(ctx context.Context) error {
	if e := s.RecoverState(ctx); e != nil {
		return e
	}
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
	if e = s.Bridge(ctx); e != nil {
		return e
	}
	if e = s.Deliver(ctx); e != nil {
		return e
	}
	if e = s.Schedule(ctx); e != nil {
		return e
	}
	// Bounded seven-day private call retention. The event journal retains replay
	// identities; callers never use a high-water mark over transaction allocation.
	_, e = s.DB.Exec(ctx, `DELETE FROM plugin_v2_runs WHERE id IN(SELECT id FROM plugin_v2_runs WHERE state IN('succeeded','failed','canceled') AND finished_at<now()-interval '7 days' ORDER BY finished_at LIMIT 100)`)
	return e
}
