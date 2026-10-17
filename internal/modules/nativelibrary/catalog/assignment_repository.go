package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/classification"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/metadata"
	"media-tool/internal/modules/nativelibrary"
	"time"
)

type AssignmentRepository struct{ DB *pgxpool.Pool }
type Asset struct {
	ID, Title, MediaType string
	Revision             int64
	Metadata             json.RawMessage
	Outputs              []string
}
type AssetView struct {
	AssetID     string         `json:"assetId"`
	Title       string         `json:"title"`
	OutputPaths []string       `json:"outputPaths"`
	Source      string         `json:"source"`
	Diagnostic  map[string]any `json:"diagnostic"`
}
type AssetPage struct {
	Items      []AssetView `json:"items"`
	Total      int64       `json:"total"`
	StartIndex int         `json:"startIndex"`
	Limit      int         `json:"limit"`
}

func (r AssignmentRepository) Assign(ctx context.Context, assetID string, assetRevision int64) error {
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
		}
	}()
	var asset Asset
	var outputsRaw []byte
	err = tx.QueryRow(bounded, `WITH eligible AS (`+ExposableAssetsSQL+`) SELECT e.asset_id::text,e.revision,e.title,e.media_type,e.metadata,jsonb_agg(s.output_path ORDER BY s.output_path)
  FROM eligible e JOIN strm_entries s ON s.asset_id=e.asset_id AND s.output_path<>'' AND lower(s.output_path) LIKE '%.strm'
  WHERE e.asset_id=$1 GROUP BY e.asset_id,e.revision,e.title,e.media_type,e.metadata`, assetID).Scan(&asset.ID, &asset.Revision, &asset.Title, &asset.MediaType, &asset.Metadata, &outputsRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		if _, err = tx.Exec(bounded, `DELETE FROM native_asset_libraries WHERE asset_id=$1`, assetID); err != nil {
			return err
		}
		if err = hideFallback(bounded, tx); err != nil {
			return err
		}
		return tx.Commit(bounded)
	}
	if err != nil {
		return err
	}
	if assetRevision > 0 && asset.Revision != assetRevision {
		return nil
	}
	if err = json.Unmarshal(outputsRaw, &asset.Outputs); err != nil {
		return err
	}
	libs, err := loadLibraries(bounded, tx)
	if err != nil {
		return err
	}
	outputs := make([]Output, 0, len(asset.Outputs))
	for _, p := range asset.Outputs {
		outputs = append(outputs, Output{AssetID: asset.ID, Path: p})
	}
	assignment := MatchOutputs(outputs, libs, asset.MediaType)
	if assignment.LibraryID == "" {
		assignment.LibraryID = FallbackLibraryID(asset.MediaType)
		if assignment.LibraryID != "" && len(outputs) > 0 {
			assignment.OutputPath = outputs[0].Path
		}
	}
	if assignment.LibraryID == "" || len(outputs) == 0 {
		if _, err = tx.Exec(bounded, `DELETE FROM native_asset_libraries WHERE asset_id=$1`, asset.ID); err != nil {
			return err
		}
		if err = hideFallback(bounded, tx); err != nil {
			return err
		}
		return tx.Commit(bounded)
	}
	diagnostic := assignment.Diagnostic
	if diagnostic == nil {
		diagnostic = map[string]any{}
	}
	if err = classificationDiagnosis(bounded, tx, asset, assignment, libs, diagnostic); err != nil {
		return err
	}
	var policyRevision int64
	if err = tx.QueryRow(bounded, `SELECT coalesce(max(policy_revision),1) FROM native_libraries`).Scan(&policyRevision); err != nil {
		return err
	}
	diagnosticRaw, marshalErr := json.Marshal(diagnostic)
	if marshalErr != nil {
		return marshalErr
	}
	if _, err = tx.Exec(bounded, `INSERT INTO native_asset_libraries(asset_id,library_id,source,policy_revision,asset_revision,diagnostic) VALUES($1,$2,'output_directory',$3,$4,$5) ON CONFLICT(asset_id) DO UPDATE SET library_id=excluded.library_id,source=excluded.source,policy_revision=excluded.policy_revision,asset_revision=excluded.asset_revision,diagnostic=excluded.diagnostic,updated_at=now()`, asset.ID, assignment.LibraryID, policyRevision, asset.Revision, diagnosticRaw); err != nil {
		return err
	}
	if err = hideFallback(bounded, tx); err != nil {
		return err
	}
	if err = EnsureAssetIDs(bounded, tx, asset.ID); err != nil {
		return err
	}
	return tx.Commit(bounded)
}
func loadLibraries(ctx context.Context, tx pgx.Tx) ([]Library, error) {
	rows, err := tx.Query(ctx, `SELECT id,name,collection_type,directory,hidden,sort_order,coalesce(group_id,''),coalesce(rule_id,''),coalesce(item_id::text,'') FROM native_libraries ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Library{}
	for rows.Next() {
		var x Library
		if err := rows.Scan(&x.ID, &x.Name, &x.CollectionType, &x.Directory, &x.Hidden, &x.SortOrder, &x.GroupID, &x.RuleID, &x.ItemID); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func classificationDiagnosis(ctx context.Context, tx pgx.Tx, asset Asset, assignment Assignment, libs []Library, diag map[string]any) error {
	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT value FROM directory_classification WHERE singleton`).Scan(&raw); err != nil {
		diag["classificationError"] = "policy_unavailable"
		return nil
	}
	record, err := classification.DecodeRecord(raw, 1)
	if err != nil {
		diag["classificationError"] = "policy_invalid"
		return nil
	}
	var item metadata.Item
	if err = json.Unmarshal(asset.Metadata, &item); err != nil {
		diag["classificationError"] = "metadata_invalid"
		return nil
	}
	item.Title = asset.Title
	item.MediaType = asset.MediaType
	match := record.Value.Match(item)
	selected := ""
	for _, l := range libs {
		if l.ID == assignment.LibraryID {
			selected = l.Directory
			break
		}
	}
	if match.Directory != selected {
		diag["classificationDirectory"] = match.Directory
		diag["outputDirectory"] = selected
		diag["classificationMismatch"] = true
	}
	return nil
}
func hideFallback(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `UPDATE native_libraries f SET hidden=NOT EXISTS(SELECT 1 FROM native_asset_libraries a WHERE a.library_id=f.id),updated_at=now() WHERE f.id IN('f:movie','f:series')`)
	return err
}
func EnqueueAsset(ctx context.Context, db *pgxpool.Pool, assetID string, revision int64) error {
	if db == nil {
		return nativelibrary.ErrSettingsUnavailable
	}
	payload, err := json.Marshal(map[string]any{"assetId": assetID, "assetRevision": revision})
	if err != nil {
		return err
	}
	key := "native.assign:" + assetID
	_, err = db.Exec(ctx, `INSERT INTO jobs(id,kind,queue,payload,idempotency_key,max_attempts)VALUES($1,'native.assign','library',$2,$3,5) ON CONFLICT(idempotency_key) WHERE idempotency_key IS NOT NULL DO UPDATE SET state=CASE WHEN jobs.state IN('failed','retry') THEN 'pending' ELSE jobs.state END,attempts=CASE WHEN jobs.state IN('failed','retry') THEN 0 ELSE jobs.attempts END,available_at=CASE WHEN jobs.state IN('failed','retry') THEN now() ELSE jobs.available_at END,last_error=CASE WHEN jobs.state IN('failed','retry') THEN NULL ELSE jobs.last_error END`, jobs.UUID(), payload, key)
	return err
}

type Engine struct{ DB *pgxpool.Pool }

func (e Engine) Process(ctx context.Context, job jobs.Job) error {
	var in struct {
		AssetID       string `json:"assetId"`
		AssetRevision int64  `json:"assetRevision"`
	}
	if err := json.Unmarshal(job.Payload, &in); err != nil || in.AssetID == "" {
		return fmt.Errorf("invalid native assignment payload")
	}
	return (AssignmentRepository{DB: e.DB}).Assign(ctx, in.AssetID, in.AssetRevision)
}
func EnqueueBatch(ctx context.Context, db *pgxpool.Pool, after string, limit int) (string, int, error) {
	if db == nil {
		return after, 0, nativelibrary.ErrSettingsUnavailable
	}
	if limit < 1 || limit > 500 {
		return after, 0, fmt.Errorf("assignment batch limit must be 1..500")
	}
	rows, err := db.Query(ctx, `SELECT a.id::text,a.revision FROM media_assets a JOIN media_asset_bindings b ON b.asset_id=a.id AND b.state='ready' JOIN cloud_files f ON f.id=a.cloud_file_id AND f.state='present' WHERE ($1='' OR a.id::text>$1) ORDER BY a.id LIMIT $2`, after, limit)
	if err != nil {
		return after, 0, err
	}
	defer rows.Close()
	next := after
	count := 0
	for rows.Next() {
		var id string
		var rev int64
		if err := rows.Scan(&id, &rev); err != nil {
			return next, count, err
		}
		if err := EnqueueAsset(ctx, db, id, rev); err != nil {
			return next, count, err
		}
		next = id
		count++
	}
	return next, count, rows.Err()
}
func (r AssignmentRepository) ListAssets(ctx context.Context, libraryID string, start, limit int) (AssetPage, error) {
	out := AssetPage{Items: []AssetView{}, StartIndex: start, Limit: limit}
	if start < 0 || start > 1000000 || limit < 1 || limit > 200 {
		return out, fmt.Errorf("invalid asset page")
	}
	if r.DB == nil {
		return out, nativelibrary.ErrSettingsUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := r.DB.QueryRow(ctx, `WITH eligible AS (`+ExposableAssetsSQL+`) SELECT count(*) FROM native_asset_libraries a JOIN eligible e ON e.asset_id=a.asset_id WHERE a.library_id=$1`, libraryID).Scan(&out.Total); err != nil {
		return out, err
	}
	rows, err := r.DB.Query(ctx, `WITH eligible AS (`+ExposableAssetsSQL+`) SELECT a.asset_id::text,e.title,jsonb_agg(s.output_path ORDER BY s.output_path),a.source,a.diagnostic FROM native_asset_libraries a JOIN eligible e ON e.asset_id=a.asset_id JOIN strm_entries s ON s.asset_id=e.asset_id AND s.output_path<>'' AND lower(s.output_path) LIKE '%.strm' WHERE a.library_id=$1 GROUP BY a.asset_id,e.title,a.source,a.diagnostic ORDER BY a.asset_id LIMIT $2 OFFSET $3`, libraryID, limit, start)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var x AssetView
		var raw []byte
		if err := rows.Scan(&x.AssetID, &x.Title, &raw, &x.Source, &x.Diagnostic); err != nil {
			return out, err
		}
		if err := json.Unmarshal(raw, &x.OutputPaths); err != nil {
			return out, err
		}
		out.Items = append(out.Items, x)
	}
	return out, rows.Err()
}
