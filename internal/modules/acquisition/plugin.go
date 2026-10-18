package acquisition

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/jackc/pgx/v5"
	"media-tool/internal/adapters/qbittorrent"
	"media-tool/internal/modules/downloads"
	"media-tool/internal/modules/jobs"
	deferutil "media-tool/internal/platform/cleanup"
	dbutil "media-tool/internal/platform/database"
	"media-tool/internal/platform/outbox"
	"slices"
	"strings"
	"time"
)

// DownloaderService exposes observation of foreign torrents, but mutation only of
// the original program-owned task/endpoint/hash/category/path/tag tuple.
type DownloaderService struct {
	Engine             Engine
	AllowedDownloaders []string
}

func (s DownloaderService) Inspect(ctx context.Context, hash string) (downloads.Task, error) {
	a, _, err := s.Engine.Connections.QB(ctx)
	if err != nil {
		return downloads.Task{}, err
	}
	return a.Inspect(ctx, hash)
}
func (s DownloaderService) AllHashes(ctx context.Context) ([]string, error) {
	a, _, err := s.Engine.Connections.QB(ctx)
	if err != nil {
		return nil, err
	}
	return a.AllHashes(ctx)
}
func (s DownloaderService) owned(ctx context.Context, idOrHash string) (string, *qbittorrent.Adapter, downloads.Task, error) {
	var id, cid, hash string
	var d taskDetails
	var raw []byte
	err := s.Engine.DB.QueryRow(ctx, `SELECT id::text,integration_id::text,info_hash,details FROM download_tasks WHERE (id::text=$1 OR info_hash=$1) AND info_hash IS NOT NULL AND download_state NOT IN('failed','cancelled') ORDER BY created_at DESC LIMIT 1`, idOrHash).Scan(&id, &cid, &hash, &raw)
	if err != nil {
		return "", nil, downloads.Task{}, err
	}
	if len(s.AllowedDownloaders) > 0 && !slices.Contains(s.AllowedDownloaders, cid) {
		return "", nil, downloads.Task{}, errors.New("downloader not authorized")
	}
	if err = decodeDetails(raw, &d); err != nil {
		return "", nil, downloads.Task{}, err
	}
	a, err := s.Engine.client(ctx, cid, d)
	if err != nil {
		return "", nil, downloads.Task{}, err
	}
	task, err := a.Inspect(ctx, hash)
	if err != nil {
		return "", nil, task, err
	}
	if task.ID != hash || task.SavePath != d.DownloadPath || task.Category != d.Category {
		return "", nil, task, errors.New("ownership mismatch")
	}
	tags, save, err := a.CleanupInfo(ctx, hash)
	if err != nil || save != d.DownloadPath || !ownsTag(tags, id) {
		return "", nil, task, errors.New("ownership tag mismatch")
	}
	return id, a, task, nil
}
func (s DownloaderService) Owned(ctx context.Context, idOrHash string) (bool, error) {
	_, _, _, err := s.owned(ctx, idOrHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}
func (s DownloaderService) Files(ctx context.Context, id string) ([]downloads.File, error) {
	_, a, t, err := s.owned(ctx, id)
	if err != nil {
		return nil, err
	}
	return a.Files(ctx, t.ID)
}
func (s DownloaderService) Trackers(ctx context.Context, id string) ([]downloads.Tracker, error) {
	_, a, t, err := s.owned(ctx, id)
	if err != nil {
		return nil, err
	}
	return a.Trackers(ctx, t.ID)
}
func (s DownloaderService) SetPaused(ctx context.Context, id string, paused bool, admit func() error) error {
	_, a, t, err := s.owned(ctx, id)
	if err != nil {
		return err
	}
	if err = admit(); err != nil {
		return err
	}
	return a.SetPaused(ctx, t.ID, paused)
}

// DeleteOwned distinguishes torrent-only retirement from explicit payload cleanup.
// Both require original ownership and known-false HR; file deletion still runs
// through the native worker's full cloud/path/sharing/lease proof pipeline.
func (s DownloaderService) DeleteOwned(ctx context.Context, idOrHash string, deleteFiles bool, admit func() error) error {
	id, a, task, err := s.owned(ctx, idOrHash)
	if err != nil {
		return err
	}
	state, _, eligible, err := s.PreviewCleanup(ctx, id)
	if err != nil {
		return err
	}
	if state != "imported" || !eligible {
		return ErrCleanupPrerequisites
	}
	if deleteFiles {
		return s.Engine.ApproveCleanup(ctx, id, true, false)
	}
	// Torrent-only retirement never unlinks any payload, let alone media/STRM.
	if err = admit(); err != nil {
		return err
	}
	if err = a.DeleteOwned(ctx, task.ID, false); err != nil {
		return err
	}
	if _, err = a.Inspect(ctx, task.ID); !errors.Is(err, qbittorrent.ErrNotFound) {
		return errors.Join(jobs.ErrOperatorRequired, errors.New("删除尚未读回确认，不重放"))
	}
	tx, err := s.Engine.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer deferutil.Rollback(ctx, tx)
	var deletedAt time.Time
	if err = tx.QueryRow(ctx, `UPDATE download_tasks SET seeding_state='removed',updated_at=now()WHERE id=$1 AND info_hash=$2 RETURNING updated_at`, id, task.ID).Scan(&deletedAt); err != nil {
		return err
	}
	if err = outbox.EmitVersionTx(ctx, tx, "download.task.deleted", id, "torrent-only:"+deletedAt.UTC().Format(time.RFC3339Nano), contracts.DownloadDeleted{TaskID: id}); err != nil {
		return err
	}
	return dbutil.Commit(ctx, tx)
}

func decodeDetails(raw []byte, out *taskDetails) error { return json.Unmarshal(raw, out) }
func ownsTag(tags, id string) bool {
	for _, t := range strings.Split(tags, ",") {
		if strings.TrimSpace(t) == id {
			return true
		}
	}
	return false
}

// PreviewCleanup is an admission preview, not authorization to unlink anything.
// The cleanup worker still rechecks cloud copies, filesystem ownership/sharing,
// original HR evidence, endpoint/tag/hash and lease immediately before deletion.
func (s DownloaderService) PreviewCleanup(ctx context.Context, id string) (string, string, bool, error) {
	var state, hash, resource string
	var raw []byte
	if err := s.Engine.DB.QueryRow(ctx, `SELECT download_state,coalesce(info_hash,''),resource_id::text,details FROM download_tasks WHERE id=$1`, id).Scan(&state, &hash, &resource, &raw); err != nil {
		return "", "", false, err
	}
	if state != "imported" || hash == "" {
		return state, hash, false, nil
	}
	if _, _, _, err := s.owned(ctx, id); err != nil {
		return "", hash, false, err
	}
	var details taskDetails
	if err := decodeDetails(raw, &details); err != nil {
		return "", hash, false, err
	}
	var snapshot []byte
	if details.SearchRunID == "" {
		return state, hash, false, nil
	}
	if err := s.Engine.DB.QueryRow(ctx, `SELECT public_snapshot FROM search_run_resources WHERE run_id=$1 AND resource_id=$2`, details.SearchRunID, resource).Scan(&snapshot); err != nil {
		return state, hash, false, nil
	}
	var evidence struct {
		Metadata map[string]any `json:"metadata"`
	}
	if err := json.Unmarshal(snapshot, &evidence); err != nil {
		return state, hash, false, err
	}
	hr, known := evidence.Metadata["hr"].(bool)
	return state, hash, known && !hr, nil
}

// TransferProgressPage exposes opaque transfer IDs required by the native Control
// facade, without sessions, destinations, providers or raw progress errors.
type TransferSummary struct {
	ID, State string
	Progress  int
}

func (s DownloaderService) Transfers(ctx context.Context, taskID string) ([]TransferSummary, error) {
	rows, err := s.Engine.DB.Query(ctx, `SELECT t.id::text,t.state,t.phase,t.bytes_transferred,t.hash_bytes,f.size_bytes FROM transfer_tasks t JOIN download_files f ON f.id=t.download_file_id WHERE f.task_id=$1 ORDER BY t.created_at,t.id LIMIT 200`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TransferSummary{}
	for rows.Next() {
		var v TransferSummary
		var phase string
		var bytes, hashed, size int64
		if err = rows.Scan(&v.ID, &v.State, &phase, &bytes, &hashed, &size); err != nil {
			return nil, err
		}
		v.Progress, _ = TransferProgress(v.State, phase, bytes, hashed, size)
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s DownloaderService) UploadControl(ctx context.Context, id, action string, admit func() error) error {
	if action != "pause" && action != "resume" && action != "retry" {
		return errors.New("unsupported transfer action")
	}
	var downloader string
	if err := s.Engine.DB.QueryRow(ctx, `SELECT d.integration_id::text FROM transfer_tasks t JOIN download_files f ON f.id=t.download_file_id JOIN download_tasks d ON d.id=f.task_id WHERE t.id=$1`, id).Scan(&downloader); err != nil {
		return err
	}
	if len(s.AllowedDownloaders) > 0 && !slices.Contains(s.AllowedDownloaders, downloader) {
		return errors.New("downloader not authorized")
	}
	if err := admit(); err != nil {
		return err
	}
	return s.Engine.Control(ctx, id, UploadControl{Action: action})
}

var ErrCleanupPrerequisites = errors.New("native cleanup prerequisites required")
