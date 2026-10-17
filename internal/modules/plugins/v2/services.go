package v2

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/jackc/pgx/v5"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/platform/scheduling"
	"time"
)

func nextService(d contracts.Descriptor, config json.RawMessage, now time.Time) (*time.Time, bool, error) {
	if d.ScheduleSetting != "" {
		var settings map[string]json.RawMessage
		if json.Unmarshal(config, &settings) != nil {
			return nil, false, ErrInvalid
		}
		value := settings[d.ScheduleSetting]
		if len(value) == 0 || string(value) == "false" {
			return nil, false, nil
		}
		if string(value) != "true" {
			switch d.Trigger {
			case "cron":
				if json.Unmarshal(value, &d.Cron) != nil {
					return nil, false, ErrInvalid
				}
			case "interval":
				var seconds int64
				if json.Unmarshal(value, &seconds) != nil || seconds < 1 || seconds > 31536000 {
					return nil, false, ErrInvalid
				}
				d.IntervalMS = seconds * 1000
			default:
				return nil, false, ErrInvalid
			}
		}
	}
	var next time.Time
	switch d.Trigger {
	case "cron":
		v, e := scheduling.Next(d.Cron, now, time.Minute)
		if e != nil {
			return nil, false, ErrInvalid
		}
		next = v
	case "interval":
		if d.IntervalMS < 1000 || d.IntervalMS > 31536000000 {
			return nil, false, ErrInvalid
		}
		next = now.Add(time.Duration(d.IntervalMS) * time.Millisecond)
	case "date":
		v, e := time.Parse(time.RFC3339Nano, d.At)
		if e != nil {
			return nil, false, ErrInvalid
		}
		next = v
		if next.Before(now) {
			next = now
		}
	case "once":
		next = now
	default:
		return nil, false, ErrInvalid
	}
	return &next, true, nil
}
func (s Store) publishSchedules(ctx context.Context, tx pgx.Tx, l Lease, snapshot string, metadata contracts.Snapshot) error {
	for _, d := range metadata.Descriptors {
		if d.Kind != "service" {
			continue
		}
		next, enabled, e := nextService(d, l.Config.Value, time.Now())
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO plugin_v2_schedules(instance_id,service_id,snapshot_id,next_at,enabled)VALUES($1,$2,$3,$4,$5) ON CONFLICT(instance_id,service_id)DO UPDATE SET snapshot_id=excluded.snapshot_id,enabled=excluded.enabled,next_at=CASE WHEN plugin_v2_schedules.enabled AND excluded.enabled AND (SELECT grant_revision FROM plugin_v2_snapshots WHERE id=plugin_v2_schedules.snapshot_id)=$6 THEN plugin_v2_schedules.next_at ELSE excluded.next_at END`, l.ID, d.ID, snapshot, next, enabled, l.GrantRevision)
		if e != nil {
			return e
		}
	}
	return nil
}

type serviceWake struct {
	InstanceID string `json:"instanceId"`
	ServiceID  string `json:"serviceId"`
	SnapshotID string `json:"snapshotId"`
	Slot       string `json:"slot"`
}

func enqueueService(ctx context.Context, tx pgx.Tx, w serviceWake, key string) (string, error) {
	b, e := json.Marshal(w)
	if e != nil {
		return "", e
	}
	id := jobs.UUID()
	e = tx.QueryRow(ctx, `INSERT INTO jobs(id,kind,payload,idempotency_key,max_attempts)VALUES($1,'plugin.v2.service',$2,$3,3) ON CONFLICT(idempotency_key) WHERE idempotency_key IS NOT NULL DO UPDATE SET idempotency_key=excluded.idempotency_key WHERE jobs.payload=excluded.payload RETURNING id::text`, id, b, key).Scan(&id)
	if errors.Is(e, pgx.ErrNoRows) {
		return "", ErrConflict
	}
	return id, e
}
func (s Store) Schedule(ctx context.Context) error {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer rollback(ctx, tx)
	rows, e := tx.Query(ctx, `SELECT p.instance_id::text,p.service_id,p.snapshot_id::text,p.next_at FROM plugin_v2_schedules p JOIN plugin_v2_snapshots s ON s.id=p.snapshot_id JOIN plugin_v2_instances i ON i.id=p.instance_id WHERE p.enabled AND p.next_at<=now() AND i.desired_state='enabled' AND i.runtime_state='running' AND i.generation=s.generation AND i.grant_revision=s.grant_revision AND i.lease_until>now() AND NOT EXISTS(SELECT 1 FROM plugin_v2_runs r WHERE r.instance_id=i.id AND r.service_id=p.service_id AND (r.state IN('pending','running') OR (r.state='unknown' AND r.grant_revision=s.grant_revision))) AND NOT EXISTS(SELECT 1 FROM jobs j WHERE j.kind='plugin.v2.service' AND j.payload->>'instanceId'=i.id::text AND j.payload->>'serviceId'=p.service_id AND j.state IN('pending','running','retry')) AND (p.wake_job IS NULL OR NOT EXISTS(SELECT 1 FROM jobs j WHERE j.id=p.wake_job AND j.state IN('pending','running','retry'))) ORDER BY p.next_at,p.instance_id,p.service_id FOR UPDATE OF p SKIP LOCKED LIMIT 100`)
	if e != nil {
		return e
	}
	type due struct {
		wake serviceWake
		at   time.Time
	}
	list := []due{}
	for rows.Next() {
		var v due
		if e = rows.Scan(&v.wake.InstanceID, &v.wake.ServiceID, &v.wake.SnapshotID, &v.at); e != nil {
			break
		}
		list = append(list, v)
	}
	other := rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	if other != nil {
		return other
	}
	for _, v := range list {
		snap, d, e := s.Descriptor(ctx, v.wake.InstanceID, "service", v.wake.ServiceID)
		if e != nil {
			continue
		}
		if snap.ID != v.wake.SnapshotID {
			continue
		}
		var cipher []byte
		if e = tx.QueryRow(ctx, `SELECT ciphertext FROM plugin_v2_configs WHERE id=$1`, v.wake.InstanceID).Scan(&cipher); e != nil {
			return e
		}
		config, e := s.open("config", v.wake.InstanceID, cipher)
		if e != nil {
			return e
		}
		next, _, e := nextService(d, config, time.Now())
		if e != nil {
			return e
		}
		if d.Trigger == "once" || d.Trigger == "date" {
			next = nil
		}
		v.wake.Slot = v.at.UTC().Format(time.RFC3339Nano)
		job, e := enqueueService(ctx, tx, v.wake, "plugin.service:"+v.wake.InstanceID+":"+v.wake.ServiceID+":"+v.wake.Slot)
		if e != nil {
			return e
		}
		args := d.Kwargs
		if len(args) == 0 {
			args = json.RawMessage(`{}`)
		}
		if _, e = s.submitTx(ctx, tx, snap, d, args, "service-job:"+job, d.ID, nil, time.Now().Add(10*time.Minute)); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE plugin_v2_schedules SET next_at=$3,wake_job=$4 WHERE instance_id=$1 AND service_id=$2`, v.wake.InstanceID, v.wake.ServiceID, next, job); e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}

// Generic jobs only wake the persistent plugin state machine. Plugin failure must
// not activate jobs' independent retry policy or execute a second side effect.
func (s Store) ProcessService(ctx context.Context, j jobs.Job) error {
	var w serviceWake
	if contracts.Decode(j.Payload, &w) != nil {
		return ErrInvalid
	}
	// The run was inserted atomically with this wake; the owner pump independently
	// observes it. Restart/retry of the wake never creates a second invocation.
	var exists bool
	e := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM plugin_v2_runs WHERE dedupe_key=$1)`, "service-job:"+j.ID).Scan(&exists)
	return e
}

func (s Store) RunService(ctx context.Context, actor, id, key, operation string) (string, error) {
	if len(operation) < 8 || len(operation) > 100 {
		return "", ErrInvalid
	}
	snap, d, e := s.Descriptor(ctx, id, "service", key)
	if e != nil {
		return "", e
	}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return "", e
	}
	defer rollback(ctx, tx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,63))`, id); e != nil {
		return "", e
	}
	requestKey := "service:" + actor + ":" + id + ":" + operation
	hash := digest([]byte(id + ":" + key))
	var saved, oldHash string
	e = tx.QueryRow(ctx, `SELECT job_id::text,request_hash FROM plugin_v2_manual_requests WHERE key=$1`, requestKey).Scan(&saved, &oldHash)
	if e == nil {
		if oldHash != hash {
			return "", ErrConflict
		}
		return saved, nil
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return "", e
	}
	var job string
	e = tx.QueryRow(ctx, `SELECT id::text FROM jobs WHERE kind='plugin.v2.service' AND payload->>'instanceId'=$1 AND payload->>'serviceId'=$2 AND payload->>'snapshotId'=$3 AND (state IN('pending','running','retry') OR EXISTS(SELECT 1 FROM plugin_v2_runs r WHERE r.dedupe_key='service-job:'||jobs.id::text AND r.state IN('pending','running'))) ORDER BY created_at LIMIT 1`, id, key, snap.ID).Scan(&job)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return "", e
	}
	if job == "" {
		var n int
		if e = tx.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE kind='plugin.v2.service' AND payload->>'instanceId'=$1 AND state IN('pending','running','retry')`, id).Scan(&n); e != nil {
			return "", e
		}
		if n >= 16 {
			return "", contracts.NewError(contracts.Busy)
		}
		job, e = enqueueService(ctx, tx, serviceWake{id, key, snap.ID, operation}, "plugin.manual:"+id+":"+operation)
		if e != nil {
			return "", e
		}
	}
	args := d.Kwargs
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	if _, e = s.submitTx(ctx, tx, snap, d, args, "service-job:"+job, d.ID, nil, time.Now().Add(10*time.Minute)); e != nil {
		return "", e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO plugin_v2_manual_requests(key,instance_id,request_hash,job_id)VALUES($1,$2,$3,$4)`, requestKey, id, hash, job); e != nil {
		return "", e
	}
	details, _ := json.Marshal(map[string]string{"instanceId": id, "serviceId": key, "jobId": job})
	if _, e = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES($1,'plugins.v2.service.run',$2)`, actor, details); e != nil {
		return "", e
	}
	return job, tx.Commit(ctx)
}
func (s Store) History(ctx context.Context, id string, start, limit int) ([]Run, error) {
	if !validUUID(id) || start < 0 || start > 10000 || limit < 1 || limit > 200 {
		return nil, ErrInvalid
	}
	rows, e := s.DB.Query(ctx, `SELECT id::text,instance_id::text,kind,handler,state,reason,created_at,finished_at FROM plugin_v2_runs WHERE instance_id=$1 ORDER BY created_at DESC,id DESC LIMIT $2 OFFSET $3`, id, limit, start)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Run{}
	for rows.Next() {
		var v Run
		if e = rows.Scan(&v.ID, &v.InstanceID, &v.Kind, &v.Handler, &v.State, &v.Reason, &v.CreatedAt, &v.FinishedAt); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
