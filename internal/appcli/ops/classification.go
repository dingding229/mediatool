package ops

import (
	"context"
	"encoding/json"
	"io"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/nativetmdb"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/secrets"
)

func classificationTargets(raw string) ([]nativetmdb.Target, error) {
	parts := strings.Split(raw, ",")
	if len(parts) == 0 || len(parts) > 20 {
		return nil, nativetmdb.ErrInvalid
	}
	out := []nativetmdb.Target{}
	seen := map[string]bool{}
	for _, s := range parts {
		pair := strings.Split(s, ":")
		if len(pair) != 2 || seen[s] {
			return nil, nativetmdb.ErrInvalid
		}
		id, e := strconv.ParseInt(pair[1], 10, 64)
		t := nativetmdb.Target{Type: pair[0], TMDBID: id}
		if e != nil || !t.Valid() || t.Type == "season" || strconv.FormatInt(id, 10) != pair[1] {
			return nil, nativetmdb.ErrInvalid
		}
		out = append(out, t)
		seen[s] = true
	}
	return out, nil
}

// An explicit owner CLI, not an HTTP/editor endpoint. Never accepts provider
// URLs/credentials or arbitrary JSON. Each identity is revalidated by the engine.
func refreshClassification(ctx context.Context, db *pgxpool.Pool, c config.Config, raw string, w io.Writer) error {
	targets, err := classificationTargets(raw)
	if err != nil {
		return err
	}
	vault, err := secrets.New(c.SecretKey)
	if err != nil {
		return nativetmdb.ErrClassificationRefresh
	}
	manager := integrations.NewManager(integrations.Repository{DB: db, Vault: vault}, c)
	defer manager.Close()
	engine := nativetmdb.Engine{DB: db, Connections: manager}
	receipts := []nativetmdb.ClassificationRefresh{}
	for _, t := range targets {
		result, e := engine.RefreshClassification(ctx, t)
		if e != nil {
			return e
		}
		receipts = append(receipts, result)
	}
	return json.NewEncoder(w).Encode(map[string]any{"classificationOnly": true, "items": receipts})
}
