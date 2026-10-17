package v2

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/platform/secrets"
	"regexp"
	"slices"
	"strings"
	"time"
)

type Store struct {
	DB    *pgxpool.Pool
	Vault *secrets.Vault
	Root  string
}

func rollback(ctx context.Context, tx pgx.Tx) { _ = tx.Rollback(ctx) } // Successful commit or primary error is handled by caller.
func validUUID(s string) bool {
	ok, _ := regexp.MatchString(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`, s)
	return ok
}
func (s Store) seal(kind, id string, raw json.RawMessage) ([]byte, error) {
	if s.Vault == nil || len(raw) > 128<<10 {
		return nil, ErrInvalid
	}
	if contracts.ValidateJSON(raw) != nil {
		return nil, ErrInvalid
	}
	cipher, e := s.Vault.Seal("plugin:v2:"+kind+":"+id, map[string]string{"value": string(raw)})
	if e != nil {
		return nil, e
	}
	if len(cipher) > 262144 {
		return nil, ErrInvalid
	}
	return cipher, nil
}
func (s Store) open(kind, id string, raw []byte) (json.RawMessage, error) {
	if s.Vault == nil {
		return nil, ErrUnavailable
	}
	m, e := s.Vault.Open("plugin:v2:"+kind+":"+id, raw)
	if e != nil {
		return nil, ErrUnavailable
	}
	out := json.RawMessage(m["value"])
	if contracts.ValidateJSON(out) != nil {
		return nil, ErrUnavailable
	}
	return out, nil
}
func (s Store) List(ctx context.Context, start, limit int) ([]Instance, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if s.DB == nil {
		return nil, ErrUnavailable
	}
	if start < 0 || limit < 1 || limit > 200 {
		return nil, ErrInvalid
	}
	rows, e := s.DB.Query(ctx, `SELECT id::text,plugin_id,desired_state,runtime_state,revision,reason FROM plugin_v2_instances WHERE desired_state<>'absent' ORDER BY created_at,id LIMIT $1 OFFSET $2`, limit, start)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Instance{}
	for rows.Next() {
		var v Instance
		if e = rows.Scan(&v.ID, &v.PluginID, &v.Desired, &v.State, &v.Revision, &v.Reason); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s Store) Apply(ctx context.Context, actor string, in Request) (Instance, error) {
	defer invalidateNameHooks(s.DB)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, e := s.apply(ctx, actor, in)
	if e != nil {
		return out, e
	}
	if in.Kind == Install || in.Kind == Clone {
		if e = s.EnsurePaths(out.ID); e != nil {
			_, markErr := s.DB.Exec(ctx, `UPDATE plugin_v2_instances SET runtime_state='needs_action',reason='data_unavailable' WHERE id=$1 AND generation IS NULL`, out.ID)
			return out, errors.Join(ErrUnavailable, markErr)
		}
	}
	return out, nil
}
func (s Store) apply(ctx context.Context, actor string, in Request) (Instance, error) {
	if s.DB == nil || s.Vault == nil {
		return Instance{}, ErrUnavailable
	}
	if !in.Confirm || len(actor) == 0 || len(actor) > 128 || len(in.OperationKey) < 8 || len(in.OperationKey) > 100 || in.ExpectedRevision < 0 {
		return Instance{}, ErrInvalid
	}
	for _, r := range in.OperationKey {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return Instance{}, ErrInvalid
		}
	}
	raw, e := json.Marshal(in)
	if e != nil {
		return Instance{}, ErrInvalid
	}
	hash := digest(raw)
	key := actor + ":" + in.OperationKey
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return Instance{}, e
	}
	defer rollback(ctx, tx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, key); e != nil {
		return Instance{}, e
	}
	var oldHash, id string
	var saved []byte
	e = tx.QueryRow(ctx, `SELECT request_digest,instance_id::text,result FROM plugin_v2_operations WHERE key=$1`, key).Scan(&oldHash, &id, &saved)
	if e == nil {
		if oldHash != hash {
			return Instance{}, ErrConflict
		}
		var out Instance
		if json.Unmarshal(saved, &out) != nil {
			return out, ErrUnavailable
		}
		return out, nil
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return Instance{}, e
	}
	var out Instance
	if in.Kind == Install || in.Kind == Clone {
		if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(7062001)`); e != nil {
			return out, e
		}
		var count int
		if e = tx.QueryRow(ctx, `SELECT count(*) FROM plugin_v2_instances WHERE desired_state<>'absent'`).Scan(&count); e != nil {
			return out, e
		}
		if count >= 32 {
			return out, ErrInvalid
		}
		if in.Kind == Install && in.ExpectedRevision != 0 {
			return out, ErrInvalid
		}
		if in.Kind == Clone {
			if !validUUID(in.InstanceID) {
				return out, ErrInvalid
			}
			if _, e = tx.Exec(ctx, `SELECT id FROM plugin_v2_instances WHERE id=$1 FOR NO KEY UPDATE`, in.InstanceID); e != nil {
				return out, e
			}
			source, e := readInstance(ctx, tx, in.InstanceID)
			if e != nil {
				return out, e
			}
			if source.Revision != in.ExpectedRevision {
				return out, ErrConflict
			}
			in.PluginID = source.PluginID
			var cipher []byte
			if e = tx.QueryRow(ctx, `SELECT ciphertext FROM plugin_v2_configs WHERE id=$1`, in.InstanceID).Scan(&cipher); e != nil {
				return out, e
			}
			in.Config, e = s.open("config", in.InstanceID, cipher)
			if e != nil {
				return out, e
			}
		}
		d, e := definition(in.PluginID)
		if e != nil {
			return out, e
		}
		if d.Fingerprint != in.Fingerprint {
			return out, ErrConflict
		}
		requested := append([]string{}, in.Scopes...)
		slices.Sort(requested)
		if !slices.Equal(requested, scopes(d)) {
			return out, ErrInvalid
		}
		if len(in.Config) == 0 {
			in.Config = json.RawMessage(`{}`)
		}
		if contracts.ValidateJSON(in.Config) != nil || len(in.Config) > 128<<10 {
			return out, ErrInvalid
		}
		// Only the approved built-in capability subset is executable in F1.
		for _, p := range d.Manifest.Permissions {
			switch p.Scope {
			case "api.register", "api.public", "api.public.write", "services.register", "commands.register", "commands.override", "actions.register", "plugin.config.read", "plugin.config.write", "plugin.data.read", "plugin.data.write", "plugin.state.read", "fs.data", "ui.json", "ui.trusted", "ui.sidebar", "system.read", "notifications.send":
			default:
				if !strings.HasPrefix(p.Scope, "events.chain.") && !strings.HasPrefix(p.Scope, "events.subscribe.") && !strings.HasPrefix(p.Scope, "module.") {
					return out, ErrUnavailable
				}
			}
			if len(p.Resources) > 0 {
				return out, ErrInvalid
			}
		}
		id = jobs.UUID()
		cipher, e := s.seal("config", id, in.Config)
		if e != nil {
			return out, e
		}
		metadata, e := json.Marshal(d.Manifest)
		if e != nil {
			return out, e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO plugin_v2_definitions(id,manifest,fingerprint)VALUES($1,$2,$3) ON CONFLICT(id)DO NOTHING`, d.Manifest.ID, metadata, d.Fingerprint); e != nil {
			return out, e
		}
		var fp string
		if e = tx.QueryRow(ctx, `SELECT fingerprint FROM plugin_v2_definitions WHERE id=$1`, d.Manifest.ID).Scan(&fp); e != nil || fp != d.Fingerprint {
			return out, ErrConflict
		}
		cloneOf := any(nil)
		if in.Kind == Clone {
			cloneOf = in.InstanceID
		}
		if _, e = tx.Exec(ctx, `INSERT INTO plugin_v2_instances(id,plugin_id,approved_fingerprint,clone_of)VALUES($1,$2,$3,$4)`, id, d.Manifest.ID, d.Fingerprint, cloneOf); e != nil {
			return out, e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO plugin_v2_configs(id,ciphertext)VALUES($1,$2)`, id, cipher); e != nil {
			return out, e
		}
		for _, scope := range requested {
			if _, e = tx.Exec(ctx, `INSERT INTO plugin_v2_grants(instance_id,scope,fingerprint)VALUES($1,$2,$3)`, id, scope, d.Fingerprint); e != nil {
				return out, e
			}
		}
		if in.Kind == Clone {
			if e = s.cloneKV(ctx, tx, in.InstanceID, id); e != nil {
				return out, e
			}
		}
	} else {
		if !validUUID(in.InstanceID) {
			return out, ErrInvalid
		}
		id = in.InstanceID
		if _, e = tx.Exec(ctx, `SELECT id FROM plugin_v2_instances WHERE id=$1 FOR NO KEY UPDATE`, id); e != nil {
			return out, e
		}
		current, e := readInstance(ctx, tx, id)
		if e != nil {
			return out, e
		}
		if current.Revision != in.ExpectedRevision {
			return out, ErrConflict
		}
		d, e := definition(current.PluginID)
		if e != nil && (in.Kind == Disable || in.Kind == Remove) {
			if e = tx.QueryRow(ctx, `SELECT fingerprint FROM plugin_v2_definitions WHERE id=$1`, current.PluginID).Scan(&d.Fingerprint); e != nil {
				return out, e
			}
		} else if e != nil {
			return out, e
		}
		if in.Fingerprint != d.Fingerprint {
			return out, ErrConflict
		}
		switch in.Kind {
		case Review:
			requested := append([]string{}, in.Scopes...)
			slices.Sort(requested)
			if !slices.Equal(requested, scopes(d)) {
				return out, ErrInvalid
			}
			if e = reviewGrants(ctx, tx, id, d); e != nil {
				return out, e
			}
			if _, e = tx.Exec(ctx, `UPDATE plugin_v2_instances SET approved_fingerprint=$2,desired_state='disabled',runtime_state=CASE WHEN generation IS NULL THEN 'stopped' ELSE 'stopping' END,reason='',revision=revision+1,grant_revision=grant_revision+1,updated_at=now() WHERE id=$1`, id, d.Fingerprint); e != nil {
				return out, e
			}
		case Remove:
			if _, e = tx.Exec(ctx, `UPDATE plugin_v2_instances SET desired_state='absent',runtime_state=CASE WHEN generation IS NULL THEN 'stopped' ELSE 'stopping' END,revision=revision+1,grant_revision=grant_revision+1 WHERE id=$1`, id); e != nil {
				return out, e
			}
		case Enable, Disable:
			desired := Disabled
			if in.Kind == Enable {
				var approved string
				if e = tx.QueryRow(ctx, `SELECT approved_fingerprint FROM plugin_v2_instances WHERE id=$1`, id).Scan(&approved); e != nil {
					return out, e
				}
				if approved != d.Fingerprint {
					return out, ErrConflict
				}
				desired = Enabled
			}
			if _, e = tx.Exec(ctx, `UPDATE plugin_v2_instances SET desired_state=$2,runtime_state=CASE WHEN generation IS NOT NULL THEN 'stopping' WHEN approved_fingerprint<>$3 OR reason IN('permission_review_required','definition_missing','definition_unsupported') THEN 'needs_action' ELSE 'stopped' END,revision=revision+1,grant_revision=grant_revision+1,restart_count=0,next_restart_at=now(),reason=CASE WHEN reason IN('permission_review_required','definition_missing','definition_unsupported') THEN reason WHEN approved_fingerprint<>$3 THEN 'permission_review_required' ELSE '' END,updated_at=now() WHERE id=$1`, id, desired, d.Fingerprint); e != nil {
				return out, e
			}
		case Configure:
			if contracts.ValidateJSON(in.Config) != nil || len(in.Config) > 128<<10 {
				return out, ErrInvalid
			}
			cipher, e := s.seal("config", id, in.Config)
			if e != nil {
				return out, e
			}
			if _, e = tx.Exec(ctx, `UPDATE plugin_v2_configs SET ciphertext=$2,revision=revision+1 WHERE id=$1`, id, cipher); e != nil {
				return out, e
			}
			if _, e = tx.Exec(ctx, `UPDATE plugin_v2_instances SET revision=revision+1,grant_revision=grant_revision+1,runtime_state=CASE WHEN generation IS NULL THEN 'stopped' ELSE 'stopping' END,updated_at=now() WHERE id=$1`, id); e != nil {
				return out, e
			}
		default:
			return out, ErrInvalid
		}
	}
	out, e = readOperationInstance(ctx, tx, id)
	if e != nil {
		return out, e
	}
	saved, e = json.Marshal(out)
	if e != nil {
		return out, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO plugin_v2_operations(key,request_digest,kind,instance_id,result)VALUES($1,$2,$3,$4,$5)`, key, hash, in.Kind, id, saved); e != nil {
		return out, e
	}
	details, _ := json.Marshal(map[string]any{"instanceId": id, "kind": in.Kind, "fingerprint": in.Fingerprint})
	if _, e = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES($1,'plugins.v2.operation',$2)`, actor, details); e != nil {
		return out, e
	}
	out, e = readOperationInstance(ctx, tx, id)
	if e != nil {
		return out, e
	}
	if e = tx.Commit(ctx); e != nil {
		return out, e
	}
	return out, nil
}
func readInstance(ctx context.Context, tx pgx.Tx, id string) (Instance, error) {
	var out Instance
	e := tx.QueryRow(ctx, `SELECT id::text,plugin_id,desired_state,runtime_state,revision,reason FROM plugin_v2_instances WHERE id=$1 AND desired_state<>'absent'`, id).Scan(&out.ID, &out.PluginID, &out.Desired, &out.State, &out.Revision, &out.Reason)
	if errors.Is(e, pgx.ErrNoRows) {
		e = ErrNotFound
	}
	return out, e
}

func (s Store) cloneKV(ctx context.Context, tx pgx.Tx, source, target string) error {
	rows, e := tx.Query(ctx, `SELECT id::text,key,ciphertext,revision FROM plugin_v2_kv WHERE instance_id=$1 ORDER BY id LIMIT 1025`, source)
	if e != nil {
		return e
	}
	type entry struct {
		id, key  string
		cipher   []byte
		revision int64
	}
	entries := []entry{}
	for rows.Next() {
		var v entry
		if e = rows.Scan(&v.id, &v.key, &v.cipher, &v.revision); e != nil {
			break
		}
		entries = append(entries, v)
	}
	other := rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	if other != nil {
		return other
	}
	if len(entries) > 1024 {
		return ErrInvalid
	}
	for _, v := range entries {
		raw, e := s.open("kv", v.id, v.cipher)
		if e != nil {
			return e
		}
		id := jobs.UUID()
		cipher, e := s.seal("kv", id, raw)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO plugin_v2_kv(id,instance_id,key,ciphertext,revision)VALUES($1,$2,$3,$4,1)`, id, target, v.key, cipher); e != nil {
			return e
		}
	}
	return nil
}

func readOperationInstance(ctx context.Context, tx pgx.Tx, id string) (Instance, error) {
	var out Instance
	e := tx.QueryRow(ctx, `SELECT id::text,plugin_id,desired_state,runtime_state,revision,reason FROM plugin_v2_instances WHERE id=$1`, id).Scan(&out.ID, &out.PluginID, &out.Desired, &out.State, &out.Revision, &out.Reason)
	return out, e
}
