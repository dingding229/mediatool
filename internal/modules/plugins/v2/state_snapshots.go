package v2

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"media-tool/internal/modules/jobs"
	deferutil "media-tool/internal/platform/cleanup"
	"os"
	"strings"
	"time"
)

const stateReserve int64 = 80 << 20

// Limits include retained snapshots of uninstalled instances; uninstall preserves data.
type StateSnapshot struct {
	ID           string    `json:"id"`
	InstanceID   string    `json:"instanceId"`
	PluginID     string    `json:"pluginId"`
	ConfigSchema int       `json:"configSchema"`
	DataSchema   int       `json:"dataSchema"`
	Revision     int64     `json:"revision"`
	Bytes        int64     `json:"bytes"`
	Files        int       `json:"files"`
	State        string    `json:"state"`
	CreatedAt    time.Time `json:"createdAt"`
	DataPath     string    `json:"dataPath"`
}
type StateRequest struct {
	Kind             string `json:"kind"`
	SnapshotID       string `json:"snapshotId"`
	OperationKey     string `json:"operationKey"`
	ExpectedRevision int64  `json:"expectedRevision"`
	Fingerprint      string `json:"fingerprint"`
	Confirm          bool   `json:"confirm"`
}

const stateColumns = `id::text,instance_id::text,plugin_id,config_schema,data_schema,operation_revision,bytes,files,state,created_at,data_path`

func scanState(row pgx.Row) (v StateSnapshot, e error) {
	e = row.Scan(&v.ID, &v.InstanceID, &v.PluginID, &v.ConfigSchema, &v.DataSchema, &v.Revision, &v.Bytes, &v.Files, &v.State, &v.CreatedAt, &v.DataPath)
	if errors.Is(e, pgx.ErrNoRows) {
		e = ErrNotFound
	}
	return
}
func (s Store) StateSnapshots(ctx context.Context, id string) ([]StateSnapshot, error) {
	if !validUUID(id) {
		return nil, ErrInvalid
	}
	out := []StateSnapshot{}
	rows, e := s.DB.Query(ctx, `SELECT `+stateColumns+` FROM plugin_v2_state_snapshots WHERE instance_id=$1 AND state IN('writing','ready') ORDER BY created_at DESC,id LIMIT 200`, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		v, e := scanState(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func stateAudit(ctx context.Context, tx pgx.Tx, actor, kind, id, snapshot string) error {
	raw, _ := json.Marshal(map[string]string{"instanceId": id, "snapshotId": snapshot})
	_, e := tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES($1,$2,$3)`, actor, "plugins.state."+kind, raw)
	return e
}
func (s Store) StateOperation(ctx context.Context, actor, id string, q StateRequest) (StateSnapshot, error) {
	var out StateSnapshot
	if !validUUID(id) || !q.Confirm || len(q.OperationKey) < 8 || len(q.OperationKey) > 100 || strings.IndexFunc(q.OperationKey, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_')
	}) >= 0 || (q.Kind != "create" && q.Kind != "restore" && q.Kind != "delete") || (q.Kind != "create" && !validUUID(q.SnapshotID)) {
		return out, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	raw, _ := json.Marshal(q)
	hash := digest(raw)
	key := actor + ":" + q.OperationKey
	task := jobs.UUID()
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return out, e
	}
	defer rollback(ctx, tx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, key); e != nil {
		return out, e
	}
	var oldHash, state string
	var saved []byte
	e = tx.QueryRow(ctx, `SELECT request_hash,state,result FROM plugin_v2_state_tasks WHERE operation_key=$1`, key).Scan(&oldHash, &state, &saved)
	if e == nil {
		if oldHash != hash || state != "done" {
			return out, ErrConflict
		}
		e = json.Unmarshal(saved, &out)
		return out, e
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return out, e
	}
	var plugin, fp, availability string
	var revision int64
	var idle bool
	e = tx.QueryRow(ctx, `SELECT i.plugin_id,d.fingerprint,d.availability,i.revision,(i.desired_state='disabled' AND i.generation IS NULL AND i.worker_pid=0 AND i.runtime_state IN('stopped','needs_action') AND i.data_operation IS NULL) FROM plugin_v2_instances i JOIN plugin_v2_definitions d ON d.id=i.plugin_id WHERE i.id=$1 FOR UPDATE OF i`, id).Scan(&plugin, &fp, &availability, &revision, &idle)
	if errors.Is(e, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if e != nil {
		return out, e
	}
	if !idle || revision != q.ExpectedRevision || fp != q.Fingerprint {
		return out, ErrConflict
	}
	d, e := definition(plugin)
	if q.Kind != "delete" && (e != nil || d.Fingerprint != fp || availability != "available") {
		return out, ErrUnavailable
	}
	if q.Kind != "delete" {
		if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(7062064)`); e != nil {
			return out, e
		}
		var total, own int64
		if e = tx.QueryRow(ctx, `SELECT COALESCE(sum(bytes),0),COALESCE(sum(bytes) FILTER(WHERE instance_id=$1),0) FROM (SELECT instance_id,bytes FROM plugin_v2_state_snapshots WHERE state IN('ready','writing') UNION ALL SELECT instance_id,reserved_bytes FROM plugin_v2_state_tasks WHERE NOT cleaned) x`, id).Scan(&total, &own); e != nil {
			return out, e
		}
		if total+stateReserve > 1<<30 || own+stateReserve > 256<<20 {
			return out, ErrInvalid
		}
	}
	if q.Kind == "create" {
		if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(7062064)`); e != nil {
			return out, e
		}
		var count, ownCount int
		var size, ownSize int64
		if e = tx.QueryRow(ctx, `SELECT count(*),COALESCE(sum(bytes),0),count(*) FILTER(WHERE instance_id=$1),COALESCE(sum(bytes) FILTER(WHERE instance_id=$1),0) FROM plugin_v2_state_snapshots WHERE state IN('writing','ready')`, id).Scan(&count, &size, &ownCount, &ownSize); e != nil {
			return out, e
		}
		if count >= 64 || ownCount >= 5 || size+stateReserve > 1<<30 || ownSize+stateReserve > 256<<20 {
			return out, ErrInvalid
		}
		out, e = scanState(tx.QueryRow(ctx, `INSERT INTO plugin_v2_state_snapshots(id,instance_id,plugin_id,fingerprint,config_schema,data_schema,operation_revision,state,bytes,data_path)VALUES($1,$2,$3,$4,$5,$6,$7,'writing',$8,$9) RETURNING `+stateColumns, jobs.UUID(), id, plugin, fp, d.Manifest.ConfigSchemaVersion, d.Manifest.DataSchemaVersion, revision, stateReserve, "snapshots/"+task+"/data"))
		if e != nil {
			return out, e
		}
		if e = s.captureStateValues(ctx, tx, id, out.ID); e != nil {
			return out, e
		}
	} else {
		out, e = scanState(tx.QueryRow(ctx, `SELECT `+stateColumns+` FROM plugin_v2_state_snapshots WHERE id=$1 AND instance_id=$2 AND state='ready' FOR UPDATE`, q.SnapshotID, id))
		if e != nil {
			return out, e
		}
		if q.Kind == "restore" && (out.PluginID != plugin || out.ConfigSchema != d.Manifest.ConfigSchemaVersion || out.DataSchema != d.Manifest.DataSchemaVersion) {
			return out, ErrInvalid
		}
	}
	if _, e = tx.Exec(ctx, `INSERT INTO plugin_v2_state_tasks(id,instance_id,snapshot_id,operation_key,request_hash,kind,state,lease_until,revision,fingerprint)VALUES($1,$2,$3,$4,$5,$6,'writing',now()+interval '3 minutes',$7,$8)`, task, id, out.ID, key, hash, q.Kind, revision, fp); e != nil {
		return out, e
	}
	if q.Kind != "create" {
		reservation := stateReserve
		if q.Kind == "delete" {
			reservation = out.Bytes
		}
		if _, e = tx.Exec(ctx, `UPDATE plugin_v2_state_tasks SET reserved_bytes=$2 WHERE id=$1`, task, reservation); e != nil {
			return out, e
		}
	}
	if _, e = tx.Exec(ctx, `UPDATE plugin_v2_instances SET data_operation=$2 WHERE id=$1`, id, task); e != nil {
		return out, e
	}
	if e = stateAudit(ctx, tx, actor, q.Kind+".started", id, out.ID); e != nil {
		return out, e
	}
	if e = tx.Commit(ctx); e != nil {
		return out, e
	}
	// Errors/unknown commits retain a fenced journal. Recovery, never a blind replay,
	// decides whether the atomic exchange must be undone using inode identity.
	e = s.finishState(ctx, actor, id, task, q.Kind, &out)
	if e != nil {
		c, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_, expireErr := s.DB.Exec(c, `UPDATE plugin_v2_state_tasks SET lease_until=now() WHERE id=$1 AND state IN('writing','prepared')`, task)
		if expireErr == nil {
			_ = s.RecoverState(c)
		}
		return StateSnapshot{}, e
	}
	return out, nil
}
func (s Store) finishState(ctx context.Context, actor, id, task, kind string, out *StateSnapshot) error {
	r, e := s.stateRoot(id)
	if e != nil {
		return e
	}
	defer deferutil.Close(r, "instance descriptor only")
	if e = r.Mkdir("snapshots", 0700); e != nil && !errors.Is(e, os.ErrExist) {
		return e
	}
	snapRoot, e := directory(r, "snapshots")
	if e != nil {
		return e
	}
	defer deferutil.Close(snapRoot, "snapshot descriptor only")
	if e = snapRoot.Mkdir(task, 0700); e != nil {
		return e
	}
	if e = syncRoot(snapRoot); e != nil {
		return e
	}
	if e = syncRoot(r); e != nil {
		return e
	}
	work, e := directory(snapRoot, task)
	if e != nil {
		return e
	}
	defer deferutil.Close(work, "operation descriptor only")
	var total stateSize
	stage := "snapshots/" + task + "/stage"
	if kind != "delete" {
		var source *os.Root
		if kind == "create" {
			source, e = directory(r, "data")
		} else {
			source, e = snapshotData(r, out.DataPath)
		}
		if e != nil {
			return e
		}
		defer deferutil.Close(source, "source descriptor only")
		if e = work.Mkdir("stage", 0700); e != nil {
			return e
		}
		dst, e := directory(work, "stage")
		if e != nil {
			return e
		}
		e = copyState(ctx, source, dst, 0, &total)
		closeState(dst)
		if e != nil {
			return e
		}
		if e = syncRoot(work); e != nil {
			return e
		}
		if kind == "restore" {
			current, e := directory(r, "data")
			if e != nil {
				return e
			}
			check := stateSize{}
			e = copyState(ctx, current, nil, 0, &check)
			closeState(current)
			if e != nil {
				return e
			}
			ino, e := identity(r, stage)
			if e != nil {
				return e
			}
			tag, e := s.DB.Exec(ctx, `UPDATE plugin_v2_state_tasks SET state='prepared',stage_identity=$2 WHERE id=$1 AND state='writing' AND lease_until>clock_timestamp()`, task, ino)
			if e != nil {
				return e
			}
			if tag.RowsAffected() != 1 {
				return ErrConflict
			}
		}
	}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer rollback(ctx, tx)
	var valid bool
	e = tx.QueryRow(ctx, `SELECT (i.data_operation=t.id AND i.revision=t.revision AND d.fingerprint=t.fingerprint AND i.desired_state='disabled' AND i.generation IS NULL AND i.worker_pid=0 AND t.lease_until>clock_timestamp() AND t.state IN('writing','prepared')) FROM plugin_v2_state_tasks t JOIN plugin_v2_instances i ON i.id=t.instance_id JOIN plugin_v2_definitions d ON d.id=i.plugin_id WHERE t.id=$1 FOR UPDATE OF i,t`, task).Scan(&valid)
	if e != nil {
		return e
	}
	if !valid {
		return ErrConflict
	}
	switch kind {
	case "create":
		if e = work.Rename("stage", "data"); e != nil {
			return e
		}
		if e = syncRoot(work); e != nil {
			return e
		}
		out.State = "ready"
		out.Bytes = total.Bytes
		out.Files = total.Files
		var dbSize int64
		if e = tx.QueryRow(ctx, `SELECT COALESCE(sum(octet_length(ciphertext)),0) FROM plugin_v2_state_values WHERE snapshot_id=$1`, out.ID).Scan(&dbSize); e != nil {
			return e
		}
		out.Bytes += dbSize
		if out.Bytes > stateReserve {
			return ErrInvalid
		}
		if _, e = tx.Exec(ctx, `UPDATE plugin_v2_state_snapshots SET state='ready',bytes=$2,files=$3 WHERE id=$1`, out.ID, out.Bytes, out.Files); e != nil {
			return e
		}
	case "restore":
		if e = s.restoreStateValues(ctx, tx, id, out.ID); e != nil {
			return e
		}
		if e = exchangeState(r, "data", stage); e != nil {
			return e
		}
		if e = syncRoot(work); e != nil {
			return e
		}
	case "delete":
		out.State = "deleted"
		if _, e = tx.Exec(ctx, `DELETE FROM plugin_v2_state_values WHERE snapshot_id=$1`, out.ID); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE plugin_v2_state_snapshots SET state='deleted',bytes=0 WHERE id=$1`, out.ID); e != nil {
			return e
		}
	}
	if _, e = tx.Exec(ctx, `UPDATE plugin_v2_instances SET data_operation=NULL,revision=revision+1,grant_revision=grant_revision+1,desired_state='disabled',updated_at=now() WHERE id=$1 AND data_operation=$2`, id, task); e != nil {
		return e
	}
	raw, _ := json.Marshal(out)
	if _, e = tx.Exec(ctx, `UPDATE plugin_v2_state_tasks SET state='done',result=$2 WHERE id=$1`, task, raw); e != nil {
		return e
	}
	if e = stateAudit(ctx, tx, actor, kind+".completed", id, out.ID); e != nil {
		return e
	}
	if e = tx.Commit(ctx); e != nil {
		return e
	}
	// Best-effort cleanup is journaled and retried by recovery. Never report a
	// committed restore as failed solely because old private files remain to GC.
	_ = s.cleanStateTask(ctx, task)
	return nil
}
func snapshotData(r *os.Root, path string) (*os.Root, error) {
	parts := strings.Split(path, "/")
	if len(parts) != 3 || parts[0] != "snapshots" || !validUUID(parts[1]) || parts[2] != "data" {
		return nil, ErrInvalid
	}
	a, e := directory(r, "snapshots")
	if e != nil {
		return nil, e
	}
	defer closeState(a)
	b, e := directory(a, parts[1])
	if e != nil {
		return nil, e
	}
	defer closeState(b)
	return directory(b, "data")
}
