package secrets

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CipherColumn struct {
	Table, Column string
	AADPrefix     string
	AllowEmpty    bool // terminal/claimed authorization rows intentionally erase their payload
}

var CipherColumns = []CipherColumn{
	{Table: "manual_download_tasks", Column: "payload_ciphertext", AADPrefix: "manual-download:"},
	{Table: "external_api_keys", Column: "token_ciphertext", AADPrefix: "api-key:"},
	{Table: "network_proxy_settings", Column: "url_ciphertext", AADPrefix: "network-proxy:", AllowEmpty: true},
	{Table: "extension_plugins", Column: "plugin_data_ciphertext", AADPrefix: "plugin-data:", AllowEmpty: true},
	{Table: "extension_plugins", Column: "config_ciphertext", AADPrefix: "plugin:", AllowEmpty: true},
	{Table: "integrations", Column: "credentials_ciphertext"},
	{Table: "resources", Column: "download_hint_ciphertext"},
	{Table: "search_run_resources", Column: "download_hint_ciphertext", AADPrefix: "search-run:"},
	{Table: "playback_grants", Column: "token_ciphertext"},
	{Table: "media_artifact_journal", Column: "content_ciphertext"},
	{Table: "site_accounts", Column: "credentials_ciphertext"},
	{Table: "storage_connections", Column: "credentials_ciphertext"},
	{Table: "notification_channels", Column: "credentials_ciphertext"},
	{Table: "open115_authorizations", Column: "payload_ciphertext", AADPrefix: "open115-auth:", AllowEmpty: true},
}

// Rekey is all-or-nothing across every persisted ciphertext, retaining IDs/AAD.
// Call only with application and workers stopped; dryRun rolls back the writes.
func Rekey(ctx context.Context, db *pgxpool.Pool, old, next *Vault, dryRun bool) (map[string]int, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	counts := map[string]int{}
	for _, spec := range CipherColumns {
		table := pgx.Identifier{spec.Table}.Sanitize()
		column := pgx.Identifier{spec.Column}.Sanitize()
		if _, err = tx.Exec(ctx, "LOCK TABLE "+table+" IN ACCESS EXCLUSIVE MODE"); err != nil {
			return nil, err
		}
		countsKey := spec.Table
		if _, exists := counts[countsKey]; exists {
			countsKey += "." + spec.Column
		}
		counts[countsKey] = 0
		condition := column + " IS NOT NULL"
		if spec.AllowEmpty {
			condition += " AND octet_length(" + column + ")>0"
		}
		last := ""
		for {
			rows, err := tx.Query(ctx, "SELECT id::text,"+column+" FROM "+table+" WHERE "+condition+" AND id::text>$1 ORDER BY id::text LIMIT 250", last)
			if err != nil {
				return nil, err
			}
			type entry struct {
				id        string
				encrypted []byte
			}
			batch := []entry{}
			for rows.Next() {
				var item entry
				if err = rows.Scan(&item.id, &item.encrypted); err != nil {
					rows.Close()
					return nil, err
				}
				batch = append(batch, item)
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return nil, err
			}
			if len(batch) == 0 {
				break
			}
			for _, item := range batch {
				values, err := old.Open(spec.AADPrefix+item.id, item.encrypted)
				if err != nil {
					return nil, ErrDecrypt
				}
				encrypted, err := next.Seal(spec.AADPrefix+item.id, values)
				if err != nil {
					return nil, err
				}
				if _, err = tx.Exec(ctx, "UPDATE "+table+" SET "+column+"=$2 WHERE id::text=$1", item.id, encrypted); err != nil {
					return nil, err
				}
				counts[countsKey]++
				last = item.id
			}
		}
	}
	if dryRun {
		return counts, tx.Rollback(ctx)
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('offline-operator','credentials.master_key_rotated','{}')`)
	if err != nil {
		return nil, err
	}
	return counts, tx.Commit(ctx)
}
