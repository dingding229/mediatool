package library

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"time"
)

type HistoryFilter struct {
	Kind, State, Query, ConnectionID, BatchID string
	From, To                                  *time.Time
	Limit, Offset                             int
}
type HistoryEntry struct {
	ID           string          `json:"id"`
	Kind         string          `json:"kind"`
	State        string          `json:"state"`
	Source       string          `json:"source"`
	ConnectionID string          `json:"connectionId"`
	BatchID      string          `json:"batchId"`
	Message      string          `json:"message"`
	CreatedAt    time.Time       `json:"createdAt"`
	UpdatedAt    time.Time       `json:"updatedAt"`
	Details      json.RawMessage `json:"details"`
}

const historyUnion = `WITH history AS (
 SELECT o.id,'file' AS kind,o.state,o.plan->'items'->0->'source'->>'path' AS source,o.integration_id,coalesce(i.batch_id::text,'') AS batch_id,o.message,o.created_at,o.updated_at,jsonb_build_object('plan',o.plan,'completedSteps',o.completed_steps) AS details FROM file_operations o LEFT JOIN file_batch_items i ON i.operation_id=o.id WHERE o.state<>'preview'
 UNION ALL SELECT b.id,'batch',b.state,'',NULL::uuid,b.id::text,'',b.created_at,b.updated_at,jsonb_build_object('items',(SELECT jsonb_agg(jsonb_build_object('ordinal',i.ordinal,'source',i.source,'state',i.state,'message',i.message,'operationId',i.operation_id) ORDER BY i.ordinal) FROM file_batch_items i WHERE i.batch_id=b.id)) FROM file_batches b
 UNION ALL SELECT s.id,'scan',s.state,s.root,s.integration_id,'',s.message,s.created_at,s.updated_at,jsonb_build_object('files',s.files_seen,'directories',s.directories_seen,'completedDirectories',(SELECT count(*) FROM library_scan_directories d WHERE d.scan_id=s.id AND state='completed'),'currentDirectory',s.current_directory,'generation',s.generation,'revision',s.integration_revision) FROM library_scans s
 UNION ALL SELECT t.id,'transfer',t.state,t.destination,t.integration_id,'',t.last_error,t.created_at,t.updated_at,jsonb_build_object('phase',t.phase,'bytesTransferred',t.bytes_transferred,'size',f.size_bytes,'hashBytes',t.hash_bytes,'sourceSha1',t.source_sha1,'taskId',f.task_id,'source',f.relative_path,'controlRevision',t.control_revision,'uploadId',t.upload_id,'originalPreserved',true) FROM transfer_tasks t JOIN download_files f ON f.id=t.download_file_id
 UNION ALL SELECT j.id,'output',j.state,j.relative_path,NULL::uuid,'','',j.created_at,j.updated_at,jsonb_build_object('assetId',j.asset_id,'operation',j.operation,'kind',j.artifact_kind,'owner',j.owner_scope) FROM media_artifact_journal j
 UNION ALL SELECT b.id,'strmRepair',CASE WHEN b.state='running' AND j.state='failed' THEN 'needs_reconcile' ELSE b.state END,'',NULL::uuid,b.id::text,'STRM缺失修复；不处理NFO或源媒体',b.created_at,b.updated_at,jsonb_build_object('items',(SELECT coalesce(jsonb_agg(jsonb_build_object('ordinal',i.ordinal,'source',i.relative_path,'state',i.state,'message',i.message)ORDER BY i.ordinal),'[]')FROM strm_repair_items i WHERE i.batch_id=b.id))FROM strm_repair_batches b LEFT JOIN jobs j ON j.id=b.job_id
 UNION ALL SELECT c.asset_id,'emby',c.state,'',c.integration_id,'',c.message,c.checked_at,c.checked_at,jsonb_build_object('assetId',c.asset_id,'revision',c.asset_revision,'paths',c.paths,'decodingVerified',false) FROM library_emby_checks c
), selected AS (SELECT * FROM history WHERE NOT EXISTS(SELECT 1 FROM library_hidden_history hidden WHERE hidden.kind=history.kind AND hidden.record_id=history.id AND history.updated_at<=hidden.observed_at) AND ($1='' OR kind=$1) AND ($2='' OR state=$2) AND ($3='' OR position(lower($3) IN lower(coalesce(source,'')||' '||message||' '||details::text))>0) AND ($4='' OR integration_id::text=$4) AND ($5='' OR batch_id=$5) AND ($6::timestamptz IS NULL OR created_at>=$6) AND ($7::timestamptz IS NULL OR created_at<$7)) `

func (e Engine) History(ctx context.Context, f HistoryFilter) ([]HistoryEntry, int, error) {
	if f.Limit < 1 || f.Limit > 200 || f.Offset < 0 || len(f.Query) > 256 {
		return nil, 0, errors.New("invalid history filter")
	}
	args := []any{f.Kind, f.State, f.Query, f.ConnectionID, f.BatchID, f.From, f.To}
	var total int
	if err := e.DB.QueryRow(ctx, historyUnion+`SELECT count(*) FROM selected`, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := e.DB.Query(ctx, historyUnion+`SELECT id::text,kind,state,coalesce(source,''),coalesce(integration_id::text,''),batch_id,message,created_at,updated_at,details FROM selected ORDER BY updated_at DESC,id DESC LIMIT $8 OFFSET $9`, append(args, f.Limit, f.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []HistoryEntry{}
	for rows.Next() {
		var item HistoryEntry
		if err = rows.Scan(&item.ID, &item.Kind, &item.State, &item.Source, &item.ConnectionID, &item.BatchID, &item.Message, &item.CreatedAt, &item.UpdatedAt, &item.Details); err != nil {
			return nil, 0, err
		}

		out = append(out, item)
	}
	return out, total, rows.Err()
}

type ManagedAsset struct {
	Poster         string          `json:"poster"`
	ID             string          `json:"id"`
	Title          string          `json:"title"`
	MediaType      string          `json:"mediaType"`
	Year           string          `json:"year"`
	Source         string          `json:"source"`
	CloudState     string          `json:"cloudState"`
	ArtifactsState string          `json:"artifactsState"`
	Revision       int64           `json:"revision"`
	Outputs        json.RawMessage `json:"outputs"`
	EmbyState      string          `json:"embyState"`
	EmbyPaths      json.RawMessage `json:"embyPaths"`
	ScrapeReport   json.RawMessage `json:"scrapeReport"`
	Revoked        bool            `json:"revoked"`
}

func (e Engine) ManagedAssets(ctx context.Context, limit, offset int, q string) ([]ManagedAsset, int, error) {
	return e.ManagedAssetsKind(ctx, limit, offset, q, false)
}
func (e Engine) ManagedAssetsKind(ctx context.Context, limit, offset int, q string, strmOnly bool) ([]ManagedAsset, int, error) {
	if limit < 1 || limit > 200 || offset < 0 || len(q) > 256 {
		return nil, 0, errors.New("invalid output filter")
	}
	where := ` FROM media_assets a JOIN cloud_files f ON f.id=a.cloud_file_id LEFT JOIN media_asset_bindings b ON b.asset_id=a.id LEFT JOIN media_items m ON m.id=b.media_id LEFT JOIN library_emby_checks c ON c.asset_id=a.id LEFT JOIN playback_grants g ON g.asset_id=a.id LEFT JOIN integrations emby ON emby.id=c.integration_id LEFT JOIN application_settings prefs ON prefs.singleton WHERE ($1='' OR position(lower($1) IN lower(coalesce(m.title,'')||' '||f.full_path))>0)`
	if strmOnly {
		where += ` AND EXISTS(SELECT 1 FROM strm_entries s JOIN managed_media_artifacts owned ON owned.relative_path=s.output_path AND owned.asset_id=s.asset_id WHERE s.asset_id=a.id AND owned.artifact_kind='strm')`
	}
	var total int
	if err := e.DB.QueryRow(ctx, `SELECT count(*)`+where, q).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := e.DB.Query(ctx, `SELECT a.id::text,coalesce(m.title,''),coalesce(m.media_type,''),coalesce(m.release_year,''),f.full_path,f.state,a.artifacts_state,a.revision,coalesce((SELECT jsonb_agg(jsonb_build_object('path',path,'kind',kind,'owner',owner) ORDER BY path) FROM (SELECT s.output_path AS path,'strm' AS kind,'asset:'||a.id::text AS owner FROM strm_entries s WHERE s.asset_id=a.id UNION ALL SELECT relative_path,artifact_kind,owner_scope FROM managed_media_artifacts WHERE asset_id=a.id AND artifact_kind<>'strm') outputs),'[]'),CASE WHEN c.asset_revision=a.revision AND emby.enabled AND c.integration_revision=emby.revision AND c.user_id=coalesce(prefs.value->>'embyUserId','') THEN c.state ELSE 'pending' END,CASE WHEN c.asset_revision=a.revision AND emby.enabled AND c.integration_revision=emby.revision AND c.user_id=coalesce(prefs.value->>'embyUserId','') THEN c.paths ELSE '[]'::jsonb END,coalesce(g.revoked_at IS NOT NULL,false),a.scrape_report,coalesce(m.metadata->>'posterPath','')`+where+` ORDER BY a.created_at DESC,a.id DESC LIMIT $2 OFFSET $3`, q, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []ManagedAsset{}
	for rows.Next() {
		var item ManagedAsset
		var poster string
		if err = rows.Scan(&item.ID, &item.Title, &item.MediaType, &item.Year, &item.Source, &item.CloudState, &item.ArtifactsState, &item.Revision, &item.Outputs, &item.EmbyState, &item.EmbyPaths, &item.Revoked, &item.ScrapeReport, &poster); err != nil {
			return nil, 0, err
		}
		if regexp.MustCompile(`^/[A-Za-z0-9_-]+\.(jpg|png|webp)$`).MatchString(poster) {
			item.Poster = "/api/v1/media/tmdb/image?path=" + url.QueryEscape(poster)
		}
		out = append(out, item)
	}
	return out, total, rows.Err()
}
