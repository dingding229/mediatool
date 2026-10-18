// Package pluginstate stores only non-secret plugin-framework state in the
// existing generic KV. No other application setting or SQL is exposed to plugins.
package pluginstate

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"strings"
)

func valid(key string) bool {
	return strings.HasPrefix(key, "plugins.") && len(key) <= 300 && !strings.ContainsAny(key, "\x00\r\n")
}
func Lock(ctx context.Context, tx pgx.Tx, key string, value any) error {
	if !valid(key) {
		return errors.New("invalid plugin state key")
	}
	seed, _ := json.Marshal(value)
	if _, e := tx.Exec(ctx, `INSERT INTO app_settings(key,value)VALUES($1,$2)ON CONFLICT DO NOTHING`, key, seed); e != nil {
		return e
	}
	var raw []byte
	if e := tx.QueryRow(ctx, `SELECT value FROM app_settings WHERE key=$1 FOR UPDATE`, key).Scan(&raw); e != nil {
		return e
	}
	return json.Unmarshal(raw, value)
}
func Put(ctx context.Context, tx pgx.Tx, key string, value any) error {
	raw, e := json.Marshal(value)
	if e != nil || !valid(key) || len(raw) > 32768 {
		return errors.New("invalid plugin framework state")
	}
	_, e = tx.Exec(ctx, `UPDATE app_settings SET value=$2,updated_at=clock_timestamp() WHERE key=$1`, key, raw)
	return e
}
