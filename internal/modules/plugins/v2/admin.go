package v2

import (
	"context"
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
)

type DataEntry struct {
	Key      string          `json:"key"`
	Value    json.RawMessage `json:"value"`
	Revision int64           `json:"revision"`
}
type Inspection struct {
	Instance    Instance         `json:"instance"`
	Config      contracts.Config `json:"config"`
	Entries     []DataEntry      `json:"entries"`
	Next        string           `json:"next"`
	Fingerprint string           `json:"fingerprint"`
}

// Inspect is administrator-only and bounded, never invoked by the media port.
func (s Store) Inspect(ctx context.Context, id, after string) (Inspection, error) {
	out := Inspection{Entries: []DataEntry{}}
	if !validUUID(id) || len(after) > 128 {
		return out, ErrInvalid
	}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return out, e
	}
	defer rollback(ctx, tx)
	out.Instance, e = readInstance(ctx, tx, id)
	if e != nil {
		return out, e
	}
	if e = tx.QueryRow(ctx, `SELECT d.fingerprint FROM plugin_v2_definitions d WHERE d.id=$1`, out.Instance.PluginID).Scan(&out.Fingerprint); e != nil {
		return out, e
	}
	var cipher []byte
	if e = tx.QueryRow(ctx, `SELECT ciphertext,revision FROM plugin_v2_configs WHERE id=$1`, id).Scan(&cipher, &out.Config.Revision); e != nil {
		return out, e
	}
	out.Config.Value, e = s.open("config", id, cipher)
	if e != nil {
		return out, e
	}
	rows, e := tx.Query(ctx, `SELECT id::text,key,ciphertext,revision FROM plugin_v2_kv WHERE instance_id=$1 AND key>$2 ORDER BY key LIMIT 51`, id, after)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var v DataEntry
		var key string
		if e = rows.Scan(&key, &v.Key, &cipher, &v.Revision); e != nil {
			return out, e
		}
		if len(out.Entries) == 50 {
			out.Next = out.Entries[49].Key
			break
		}
		v.Value, e = s.open("kv", key, cipher)
		if e != nil {
			return out, e
		}
		out.Entries = append(out.Entries, v)
	}
	return out, rows.Err()
}
