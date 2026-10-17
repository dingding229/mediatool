package catalog

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"media-tool/internal/modules/nativelibrary"
)

// The normal browse path is read-only. Missing IDs are repaired for this page
// (<=200 items plus their ancestors), never for the whole visible collection.
func (r BrowseRepository) pageIDs(ctx context.Context, keys []nativelibrary.Key, pageSize int) (map[nativelibrary.Key]int64, error) {
	unique := map[nativelibrary.Key]bool{}
	for _, k := range keys {
		unique[k] = true
	}
	wanted := make([]nativelibrary.Key, 0, len(unique))
	for k := range unique {
		wanted = append(wanted, k)
	}
	raw, err := json.Marshal(wanted)
	if err != nil {
		return nil, err
	}
	rows, err := r.DB.Query(ctx, `SELECT n.entity_type,n.internal_key,n.id,n.retired_at FROM native_item_ids n JOIN jsonb_to_recordset($1) k(entity_type text,internal_key text) ON n.entity_type=k.entity_type AND n.internal_key=k.internal_key LIMIT 600`, raw)
	if err != nil {
		return nil, err
	}
	ids := map[nativelibrary.Key]int64{}
	for rows.Next() {
		var k nativelibrary.Key
		var id int64
		var retired *time.Time
		if err := rows.Scan(&k.EntityType, &k.InternalKey, &id, &retired); err != nil {
			rows.Close()
			return nil, err
		}
		if retired == nil {
			ids[k] = id
			delete(unique, k)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(unique) > 0 {
		logger := r.Logger
		if logger == nil {
			logger = slog.Default()
		}
		warning := r.warning
		if warning == nil {
			warning = &defaultBrowseWarning
		}
		warning.Do(func() {
			logger.Warn("Native browse ID preallocation missing; repairing current page only", "module", "native_media", "pageItems", pageSize, "missingKeys", len(unique))
		})
		missing := make([]nativelibrary.Key, 0, len(unique))
		for k := range unique {
			missing = append(missing, k)
		}
		for len(missing) > 0 {
			n := min(500, len(missing))
			batch, err := nativelibrary.EnsureIDs(ctx, IDQueries{DB: r.DB}, missing[:n])
			if err != nil {
				return nil, err
			}
			for k, id := range batch {
				ids[k] = id
			}
			missing = missing[n:]
		}
	}
	return ids, nil
}
