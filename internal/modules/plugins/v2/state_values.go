package v2

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"media-tool/internal/modules/jobs"
	"strings"
)

func (s Store) captureStateValues(ctx context.Context, tx pgx.Tx, id, snapshot string) error {
	rows, e := tx.Query(ctx, `SELECT 'config','',id::text,ciphertext,revision FROM plugin_v2_configs WHERE id=$1 UNION ALL SELECT 'kv',key,id::text,ciphertext,revision FROM plugin_v2_kv WHERE instance_id=$1 AND key NOT LIKE '.host/%'`, id)
	if e != nil {
		return e
	}
	type value struct {
		kind, key, id string
		cipher        []byte
		rev           int64
	}
	values := []value{}
	var size int
	for rows.Next() {
		var v value
		if e = rows.Scan(&v.kind, &v.key, &v.id, &v.cipher, &v.rev); e != nil {
			break
		}
		size += len(v.cipher)
		if size > 12<<20 || len(values) >= 1025 {
			e = ErrInvalid
			break
		}
		values = append(values, v)
	}
	other := rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	if other != nil {
		return other
	}
	for _, v := range values {
		raw, e := s.open(v.kind, v.id, v.cipher)
		if e != nil {
			return e
		}
		key := jobs.UUID()
		cipher, e := s.Vault.Seal("plugin:v2:state-value:"+key, map[string]string{"json": string(raw)})
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO plugin_v2_state_values(id,snapshot_id,kind,key,revision,ciphertext)VALUES($1,$2,$3,$4,$5,$6)`, key, snapshot, v.kind, v.key, v.rev, cipher); e != nil {
			return e
		}
	}
	return nil
}
func (s Store) restoreStateValues(ctx context.Context, tx pgx.Tx, id, snapshot string) error {
	rows, e := tx.Query(ctx, `SELECT id::text,kind,key,ciphertext,revision FROM plugin_v2_state_values WHERE snapshot_id=$1 ORDER BY kind,key`, snapshot)
	if e != nil {
		return e
	}
	type value struct {
		kind, key string
		raw       json.RawMessage
		rev       int64
	}
	values := []value{}
	config := false
	for rows.Next() {
		var key, kind, name string
		var cipher []byte
		var rev int64
		if e = rows.Scan(&key, &kind, &name, &cipher, &rev); e != nil {
			break
		}
		var plain map[string]string
		plain, e = s.Vault.Open("plugin:v2:state-value:"+key, cipher)
		if e != nil {
			break
		}
		values = append(values, value{kind, name, json.RawMessage(plain["json"]), rev})
		config = config || kind == "config"
	}
	other := rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	if other != nil {
		return other
	}
	if !config {
		return ErrInvalid
	}
	for _, v := range values {
		if !json.Valid(v.raw) {
			return ErrInvalid
		}
		if v.kind == "config" {
			cipher, e := s.seal("config", id, v.raw)
			if e != nil {
				return e
			}
			if _, e = tx.Exec(ctx, `UPDATE plugin_v2_configs SET ciphertext=$2,revision=revision+1 WHERE id=$1`, id, cipher); e != nil {
				return e
			}
		}
	}
	revisions := map[string]int64{}
	rows, e = tx.Query(ctx, `SELECT key,revision FROM plugin_v2_kv WHERE instance_id=$1`, id)
	if e != nil {
		return e
	}
	for rows.Next() {
		var k string
		var rev int64
		if e = rows.Scan(&k, &rev); e != nil {
			break
		}
		revisions[k] = rev
	}
	other = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	if other != nil {
		return other
	}
	if _, e = tx.Exec(ctx, `DELETE FROM plugin_v2_kv WHERE instance_id=$1 AND key NOT LIKE '.host/%'`, id); e != nil {
		return e
	}
	var retainedCount int
	var retainedBytes int64
	if e = tx.QueryRow(ctx, `SELECT count(*),COALESCE(sum(octet_length(ciphertext)),0) FROM plugin_v2_kv WHERE instance_id=$1 AND key LIKE '.host/%'`, id).Scan(&retainedCount, &retainedBytes); e != nil {
		return e
	}
	for _, v := range values {
		if v.kind != "kv" {
			continue
		}
		if strings.HasPrefix(v.key, ".host/") {
			return ErrInvalid
		}
		key := jobs.UUID()
		cipher, e := s.seal("kv", key, v.raw)
		if e != nil {
			return e
		}
		retainedCount++
		retainedBytes += int64(len(cipher))
		if retainedCount > 1024 || retainedBytes > 10<<20 {
			return ErrInvalid
		}
		if _, e = tx.Exec(ctx, `INSERT INTO plugin_v2_kv(id,instance_id,key,ciphertext,revision)VALUES($1,$2,$3,$4,$5)`, key, id, v.key, cipher, max(v.rev, revisions[v.key])+1); e != nil {
			return e
		}
	}
	return nil
}
