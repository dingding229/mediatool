package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/classification"
	"media-tool/internal/modules/nativelibrary"
	"time"
)

type Repository struct{ DB *pgxpool.Pool }

func (r Repository) Sync(ctx context.Context, policy classification.Policy, revision int64, granularity nativelibrary.LibraryGranularity) error {
	plans, err := Plan(policy, granularity)
	if err != nil {
		return err
	}
	if r.DB == nil {
		return nativelibrary.ErrSettingsUnavailable
	}
	bounded, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	tx, err := r.DB.Begin(bounded)
	if err != nil {
		return err
	}
	defer func() {
		if rollbackErr := tx.Rollback(context.Background()); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			// The commit/returned operation error is authoritative; no second error can be returned here.
		}
	}()
	if _, err = tx.Exec(bounded, `SET LOCAL lock_timeout='2s'; SET LOCAL statement_timeout='8s'`); err != nil {
		return err
	}
	raw, err := json.Marshal(plans)
	if err != nil {
		return err
	}
	if err = syncLibraries(bounded, tx, raw, revision); err != nil {
		return err
	}
	return tx.Commit(bounded)
}
func (r Repository) SyncCurrent(ctx context.Context) error {
	_, err := r.sync(ctx, nil, 0, nil, 0)
	return err
}
func (r Repository) SavePolicy(ctx context.Context, p classification.Policy, revision int64) (classification.Record, error) {
	return r.sync(ctx, &p, revision, nil, 0)
}

// T45's future settings editor must call this method rather than updating the
// column directly: changing granularity and syncing libraries are one commit.
func (r Repository) SetGranularity(ctx context.Context, g nativelibrary.LibraryGranularity, revision int64) error {
	_, err := r.sync(ctx, nil, 0, &g, revision)
	return err
}
func (r Repository) sync(ctx context.Context, p *classification.Policy, revision int64, g *nativelibrary.LibraryGranularity, settingsRevision int64) (record classification.Record, err error) {
	if r.DB == nil {
		return record, nativelibrary.ErrSettingsUnavailable
	}
	bounded, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	tx, err := r.DB.Begin(bounded)
	if err != nil {
		return record, err
	}
	defer func() {
		cleanup, done := context.WithTimeout(context.Background(), 2*time.Second)
		defer done()
		if e := tx.Rollback(cleanup); e != nil && !errors.Is(e, pgx.ErrTxClosed) {
			err = errors.Join(err, e)
		}
	}()
	if _, err = tx.Exec(bounded, `SET LOCAL lock_timeout='2s'; SET LOCAL statement_timeout='8s'`); err != nil {
		return record, err
	}
	// The same order is used by startup, classification saves and granularity
	// changes. These singleton row locks are ONLY on admin/startup writes.
	var raw []byte
	if err = tx.QueryRow(bounded, `SELECT value,revision FROM directory_classification WHERE singleton FOR UPDATE`).Scan(&raw, &record.Revision); err != nil {
		return record, err
	}
	record, err = classification.DecodeRecord(raw, record.Revision)
	if err != nil {
		return record, err
	}
	var granularity nativelibrary.LibraryGranularity
	var currentSettingsRevision int64
	if err = tx.QueryRow(bounded, `SELECT library_granularity,revision FROM native_server_settings WHERE singleton FOR UPDATE`).Scan(&granularity, &currentSettingsRevision); err != nil {
		return record, err
	}
	if p != nil {
		if record.Revision != revision {
			return record, ErrConflict
		}
		raw, err = json.Marshal(p)
		if err != nil {
			return record, ErrInvalid
		}
		record, err = classification.DecodeRecord(raw, revision+1)
		if err != nil {
			return record, ErrInvalid
		}
		tag, e := tx.Exec(bounded, `UPDATE directory_classification SET value=$1,revision=$2,updated_at=now() WHERE singleton AND revision=$3`, raw, record.Revision, revision)
		if e != nil {
			return record, e
		}
		if tag.RowsAffected() != 1 {
			return record, ErrConflict
		}
	}
	if g != nil {
		if *g != nativelibrary.GranularityRule && *g != nativelibrary.GranularityGroup {
			return record, ErrInvalid
		}
		if settingsRevision > 0 && currentSettingsRevision != settingsRevision {
			return record, ErrConflict
		}
		if granularity != *g {
			tag, e := tx.Exec(bounded, `UPDATE native_server_settings SET library_granularity=$1,revision=revision+1,updated_at=now() WHERE singleton AND revision=$2`, *g, settingsRevision)
			if e != nil {
				return record, e
			}
			if tag.RowsAffected() != 1 {
				return record, ErrConflict
			}
		}
		granularity = *g
	}
	plans, err := Plan(record.Value, granularity)
	if err != nil {
		return record, err
	}
	raw, err = json.Marshal(plans)
	if err != nil {
		return record, err
	}
	if err = syncLibraries(bounded, tx, raw, record.Revision); err != nil {
		return record, err
	}
	err = tx.Commit(bounded)
	return record, err
}
func syncLibraries(ctx context.Context, tx pgx.Tx, plans []byte, revision int64) error {
	// Existing keys are filtered BEFORE nextval, so a no-op sync does not even
	// consume sequence values. Concurrent allocators still have ON CONFLICT safety.
	if _, err := tx.Exec(ctx, `INSERT INTO native_item_ids(entity_type,internal_key)
  SELECT 'library',p.id FROM jsonb_to_recordset($1::jsonb) AS p(id text,"sortOrder" int)
  WHERE NOT EXISTS(SELECT 1 FROM native_item_ids n WHERE n.entity_type='library' AND n.internal_key=p.id)
  ORDER BY p."sortOrder" ON CONFLICT(entity_type,internal_key) DO NOTHING`, string(plans)); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO native_libraries(id,item_id,group_id,rule_id,name,directory,collection_type,hidden,sort_order,policy_revision)
  SELECT p.id,n.id,p."groupId",NULLIF(p."ruleId",''),p.name,p.directory,p."collectionType",false,p."sortOrder",$2
  FROM jsonb_to_recordset($1::jsonb) AS p(id text,"groupId" text,"ruleId" text,name text,directory text,"collectionType" text,"sortOrder" int)
  JOIN native_item_ids n ON n.entity_type='library' AND n.internal_key=p.id
  ON CONFLICT(id) DO UPDATE SET name=EXCLUDED.name,directory=EXCLUDED.directory,collection_type=EXCLUDED.collection_type,hidden=false,sort_order=EXCLUDED.sort_order,policy_revision=EXCLUDED.policy_revision,updated_at=now()
  WHERE (native_libraries.name,native_libraries.directory,native_libraries.collection_type,native_libraries.hidden,native_libraries.sort_order,native_libraries.policy_revision)
   IS DISTINCT FROM (EXCLUDED.name,EXCLUDED.directory,EXCLUDED.collection_type,false,EXCLUDED.sort_order,EXCLUDED.policy_revision)`, string(plans), revision); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE native_libraries SET hidden=true,policy_revision=$2,updated_at=now()
  WHERE NOT hidden AND id NOT IN('f:movie','f:series') AND NOT EXISTS(SELECT 1 FROM jsonb_to_recordset($1::jsonb)AS p(id text) WHERE p.id=native_libraries.id)`, string(plans), revision); err != nil {
		return err
	}
	// Fallbacks are assignment-owned, never classification-plan members. Repair
	// their persisted visibility in this transaction, including on startup, and
	// before expanding legacy grants to the resulting visible libraries.
	if err := hideFallback(ctx, tx); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT user_id,media_type,can_play FROM media_user_legacy_grants FOR UPDATE`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type legacyGrant struct {
		userID, mediaType string
		canPlay           bool
	}
	pending := []legacyGrant{}
	for rows.Next() {
		var g legacyGrant
		if err := rows.Scan(&g.userID, &g.mediaType, &g.canPlay); err != nil {
			return err
		}
		pending = append(pending, g)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, g := range pending {
		collection := "movies"
		if g.mediaType == "series" {
			collection = "tvshows"
		}
		var visible int64
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM native_libraries WHERE NOT hidden AND collection_type=$1`, collection).Scan(&visible); err != nil {
			return err
		}
		if visible == 0 {
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO media_user_library_grants(user_id,library_id,can_play,can_download)
          SELECT $1,id,$2,false FROM native_libraries WHERE NOT hidden AND collection_type=$3 ON CONFLICT(user_id,library_id) DO NOTHING`, g.userID, g.canPlay, collection); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM media_user_legacy_grants WHERE user_id=$1 AND media_type=$2`, g.userID, g.mediaType); err != nil {
			return err
		}
	}
	return err
}
func (r Repository) List(ctx context.Context, start, limit int) (Page, error) {
	out := Page{Items: []Library{}, StartIndex: start, Limit: limit}
	if start < 0 || start > 1000000 || limit < 1 || limit > 200 {
		return out, ErrInvalid
	}
	if r.DB == nil {
		return out, nativelibrary.ErrSettingsUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	policy, err := (classification.Repository{DB: r.DB}).Get(ctx)
	if err != nil {
		return out, err
	}
	groups, rules := map[string]string{}, map[string]string{}
	for _, g := range policy.Value.Groups {
		groups[g.ID] = g.Name
		for _, rule := range g.Rules {
			rules[g.ID+":"+rule.ID] = rule.Name
		}
	}
	if err = r.DB.QueryRow(ctx, `SELECT count(*) FROM native_libraries`).Scan(&out.Total); err != nil {
		return out, err
	}
	rows, err := r.DB.Query(ctx, `SELECT l.id,l.item_id::text,l.name,l.collection_type,l.directory,l.hidden,l.sort_order,coalesce(l.group_id,''),coalesce(l.rule_id,''),coalesce((SELECT count(*) FROM native_asset_libraries a JOIN (`+ExposableAssetsSQL+`) e ON e.asset_id=a.asset_id WHERE a.library_id=l.id),0) FROM native_libraries l ORDER BY hidden,sort_order,id LIMIT $1 OFFSET $2`, limit, start)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var item Library
		if err = rows.Scan(&item.ID, &item.ItemID, &item.Name, &item.CollectionType, &item.Directory, &item.Hidden, &item.SortOrder, &item.GroupID, &item.RuleID, &item.AssetCount); err != nil {
			return out, err
		}
		item.GroupName = groups[item.GroupID]
		item.RuleName = rules[item.GroupID+":"+item.RuleID]
		out.Items = append(out.Items, item)
	}
	return out, rows.Err()
}
