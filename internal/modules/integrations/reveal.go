package integrations

import (
	"context"
	"encoding/json"
)

// Reveal is an explicitly authorized management operation, never part of List.
// Only declared operator secrets are visible, not refresh bookkeeping/master keys.
func (r Repository) Reveal(ctx context.Context, id string, revision int64, fields []string) (map[string]string, error) {
	if revision < 1 || len(fields) == 0 || len(fields) > 16 {
		return nil, ErrInvalid
	}
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer tx.Rollback(ctx)
	c, err := r.scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM integrations WHERE id=$1 FOR SHARE`, id))
	if err != nil {
		return nil, err
	}
	if c.Record.Revision != revision {
		return nil, ErrRevision
	}
	out := map[string]string{}
	seen := map[string]bool{}
	for _, name := range fields {
		if !contains(secretKeys[c.Record.Kind], name) || seen[name] {
			return nil, ErrInvalid
		}
		seen[name] = true
		if value, exists := c.Values[name]; exists {
			out[name] = value
		}
	}
	raw, _ := json.Marshal(map[string]any{"integrationId": id, "fields": fields})
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','integration.secrets.reveal',$1)`, raw); err != nil {
		return nil, ErrUnavailable
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, ErrUnavailable
	}
	return out, nil
}
