package v2

import (
	"context"
	"encoding/json"
)

// Encrypted configurations cannot be inspected in SQL migrations. Retire only
// the external provider after opening with the existing vault, before workers.
// Preserve the ciphertext and plugin KV; never reinterpret it as builtin.
func (s Store) retireExternalMediaConfigs(ctx context.Context) error {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer rollback(ctx, tx)
	rows, e := tx.Query(ctx, `SELECT i.id::text,c.ciphertext FROM plugin_v2_instances i JOIN plugin_v2_configs c ON c.id=i.id WHERE i.plugin_id='library-refresh-notify' AND i.desired_state<>'absent' FOR UPDATE OF i,c`)
	if e != nil {
		return e
	}
	ids := []string{}
	for rows.Next() {
		var id string
		var cipher []byte
		if e = rows.Scan(&id, &cipher); e != nil {
			rows.Close()
			return e
		}
		raw, err := s.open("config", id, cipher)
		if err != nil {
			rows.Close()
			return err
		}
		var value struct {
			Provider string `json:"provider"`
		}
		if json.Unmarshal(raw, &value) != nil {
			rows.Close()
			return ErrInvalid
		}
		if value.Provider == "emby" {
			ids = append(ids, id)
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, id := range ids {
		if _, e = tx.Exec(ctx, `UPDATE plugin_v2_instances SET desired_state='disabled',reason='external_media_removed',revision=revision+1,updated_at=now() WHERE id=$1 AND (desired_state<>'disabled' OR reason<>'external_media_removed')`, id); e != nil {
			return e
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return e
	}
	return nil
}
