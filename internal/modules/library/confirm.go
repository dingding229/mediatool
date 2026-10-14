package library

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/metadata"
	"media-tool/internal/modules/playbackgrants"
	"media-tool/internal/modules/releasename"
	"media-tool/internal/modules/storage"
)

type Candidate struct {
	ID         string    `json:"id"`
	FileID     string    `json:"fileId"`
	Name       string    `json:"name"`
	AssetID    string    `json:"assetId,omitempty"`
	Path       string    `json:"path"`
	Size       int64     `json:"size"`
	State      string    `json:"state"`
	Revision   int64     `json:"revision"`
	CloudState string    `json:"cloudState"`
	UpdatedAt  time.Time `json:"updatedAt"`
}
type Confirmation struct {
	MediaType      string `json:"mediaType"`
	TMDBID         int    `json:"tmdbId"`
	Season         int    `json:"season"`
	Episodes       []int  `json:"episodes"`
	Revision       int64  `json:"revision"`
	MigrateOutputs bool   `json:"migrateOutputs"`
	FileDigest     string `json:"fileDigest,omitempty"`
	PlanDigest     string `json:"planDigest,omitempty"`
}

func (e Engine) Candidates(ctx context.Context) ([]Candidate, error) {
	return e.CandidatePage(ctx, 500, 0, "")
}
func (e Engine) CandidatePage(ctx context.Context, limit, offset int, state string) ([]Candidate, error) {
	if state == "pending" {
		state = "needs_confirmation"
	}
	rows, err := e.DB.Query(ctx, `SELECT c.id::text,f.id::text,f.full_path,f.size_bytes,c.state,c.revision,f.state,c.updated_at,coalesce(c.asset_id::text,'') FROM library_candidates c JOIN cloud_files f ON f.id=c.file_id WHERE ($3='' OR c.state=$3 OR ($3='needs_confirmation' AND c.state IN ('pending','needs_output_reconciliation'))) ORDER BY c.updated_at DESC,c.id DESC LIMIT $1 OFFSET $2`, limit, offset, state)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Candidate{}
	for rows.Next() {
		var item Candidate
		if err = rows.Scan(&item.ID, &item.FileID, &item.Path, &item.Size, &item.State, &item.Revision, &item.CloudState, &item.UpdatedAt, &item.AssetID); err != nil {
			return nil, err
		}
		item.Name = path.Base(item.Path)
		out = append(out, item)
	}
	return out, rows.Err()
}
func (e Engine) Confirm(ctx context.Context, id string, in Confirmation) (string, error) {
	if in.MediaType != "movie" && in.MediaType != "series" || in.TMDBID < 1 {
		return "", errors.New("select a verified media identity")
	}
	if in.MediaType == "series" && (in.Season < 0 || in.Season > 999 || len(in.Episodes) == 0 || len(in.Episodes) > 100) {
		return "", errors.New("series confirmation requires a season and episode coverage")
	}
	for _, ep := range in.Episodes {
		if ep < 1 || ep > 9999 {
			return "", errors.New("invalid episode")
		}
	}
	if in.MediaType == "series" {
		var err error
		in.Episodes, err = metadata.NormalizeEpisodes(in.Season, in.Episodes)
		if err != nil {
			return "", err
		}
	}
	snapshot, sourceConnection, indexed, err := e.candidateFile(ctx, id)
	if err != nil {
		return "", errors.New("candidate unavailable")
	}
	if snapshot.State != "confirmed" {
		actual, err := e.observe(ctx, sourceConnection, indexed.Path)
		if err != nil {
			return "", err
		}
		if !fileMatches(indexed, actual) {
			return "", errors.New("cloud file identity changed; rescan before confirmation")
		}
		if in.FileDigest != "" && in.FileDigest != evidenceDigest(indexed) {
			return "", errors.New("recognition preview changed; refresh evidence")
		}
	}
	tmdb, err := e.Connections.TMDB(ctx)
	if err != nil {
		return "", err
	}
	item, err := tmdb.Details(ctx, in.MediaType, in.TMDBID)
	if err != nil {
		return "", err
	}
	if item.ID != in.TMDBID || item.MediaType != in.MediaType {
		return "", errors.New("metadata identity mismatch")
	}
	item, err = tmdb.WithEpisodes(ctx, item, in.Season, in.Episodes)
	if err != nil {
		return "", err
	}
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var fileID, connectionID, cloudPath, cloudState, candidateState string
	var revision int64
	err = tx.QueryRow(ctx, `SELECT c.file_id::text,f.storage_id::text,f.full_path,f.state,c.state,c.revision FROM library_candidates c JOIN cloud_files f ON f.id=c.file_id WHERE c.id=$1 FOR UPDATE OF c,f`, id).Scan(&fileID, &connectionID, &cloudPath, &cloudState, &candidateState, &revision)
	if err != nil {
		return "", errors.New("candidate unavailable")
	}
	if cloudPath != indexed.Path || connectionID != sourceConnection || revision != snapshot.Revision {
		return "", errors.New("candidate snapshot changed during metadata lookup")
	}
	var providerID string
	var fileSize int64
	var hashes []byte
	if err = tx.QueryRow(ctx, `SELECT coalesce(provider_file_id,''),size_bytes,metadata FROM cloud_files WHERE id=$1`, fileID).Scan(&providerID, &fileSize, &hashes); err != nil {
		return "", err
	}
	var currentHashes map[uint32]string
	json.Unmarshal(hashes, &currentHashes)
	if evidenceDigest(storage.File{ID: providerID, Name: path.Base(cloudPath), Path: cloudPath, Size: fileSize, Hashes: currentHashes}) != evidenceDigest(indexed) {
		return "", errors.New("indexed file changed during confirmation")
	}
	if revision != in.Revision {
		return "", errors.New("candidate changed; reload before confirmation")
	}
	if cloudState != "present" {
		return "", errors.New("cloud file is missing or not confirmed present")
	}
	if candidateState == "confirmed" {
		var asset string
		if err = tx.QueryRow(ctx, `SELECT asset_id::text FROM library_candidates WHERE id=$1`, id).Scan(&asset); err != nil {
			return "", err
		}
		return asset, nil
	}
	if candidateState == "artifacts_pending" || candidateState == "needs_output_reconciliation" {
		return "", ErrOutputLayoutRequired
	}
	connection, err := e.Connections.Repository.Get(ctx, connectionID)
	if err != nil {
		return "", err
	}
	allowed := connection.String("cloudRoot")
	if allowed == "" && len(e.Connections.Config.STRMAllowedRoots) > 0 {
		allowed = e.Connections.Config.STRMAllowedRoots[0]
	}
	if allowed == "" || !connection.Record.Enabled {
		return "", errors.New("cloud connection disabled or root missing")
	}
	if !inside(path.Clean(allowed), cloudPath) {
		return "", errors.New("file no longer belongs to the configured root")
	}
	if candidateState != "confirmed" {
		actual, err := e.observe(ctx, connectionID, cloudPath)
		if err != nil {
			return "", err
		}
		if !fileMatches(indexed, actual) {
			return "", errors.New("cloud file changed before import; no output or playback grant was written")
		}
	}
	mediaJSON, _ := json.Marshal(item.ShowMetadata())
	assetJSON, _ := json.Marshal(item)
	var mediaID string
	if err = tx.QueryRow(ctx, `INSERT INTO media_items(id,media_type,tmdb_id,title,release_year,metadata)VALUES($1,$2,$3,$4,$5,$6)ON CONFLICT(media_type,tmdb_id)DO UPDATE SET metadata=excluded.metadata RETURNING id::text`, jobs.UUID(), item.MediaType, item.ID, item.Title, item.Year, mediaJSON).Scan(&mediaID); err != nil {
		return "", err
	}
	assetID := fileID
	if _, err = tx.Exec(ctx, `INSERT INTO media_assets(id,cloud_file_id,media_identity,edition)VALUES($1,$2,$3,'cloud-library')ON CONFLICT(id)DO NOTHING`, assetID, fileID, assetJSON); err != nil {
		return "", err
	}
	var season, episode any
	episodes := []int{}
	if item.MediaType == "series" {
		season = in.Season
		episode = in.Episodes[0]
		episodes = in.Episodes
	}
	episodeJSON, _ := json.Marshal(episodes)
	if _, err = tx.Exec(ctx, `INSERT INTO media_asset_bindings(asset_id,media_id,season,episode,episodes)VALUES($1,$2,$3,$4,$5)ON CONFLICT(asset_id)DO UPDATE SET media_id=excluded.media_id,season=excluded.season,episode=excluded.episode,episodes=excluded.episodes`, assetID, mediaID, season, episode, episodeJSON); err != nil {
		return "", err
	}
	grantService := playbackgrants.Service{DB: e.DB, Vault: e.Connections.Repository.Vault, PublicURL: e.Connections.Config.PublicURL}
	link, grantID, err := grantService.Prepare(ctx, tx, assetID, cloudPath)
	if err != nil {
		return "", err
	}
	rootDir := e.Connections.Config.STRMOutputDir
	if err = os.MkdirAll(rootDir, 0750); err != nil {
		return "", err
	}
	journal := Journal{DB: e.DB, Vault: e.Connections.Repository.Vault}
	var seasonValue *int
	if item.MediaType == "series" {
		seasonValue = &in.Season
	}
	projections, err := AssetOutputProjections(cloudPath, assetID, item, seasonValue, in.Episodes)
	if err != nil {
		return "", err
	}
	projections, err = ApplyOutputTemplates(ctx, e.DB, cloudPath, assetID, item, projections)
	if err != nil {
		return "", err
	}
	intents, err := StageOutputProjections(ctx, journal, tx, rootDir, assetID, item, seasonValue, projections, []byte(link+"\n"), playbackgrants.SecretFreeKeyRef(grantID))
	if err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `UPDATE library_candidates SET state='artifacts_pending',asset_id=$2,revision=revision+1,updated_at=now()WHERE id=$1`, id, assetID); err != nil {
		return "", err
	}
	event, _ := json.Marshal(map[string]string{"title": item.Title, "assetId": assetID})
	if _, err = tx.Exec(ctx, `INSERT INTO outbox_events(id,event_type,aggregate_id,payload)VALUES($1,'media.artifacts_staged',$2,$3)`, jobs.UUID(), assetID, event); err != nil {
		return "", err
	}
	audit, _ := json.Marshal(map[string]string{"candidateId": id, "assetId": assetID})
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','library.confirm',$1)`, audit); err != nil {
		return "", err
	}
	if err = tx.Commit(ctx); err != nil {
		return "", err
	}
	for _, intent := range intents {
		if err = journal.Apply(ctx, intent); err != nil {
			return assetID, err
		}
	}
	if _, err = e.DB.Exec(ctx, `UPDATE library_candidates SET state='confirmed',updated_at=now()WHERE id=$1`, id); err != nil {
		return assetID, err
	}
	if err = e.QueueRefresh(ctx, assetID); err != nil {
		return assetID, err
	}
	return assetID, nil
}
func safeName(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`/\:*?"<>|`, r) {
			return '_'
		}
		return r
	}, s)
	s = strings.Trim(s, " .")
	if s == "" {
		return "media"
	}
	return s
}
func (e Engine) Suggest(ctx context.Context, id string) ([]metadata.Item, error) {
	var filename string
	if e.DB.QueryRow(ctx, `SELECT f.full_path FROM library_candidates c JOIN cloud_files f ON f.id=c.file_id WHERE c.id=$1`, id).Scan(&filename) != nil {
		return nil, errors.New("candidate unavailable")
	}
	query := path.Base(path.Dir(filename))
	query = regexp.MustCompile(`\([^)]*\)|\[[^]]*\]`).ReplaceAllString(query, "")
	hint := releasename.Parse(path.Base(filename))
	if hint.Title != "" && hint.MediaType != "unknown" {
		query = hint.Title
	}
	client, err := e.Connections.TMDB(ctx)
	if err != nil {
		return nil, err
	}
	movies, err := client.Search(ctx, "movie", strings.TrimSpace(query))
	if err != nil {
		return nil, err
	}
	series, err := client.Search(ctx, "series", strings.TrimSpace(query))
	if err != nil {
		return movies, nil
	}
	return append(movies, series...), nil
}
