package nativetmdb

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"media-tool/internal/modules/classification"
	"media-tool/internal/modules/nativelibrary/catalog"
)

var ErrClassificationRefresh = errors.New("分类事实补齐未完成，既有元数据和整理结果保持不变")
var ErrClassificationConflict = errors.New("详情快照已变化，请重新核对后再补齐分类事实")

type ClassificationRefresh struct {
	Type    string `json:"type"`
	TMDBID  int64  `json:"tmdbId"`
	Changed bool   `json:"changed"`
}

// RefreshClassification is an explicitly invoked, idempotent repair for an
// already exposed identity with an existing successful snapshot. Only one JSON
// child is updated: no ingest rows, output paths, refresh jobs or scraped events.
// No transaction/row lock is held while the read-only TMDB request is in flight.
func (e Engine) RefreshClassification(parent context.Context, t Target) (ClassificationRefresh, error) {
	out := ClassificationRefresh{Type: t.Type, TMDBID: t.TMDBID}
	if e.DB == nil || e.Connections == nil || !t.Valid() || t.Type == "season" {
		return out, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(parent, 40*time.Second)
	defer cancel()
	client, err := e.Connections.TMDB(ctx)
	if err != nil {
		return out, ErrClassificationRefresh
	}
	language := client.Language
	if language == "" {
		language = "zh-CN"
	}
	client.Language = language
	var id string
	var raw []byte
	scope := `WITH live AS (` + catalog.ExposableAssetsSQL + `) `
	err = e.DB.QueryRow(ctx, scope+`SELECT d.id::text,d.payload FROM native_tmdb_details d WHERE d.tmdb_type=$1 AND d.tmdb_id=$2 AND d.season_number IS NULL AND d.episode_number IS NULL AND d.language=$3 AND d.fetched_at IS NOT NULL AND d.state<>'fetching' AND EXISTS(SELECT 1 FROM live WHERE tmdb_id=d.tmdb_id AND media_type=d.tmdb_type)`, t.Type, t.TMDBID, language).Scan(&id, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrInvalid
	}
	if err != nil || len(raw) > 4<<20 {
		return out, ErrClassificationRefresh
	}
	var prior struct {
		Facts *classification.Facts `json:"classification"`
	}
	if json.Unmarshal(raw, &prior) == nil && prior.Facts != nil && prior.Facts.Valid() {
		return out, nil
	}
	snapshot, err := client.ClassificationDetails(ctx, t.Type, t.TMDBID)
	if err != nil || !snapshot.Valid() {
		return out, ErrClassificationRefresh
	}
	facts, err := json.Marshal(snapshot)
	if err != nil {
		return out, ErrClassificationRefresh
	}
	// The payload equality fence also prevents overwriting a concurrent full
	// refresh/edit. A disappeared/rebound asset is rechecked at the write boundary.
	tag, err := e.DB.Exec(ctx, scope+`UPDATE native_tmdb_details d SET payload=jsonb_set(d.payload,'{classification}',$4::jsonb,true),updated_at=clock_timestamp()
 WHERE d.id=$1::uuid AND d.payload=$2::jsonb AND d.language=$3 AND d.tmdb_type=$5 AND d.tmdb_id=$6 AND d.season_number IS NULL AND d.episode_number IS NULL AND d.state<>'fetching' AND d.fetched_at IS NOT NULL AND EXISTS(SELECT 1 FROM live WHERE tmdb_id=d.tmdb_id AND media_type=d.tmdb_type)`, id, raw, language, facts, t.Type, t.TMDBID)
	if err != nil {
		return out, ErrClassificationRefresh
	}
	if tag.RowsAffected() != 1 {
		return out, ErrClassificationConflict
	}
	out.Changed = true
	return out, nil
}
