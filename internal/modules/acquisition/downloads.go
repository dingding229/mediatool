package acquisition

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/adapters/qbittorrent"
	"media-tool/internal/modules/downloads"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/metadata"
	"media-tool/internal/modules/search"
)

type Engine struct {
	DB          *pgxpool.Pool
	Connections *integrations.Manager
	Search      search.Engine
}
type taskDetails struct {
	SearchRunID          string        `json:"searchRunId,omitempty"`
	SourceSnapshotDigest string        `json:"sourceSnapshotDigest,omitempty"`
	SubscriptionRunID    string        `json:"subscriptionRunId,omitempty"`
	Media                metadata.Item `json:"media"`
	Endpoint             string        `json:"endpoint"`
	DownloadPath         string        `json:"downloadPath"`
	LocalPath            string        `json:"localPath"`
	Category             string        `json:"category"`
	HashAliases          []string      `json:"hashAliases,omitempty"`
	ExpectedHash         string        `json:"expectedHash,omitempty"`
	ExpectedSeason       *int          `json:"expectedSeason,omitempty"`
	ExpectedEpisodes     []int         `json:"expectedEpisodes,omitempty"`
	Progress             float64       `json:"progress"`
	QBState              string        `json:"qbState"`
	Message              string        `json:"message,omitempty"`
}
type Task struct {
	ID                     string        `json:"id"`
	ResourceID             string        `json:"resourceId"`
	Title                  string        `json:"title"`
	State                  string        `json:"state"`
	SeedingState           string        `json:"seedingState"`
	HashAliases            []string      `json:"hashAliases,omitempty"`
	ExpectedHash           string        `json:"expectedHash,omitempty"`
	ExpectedSeason         *int          `json:"expectedSeason,omitempty"`
	ExpectedEpisodes       []int         `json:"expectedEpisodes,omitempty"`
	Progress               float64       `json:"progress"`
	InfoHash               string        `json:"infoHash"`
	Message                string        `json:"message"`
	SourceRecoveryRequired bool          `json:"sourceRecoveryRequired"`
	SourceControlRevision  int64         `json:"sourceControlRevision"`
	Media                  metadata.Item `json:"media"`
	CreatedAt              time.Time     `json:"createdAt"`
}

func (e Engine) Start(ctx context.Context, resourceID, mediaType string, tmdbID int) (string, error) {
	return e.StartExpected(ctx, resourceID, mediaType, tmdbID, "", nil, nil)
}
func (e Engine) StartExpected(ctx context.Context, resourceID, mediaType string, tmdbID int, expectedHash string, season *int, episodes []int) (string, error) {
	// Retained for already-existing internal/legacy task callers. New HTTP and
	// subscription creation must use the run-bound entry points below.
	return e.startExpected(ctx, "", resourceID, mediaType, tmdbID, expectedHash, season, episodes, "", 0)
}

func (e Engine) StartFromRun(ctx context.Context, runID, resourceID, mediaType string, tmdbID int) (string, error) {
	if runID == "" {
		var err error
		runID, err = e.Search.LatestRunForResource(ctx, resourceID)
		if err != nil {
			return "", err
		}
	}
	raw, err := e.Search.TorrentFromRun(ctx, runID, resourceID)
	if err != nil {
		return "", err
	}
	meta, err := downloads.ParseTorrent(raw)
	if err != nil {
		return "", err
	}
	return e.startExpected(ctx, runID, resourceID, mediaType, tmdbID, meta.InfoHash, nil, nil, "", 0)
}

func (e Engine) StartExpectedFromRun(ctx context.Context, runID, resourceID, mediaType string, tmdbID int, expectedHash string, season *int, episodes []int, subscriptionRunID string, revision int64) (string, error) {
	return e.startExpected(ctx, runID, resourceID, mediaType, tmdbID, expectedHash, season, episodes, subscriptionRunID, revision)
}

func (e Engine) startExpected(ctx context.Context, runID, resourceID, mediaType string, tmdbID int, expectedHash string, season *int, episodes []int, subscriptionRunID string, revision int64) (string, error) {
	if runID != "" {
		if err := e.Search.ValidateRunResource(ctx, runID, resourceID); err != nil {
			return "", err
		}
	}
	_, connection, err := e.Connections.QB(ctx)
	if err != nil {
		return "", errors.New("configure and enable qBittorrent")
	}
	source, local := connection.String("downloadPath"), connection.String("localPath")
	if source == "" || local == "" {
		return "", errors.New("configure downloader and shared local paths")
	}
	info, err := os.Stat(local)
	if err != nil || !info.IsDir() {
		return "", errors.New("shared download directory is not accessible to this program")
	}
	tmdb, err := e.Connections.TMDB(ctx)
	if err != nil {
		return "", errors.New("configure TMDB and select the media identity")
	}
	media, err := tmdb.Details(ctx, mediaType, tmdbID)
	if err != nil || media.ID != tmdbID || media.MediaType != mediaType {
		return "", errors.New("media identity verification failed")
	}
	details := taskDetails{SearchRunID: runID, SubscriptionRunID: subscriptionRunID, ExpectedHash: expectedHash, ExpectedSeason: season, ExpectedEpisodes: episodes, Media: media, Endpoint: connection.String("endpoint"), DownloadPath: source, LocalPath: local, Category: connection.String("category")}
	if details.Category == "" {
		details.Category = "media-tool"
	}
	raw, _ := json.Marshal(details)
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	if subscriptionRunID != "" {
		var currentRevision, runRevision int64
		var enabled bool
		var phase string
		if err = tx.QueryRow(ctx, `SELECT s.revision,s.enabled,r.revision,r.collection_state FROM subscription_runs r JOIN subscriptions s ON s.id=r.subscription_id WHERE r.id=$1 AND r.search_run_id=$2 FOR SHARE OF s`, subscriptionRunID, runID).Scan(&currentRevision, &enabled, &runRevision, &phase); err != nil {
			return "", search.ErrSnapshotChanged
		}
		if !enabled || revision != currentRevision || runRevision != revision || phase != "collected" {
			return "", search.ErrSnapshotChanged
		}
	}
	if runID != "" {
		if err = e.Search.ValidateRunResourceTx(ctx, tx, runID, resourceID); err != nil {
			return "", err
		}
		details.SourceSnapshotDigest, err = e.Search.SourceSnapshotDigest(ctx, runID, resourceID)
		if err != nil {
			return "", err
		}
		raw, err = json.Marshal(details)
		if err != nil {
			return "", err
		}
	}
	var lockedResource string
	if err = tx.QueryRow(ctx, `SELECT id::text FROM resources WHERE id=$1 FOR UPDATE`, resourceID).Scan(&lockedResource); err != nil {
		return "", errors.New("resource unavailable")
	}
	var existing string
	var existingRaw []byte
	var existingHash string
	var sourceRequired bool
	err = tx.QueryRow(ctx, `SELECT id::text,details,coalesce(info_hash,''),source_recovery_required FROM download_tasks WHERE resource_id=$1 AND download_state NOT IN ('failed','cancelled')`, resourceID).Scan(&existing, &existingRaw, &existingHash, &sourceRequired)
	if err == nil {
		var previous taskDetails
		if json.Unmarshal(existingRaw, &previous) != nil {
			return "", ErrDownloadIdentityConflict
		}
		if previous.Media.ID != media.ID || previous.Media.MediaType != media.MediaType {
			return "", ErrDownloadIdentityConflict
		}
		if expectedHash != "" {
			known := existingHash
			if known == "" {
				known = previous.ExpectedHash
			}
			if known == "" || known != expectedHash {
				return "", ErrDownloadIdentityConflict
			}
		}
		if season != nil {
			if previous.ExpectedSeason == nil || *previous.ExpectedSeason != *season {
				return "", ErrDownloadIdentityConflict
			}
			want, _ := json.Marshal(episodes)
			prior, _ := json.Marshal(previous.ExpectedEpisodes)
			if string(want) != string(prior) {
				return "", ErrDownloadIdentityConflict
			}
		}
		if sourceRequired {
			return "", ErrSourceRecoveryRequired
		}
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", errors.New("resource unavailable")
	}
	id := jobs.UUID()
	tag, err := tx.Exec(ctx, `INSERT INTO download_tasks(id,resource_id,integration_id,media_type,tmdb_id,details)VALUES($1,$2,$3,$4,$5,$6)ON CONFLICT(resource_id)WHERE download_state NOT IN ('failed','cancelled')DO NOTHING`, id, resourceID, connection.Record.ID, mediaType, tmdbID, raw)
	if err != nil {
		return "", errors.New("download creation failed")
	}
	if tag.RowsAffected() == 0 {
		return "", ErrDownloadIdentityConflict
	}
	payload, _ := json.Marshal(map[string]string{"taskId": id})
	if _, err = tx.Exec(ctx, `INSERT INTO jobs(id,kind,payload,idempotency_key)VALUES($1,'download.add',$2,$3)`, jobs.UUID(), payload, "download.add:"+id); err != nil {
		return "", err
	}
	return id, tx.Commit(ctx)
}
func (e Engine) List(ctx context.Context) ([]Task, error) {
	rows, err := e.DB.Query(ctx, `SELECT d.id::text,d.resource_id::text,r.title,d.download_state,d.seeding_state,coalesce(d.info_hash,''),d.details,d.created_at,d.source_recovery_required,d.source_control_revision FROM download_tasks d JOIN resources r ON r.id=d.resource_id ORDER BY d.created_at DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Task{}
	for rows.Next() {
		var task Task
		var raw []byte
		if err = rows.Scan(&task.ID, &task.ResourceID, &task.Title, &task.State, &task.SeedingState, &task.InfoHash, &raw, &task.CreatedAt, &task.SourceRecoveryRequired, &task.SourceControlRevision); err != nil {
			return nil, err
		}
		var details taskDetails
		json.Unmarshal(raw, &details)
		task.Progress = details.Progress
		task.Message = details.Message
		task.Media = details.Media
		out = append(out, task)
	}
	return out, rows.Err()
}
func (e Engine) load(ctx context.Context, id string) (resourceID, connectionID, hash, state string, details taskDetails, err error) {
	var raw []byte
	err = e.DB.QueryRow(ctx, `SELECT resource_id::text,integration_id::text,coalesce(info_hash,''),download_state,details FROM download_tasks WHERE id=$1`, id).Scan(&resourceID, &connectionID, &hash, &state, &raw)
	if err == nil {
		err = json.Unmarshal(raw, &details)
	}
	return
}
func (e Engine) client(ctx context.Context, id string, details taskDetails) (*qbittorrent.Adapter, error) {
	connection, err := e.Connections.Repository.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if connection.Record.Kind != "qbittorrent" || !connection.Record.Enabled || connection.String("endpoint") != details.Endpoint {
		return nil, errors.New("downloader endpoint changed; manual reconciliation required")
	}
	return &qbittorrent.Adapter{Endpoint: connection.String("endpoint"), Username: connection.Values["username"], Password: connection.Values["password"]}, nil
}
func (e Engine) Add(ctx context.Context, job jobs.Job) (result error) {
	var payload struct {
		TaskID string `json:"taskId"`
	}
	if json.Unmarshal(job.Payload, &payload) != nil {
		return errors.New("invalid download job")
	}
	lockConn, lockErr := e.DB.Acquire(ctx)
	if lockErr != nil {
		return lockErr
	}
	defer lockConn.Release()
	if _, lockErr = lockConn.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended($1,0))`, "download-task:"+payload.TaskID); lockErr != nil {
		return lockErr
	}
	defer lockConn.Exec(context.Background(), `SELECT pg_advisory_unlock(hashtextextended($1,0))`, "download-task:"+payload.TaskID)
	resource, connectionID, hash, state, details, err := e.load(ctx, payload.TaskID)
	loaded := err == nil
	defer func() {
		// An unreadable row is not permission to overwrite the original details
		// with an empty local struct after the request context is detached.
		if result != nil && loaded {
			details.Message = "下载提交失败，请检查站点、下载器以及后台任务；修复后可重试"
			if errors.Is(result, jobs.ErrOperatorRequired) {
				details.Message = "下载来源快照已变化，任务暂停；需要重新确认来源，不会自动改用其他搜索结果"
			}
			raw, _ := json.Marshal(details)
			bounded, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
			defer cancel()
			e.DB.Exec(bounded, `UPDATE download_tasks SET details=$2,source_control_revision=CASE WHEN $3 AND NOT source_recovery_required THEN source_control_revision+1 ELSE source_control_revision END,source_recovery_required=source_recovery_required OR $3,updated_at=now()WHERE id=$1`, payload.TaskID, raw, errors.Is(result, jobs.ErrOperatorRequired))
		}
	}()
	if err != nil {
		return err
	}
	if state != "pending" {
		return nil
	}
	var sourceRequired bool
	if err = e.DB.QueryRow(ctx, `SELECT source_recovery_required FROM download_tasks WHERE id=$1`, payload.TaskID).Scan(&sourceRequired); err != nil {
		return err
	}
	if sourceRequired {
		return errors.Join(jobs.ErrOperatorRequired, ErrSourceRecoveryRequired)
	}
	client, err := e.client(ctx, connectionID, details)
	if err != nil {
		return err
	}
	if hash == "" {
		torrent, err := e.taskTorrent(ctx, resource, details)
		if err != nil {
			if sourceSnapshotError(err) {
				return errors.Join(jobs.ErrOperatorRequired, err)
			}
			return errors.New("torrent fetch failed; verify the PT account")
		}
		meta, err := downloads.ParseTorrent(torrent)
		if err != nil {
			return err
		}
		if details.ExpectedHash != "" && details.ExpectedHash != meta.InfoHash {
			return errors.New("torrent identity changed since subscription preview")
		}
		details.HashAliases = meta.Aliases
		hash = meta.InfoHash
		if _, err = e.DB.Exec(ctx, `UPDATE download_tasks SET info_hash=$2,downloader_task_id=$2,updated_at=now() WHERE id=$1`, payload.TaskID, hash); err != nil {
			return err
		}
		rawDetails, _ := json.Marshal(details)
		if _, err = e.DB.Exec(ctx, `UPDATE download_tasks SET details=$2 WHERE id=$1`, payload.TaskID, rawDetails); err != nil {
			return err
		}
		if _, err = inspectDownloadTarget(ctx, client, hash, details); errors.Is(err, qbittorrent.ErrNotFound) {
			if _, err = client.Add(ctx, downloads.Request{Torrent: torrent, SavePath: details.DownloadPath, Category: details.Category, Tags: []string{"media-tool", payload.TaskID}}); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	} else {
		if _, err = inspectDownloadTarget(ctx, client, hash, details); errors.Is(err, qbittorrent.ErrNotFound) {
			torrent, fetchErr := e.taskTorrent(ctx, resource, details)
			if fetchErr != nil {
				if sourceSnapshotError(fetchErr) {
					return errors.Join(jobs.ErrOperatorRequired, fetchErr)
				}
				return errors.New("torrent fetch failed during reconciliation")
			}
			meta, parseErr := downloads.ParseTorrent(torrent)
			if parseErr != nil || meta.InfoHash != hash {
				return errors.New("torrent identity changed; manual confirmation required")
			}
			if _, err = client.Add(ctx, downloads.Request{Torrent: torrent, SavePath: details.DownloadPath, Category: details.Category, Tags: []string{"media-tool", payload.TaskID}}); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}
	return e.transition(ctx, payload.TaskID, "downloading", "download.started", details)
}
func (e Engine) taskTorrent(ctx context.Context, resourceID string, details taskDetails) ([]byte, error) {
	if details.SearchRunID != "" {
		if details.SourceSnapshotDigest == "" {
			return nil, search.ErrSnapshotUnavailable
		}
		current, err := e.Search.SourceSnapshotDigest(ctx, details.SearchRunID, resourceID)
		if err != nil {
			return nil, err
		}
		if current != details.SourceSnapshotDigest {
			return nil, search.ErrSnapshotChanged
		}
		return e.Search.TorrentFromRun(ctx, details.SearchRunID, resourceID)
	}
	return e.Search.Torrent(ctx, resourceID)
}

func (e Engine) transition(ctx context.Context, id, state, event string, details taskDetails) error {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	raw, _ := json.Marshal(details)
	tag, err := tx.Exec(ctx, `UPDATE download_tasks SET download_state=$2,details=$3,updated_at=now() WHERE id=$1 AND download_state<>$2`, id, state, raw)
	if err != nil {
		return err
	}
	if tag.RowsAffected() > 0 && event != "" {
		body, _ := json.Marshal(map[string]any{"taskId": id, "title": details.Media.Title, "state": state})
		if _, err = tx.Exec(ctx, `INSERT INTO outbox_events(id,event_type,aggregate_id,payload)VALUES($1,$2,$3,$4)`, jobs.UUID(), event, id, body); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
func (e Engine) Reconcile(ctx context.Context) error {
	rows, err := e.DB.Query(ctx, `SELECT id::text FROM download_tasks WHERE download_state='downloading' ORDER BY created_at LIMIT 100`)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		e.reconcileTask(ctx, id)
	}
	return nil
}
func (e Engine) reconcileTask(ctx context.Context, id string) {
	_, connectionID, hash, _, details, err := e.load(ctx, id)
	if err != nil {
		return
	}
	client, err := e.client(ctx, connectionID, details)
	if err != nil {
		details.Message = "下载器地址已变更，请人工核对现有任务"
		raw, _ := json.Marshal(details)
		e.DB.Exec(ctx, `UPDATE download_tasks SET details=$2 WHERE id=$1`, id, raw)
		return
	}
	bounded, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	task, err := client.InspectAliases(bounded, hash, details.HashAliases)
	if err != nil {
		return
	}
	details.Progress = task.Progress
	details.QBState = task.State
	raw, _ := json.Marshal(details)
	seeding := "pending"
	if task.Seeding {
		seeding = "seeding"
	}
	e.DB.Exec(ctx, `UPDATE download_tasks SET details=$2,seeding_state=$3,updated_at=now() WHERE id=$1`, id, raw, seeding)
	if !task.Complete {
		return
	}
	files, err := client.Files(bounded, task.ID)
	if err != nil {
		return
	}
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return
	}
	defer tx.Rollback(ctx)
	remoteRoot := path.Clean(details.DownloadPath)
	actualSave := path.Clean(strings.ReplaceAll(task.SavePath, "\\", "/"))
	if actualSave != remoteRoot && !strings.HasPrefix(actualSave, strings.TrimRight(remoteRoot, "/")+"/") {
		details.Message = "下载器保存路径不在共享映射范围内，需人工修正映射"
		raw, _ := json.Marshal(details)
		e.DB.Exec(ctx, `UPDATE download_tasks SET details=$2 WHERE id=$1`, id, raw)
		return
	}
	prefix := strings.TrimPrefix(strings.TrimPrefix(actualSave, remoteRoot), "/")
	root, err := os.OpenRoot(details.LocalPath)
	if err != nil {
		return
	}
	defer root.Close()
	wanted := 0
	for _, file := range files {
		if file.Priority == 0 {
			continue
		}
		wanted++
		if file.Progress < 1 {
			return
		}
		relative := filepath.ToSlash(file.Name)
		if relative == "" || filepath.IsAbs(file.Name) || strings.Contains(relative, "\\") || filepath.ToSlash(filepath.Clean(relative)) != relative || strings.HasPrefix(relative, "../") {
			return
		}
		localRelative := path.Join(prefix, relative)
		local, err := root.Open(localRelative)
		if err != nil {
			return
		}
		stat, err := local.Stat()
		local.Close()
		if err != nil || !stat.Mode().IsRegular() || stat.Size() != file.Size {
			return
		}
		if _, err = tx.Exec(ctx, `INSERT INTO download_files(id,task_id,file_index,relative_path,size_bytes,local_path)VALUES($1,$2,$3,$4,$5,$6)ON CONFLICT(task_id,file_index)DO UPDATE SET relative_path=excluded.relative_path,size_bytes=excluded.size_bytes,local_path=excluded.local_path`, jobs.UUID(), id, file.Index, localRelative, file.Size, filepath.Join(details.LocalPath, filepath.FromSlash(localRelative))); err != nil {
			return
		}
	}
	if wanted == 0 {
		return
	}
	if err = tx.Commit(ctx); err != nil {
		return
	}
	e.transition(ctx, id, "local_complete", "download.completed", details)
}

func sourceSnapshotError(err error) bool {
	return errors.Is(err, search.ErrSnapshotUnavailable) || errors.Is(err, search.ErrSnapshotChanged) || errors.Is(err, search.ErrSnapshotExpired)
}

func inspectDownloadTarget(ctx context.Context, client *qbittorrent.Adapter, hash string, details taskDetails) (downloads.Task, error) {
	task, err := client.InspectAliases(ctx, hash, details.HashAliases)
	if err != nil {
		return task, err
	}
	if !strings.EqualFold(task.ID, hash) && !containsHash(details.HashAliases, task.ID) {
		return task, errors.Join(jobs.ErrOperatorRequired, errors.New("下载器返回不同种子身份，需人工核对"))
	}
	if path.Clean(task.SavePath) != path.Clean(details.DownloadPath) {
		return task, errors.Join(jobs.ErrOperatorRequired, errors.New("下载器保存位置与原任务不同，需人工核对"))
	}
	return task, nil
}
