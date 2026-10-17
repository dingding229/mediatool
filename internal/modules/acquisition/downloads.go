package acquisition

import (
	"context"
	"encoding/json"
	"errors"
	deferutil "media-tool/internal/platform/cleanup"
	dbutil "media-tool/internal/platform/database"
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
	"media-tool/internal/modules/settings"
)

type Engine struct {
	DB          *pgxpool.Pool
	Connections *integrations.Manager
	Search      search.Engine
}
type taskDetails struct {
	CleanupAfterImport    bool                `json:"cleanupAfterImport"`
	Trackers              []downloads.Tracker `json:"trackers"`
	TrackerError          string              `json:"trackerError"`
	YearMismatchConfirmed bool                `json:"yearMismatchConfirmed"`
	IdentityOverride      bool                `json:"identityOverride"`
	SyncedAt              *time.Time          `json:"syncedAt"`
	SyncAttemptAt         *time.Time          `json:"syncAttemptAt"`
	SyncError             string              `json:"syncError"`
	DownloadSpeed         int64               `json:"downloadSpeed"`
	UploadSpeed           int64               `json:"uploadSpeed"`
	Seeds                 int                 `json:"seeds"`
	Peers                 int                 `json:"peers"`

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
	CleanupState   string              `json:"cleanupState"`
	CleanupMessage string              `json:"cleanupMessage"`
	Trackers       []downloads.Tracker `json:"trackers"`
	TrackerError   string              `json:"trackerError"`
	DownloadPath   string              `json:"downloadPath"`
	LocalPath      string              `json:"localPath"`
	Endpoint       string              `json:"endpoint"`
	Category       string              `json:"category"`
	SyncedAt       *time.Time          `json:"syncedAt"`
	SyncAttemptAt  *time.Time          `json:"syncAttemptAt"`
	SyncError      string              `json:"syncError"`
	DownloadSpeed  int64               `json:"downloadSpeed"`
	UploadSpeed    int64               `json:"uploadSpeed"`
	Seeds          int                 `json:"seeds"`
	Peers          int                 `json:"peers"`
	QBState        string              `json:"qbState"`

	ID                     string        `json:"id"`
	ResourceID             string        `json:"resourceId"`
	Title                  string        `json:"title"`
	State                  string        `json:"state"`
	SeedingState           string        `json:"seedingState"`
	HashAliases            []string      `json:"hashAliases,omitempty"`
	ExpectedHash           string        `json:"expectedHash,omitempty"`
	ExpectedSeason         *int          `json:"expectedSeason,omitempty"`
	ExpectedEpisodes       []int         `json:"expectedEpisodes,omitempty"`
	Progress               *float64      `json:"progress"`
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
	return e.StartFromRunIdentity(ctx, runID, resourceID, mediaType, tmdbID, false)
}
func (e Engine) StartFromRunIdentity(ctx context.Context, runID, resourceID, mediaType string, tmdbID int, correction bool, yearConfirmation ...bool) (string, error) {
	if runID == "" {
		var err error
		runID, err = e.Search.LatestRunForResource(ctx, resourceID)
		if err != nil {
			return "", err
		}
	}
	yearConfirmed := len(yearConfirmation) > 0 && yearConfirmation[0]
	if !correction {
		if err := e.Search.MediaResource(ctx, runID, resourceID, yearConfirmed); err != nil {
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
	return e.startExpected(ctx, runID, resourceID, mediaType, tmdbID, meta.InfoHash, nil, nil, "", 0, correction, yearConfirmed)
}

func (e Engine) StartExpectedFromRun(ctx context.Context, runID, resourceID, mediaType string, tmdbID int, expectedHash string, season *int, episodes []int, subscriptionRunID string, revision int64) (string, error) {
	return e.startExpected(ctx, runID, resourceID, mediaType, tmdbID, expectedHash, season, episodes, subscriptionRunID, revision)
}

func (e Engine) startExpected(ctx context.Context, runID, resourceID, mediaType string, tmdbID int, expectedHash string, season *int, episodes []int, subscriptionRunID string, revision int64, correction ...bool) (string, error) {
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
	prefs, prefsErr := (settings.Repository{DB: e.DB}).Get(ctx)
	if prefsErr != nil {
		return "", prefsErr
	}
	details := taskDetails{YearMismatchConfirmed: len(correction) > 1 && correction[1], CleanupAfterImport: prefs.Value.CleanupAfterImport, IdentityOverride: len(correction) > 0 && correction[0], SearchRunID: runID, SubscriptionRunID: subscriptionRunID, ExpectedHash: expectedHash, ExpectedSeason: season, ExpectedEpisodes: episodes, Media: media, Endpoint: connection.String("endpoint"), DownloadPath: source, LocalPath: local, Category: connection.String("category")}
	if details.Category == "" {
		details.Category = "media-tool"
	}
	raw, _ := json.Marshal(details)
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer deferutil.Rollback(ctx, tx)
	var subscriptionID string
	if subscriptionRunID != "" {
		var currentRevision, runRevision int64
		var enabled bool
		var phase string
		if err = tx.QueryRow(ctx, `SELECT s.revision,s.enabled,r.revision,r.collection_state,s.id::text FROM subscription_runs r JOIN subscriptions s ON s.id=r.subscription_id WHERE r.id=$1 AND r.search_run_id=$2 FOR SHARE OF s`, subscriptionRunID, runID).Scan(&currentRevision, &enabled, &runRevision, &phase, &subscriptionID); err != nil {
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
		candidate, candidateErr := e.Search.RunResource(ctx, runID, resourceID)
		if candidateErr != nil {
			return "", candidateErr
		}
		if search.MediaYearWarning(candidate, media) != "" && !details.YearMismatchConfirmed {
			return "", search.ErrMediaYearConfirmation
		}
		var rawIdentity []byte
		if err = tx.QueryRow(ctx, `SELECT media_context FROM search_runs WHERE id=$1 FOR SHARE`, runID).Scan(&rawIdentity); err != nil {
			return "", search.ErrSnapshotUnavailable
		}
		var identity metadata.Item
		if len(rawIdentity) > 0 && json.Unmarshal(rawIdentity, &identity) != nil {
			return "", search.ErrSnapshotChanged
		}
		if identity.ID > 0 && (identity.ID != tmdbID || identity.MediaType != mediaType) && !details.IdentityOverride {
			return "", errors.New("搜索中的TMDB身份已变化；请刷新或明确更正身份")
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
		if subscriptionID != "" {
			if bindErr := assignDownloadSubscription(ctx, tx, existing, subscriptionID); bindErr != nil {
				return "", bindErr
			}
			return existing, tx.Commit(ctx)
		}
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", errors.New("resource unavailable")
	}
	id := jobs.UUID()
	tag, err := tx.Exec(ctx, `INSERT INTO download_tasks(id,resource_id,integration_id,media_type,tmdb_id,details,subscription_id)VALUES($1,$2,$3,$4,$5,$6,NULLIF($7::text,'')::uuid)ON CONFLICT(resource_id)WHERE download_state NOT IN ('failed','cancelled')DO NOTHING`, id, resourceID, connection.Record.ID, mediaType, tmdbID, raw, subscriptionID)
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
	rows, err := e.DB.Query(ctx, `SELECT d.id::text,d.resource_id::text,r.title,d.download_state,d.seeding_state,coalesce(d.info_hash,''),d.details,d.created_at,d.source_recovery_required,d.source_control_revision,coalesce(c.state,'disabled'),coalesce(c.message,'') FROM download_tasks d JOIN resources r ON r.id=d.resource_id LEFT JOIN download_cleanup_requests c ON c.task_id=d.id ORDER BY d.created_at DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Task{}
	for rows.Next() {
		var task Task
		var raw []byte
		if err = rows.Scan(&task.ID, &task.ResourceID, &task.Title, &task.State, &task.SeedingState, &task.InfoHash, &raw, &task.CreatedAt, &task.SourceRecoveryRequired, &task.SourceControlRevision, &task.CleanupState, &task.CleanupMessage); err != nil {
			return nil, err
		}
		var details taskDetails
		if json.Unmarshal(raw, &details) != nil {
			return nil, errors.New("download details unavailable")
		}
		if details.QBState != "" {
			value := details.Progress
			task.Progress = &value
		}
		task.SyncedAt, task.SyncAttemptAt, task.SyncError = details.SyncedAt, details.SyncAttemptAt, details.SyncError
		task.QBState, task.DownloadSpeed, task.UploadSpeed, task.Seeds, task.Peers = details.QBState, details.DownloadSpeed, details.UploadSpeed, details.Seeds, details.Peers
		task.HashAliases, task.ExpectedHash, task.ExpectedSeason, task.ExpectedEpisodes = details.HashAliases, details.ExpectedHash, details.ExpectedSeason, details.ExpectedEpisodes
		task.DownloadPath, task.LocalPath, task.Endpoint, task.Category = details.DownloadPath, details.LocalPath, details.Endpoint, details.Category
		task.Trackers, task.TrackerError = details.Trackers, details.TrackerError
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
	defer dbutil.Unlock(context.Background(), lockConn, `SELECT pg_advisory_unlock(hashtextextended($1,0))`, "download-task:"+payload.TaskID)
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
			if _, checkedErr := e.DB.Exec(bounded, `UPDATE download_tasks SET details=$2,source_control_revision=CASE WHEN $3 AND NOT source_recovery_required THEN source_control_revision+1 ELSE source_control_revision END,source_recovery_required=source_recovery_required OR $3,updated_at=now()WHERE id=$1`, payload.TaskID, raw, errors.Is(result, jobs.ErrOperatorRequired)); checkedErr != nil {
				result = errors.Join(result, checkedErr)
				return
			}
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
			release, guardErr := e.guardSubscriptionAdd(ctx, details, resource)
			if guardErr != nil {
				return guardErr
			}
			addErr := func() error {
				defer release()
				_, err := client.Add(ctx, downloads.Request{Torrent: torrent, SavePath: details.DownloadPath, Category: details.Category, Tags: []string{"media-tool", payload.TaskID}})
				return err
			}()
			if err = addErr; err != nil {
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
			release, guardErr := e.guardSubscriptionAdd(ctx, details, resource)
			if guardErr != nil {
				return guardErr
			}
			addErr := func() error {
				defer release()
				_, err := client.Add(ctx, downloads.Request{Torrent: torrent, SavePath: details.DownloadPath, Category: details.Category, Tags: []string{"media-tool", payload.TaskID}})
				return err
			}()
			if err = addErr; err != nil {
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
	defer deferutil.Rollback(ctx, tx)
	raw, _ := json.Marshal(details)
	tag, err := tx.Exec(ctx, `UPDATE download_tasks SET download_state=$2,details=details||($3::jsonb - 'syncedAt' - 'syncAttemptAt' - 'syncError' - 'progress' - 'qbState' - 'downloadSpeed' - 'uploadSpeed' - 'seeds' - 'peers' - 'trackers' - 'trackerError'),updated_at=now() WHERE id=$1 AND download_state<>$2`, id, state, raw)
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
	rows, err := e.DB.Query(ctx, `SELECT id::text FROM download_tasks WHERE info_hash IS NOT NULL AND seeding_state<>'removed' AND download_state NOT IN ('pending','failed','cancelled') ORDER BY coalesce((details->>'syncAttemptAt')::timestamptz,created_at) LIMIT 100`)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if checkedErr := rows.Scan(&id); checkedErr != nil {
			return checkedErr
		}
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
	_, connectionID, hash, state, details, err := e.load(ctx, id)
	if err != nil {
		return
	}
	task, err := e.syncTask(ctx, id, connectionID, hash, details)
	if err != nil {
		return
	}
	details.Progress, details.QBState = task.Progress, task.State
	bounded, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	client, err := e.client(bounded, connectionID, details)
	if err != nil {
		return
	}
	if state != "downloading" || !task.Complete {
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
	defer deferutil.Rollback(ctx, tx)
	remoteRoot := path.Clean(details.DownloadPath)
	actualSave := path.Clean(strings.ReplaceAll(task.SavePath, "\\", "/"))
	if actualSave != remoteRoot && !strings.HasPrefix(actualSave, strings.TrimRight(remoteRoot, "/")+"/") {
		details.Message = "下载器保存路径不在共享映射范围内，需人工修正映射"
		raw, _ := json.Marshal(details)
		if _, checkedErr := e.DB.Exec(ctx, `UPDATE download_tasks SET details=jsonb_set(details,'{message}',($2::jsonb)->'message') WHERE id=$1`, id, raw); checkedErr != nil {
			deferutil.Report(checkedErr, "internal/modules/acquisition/downloads.go:reconcileTask")
			return
		}
		return
	}
	prefix := strings.TrimPrefix(strings.TrimPrefix(actualSave, remoteRoot), "/")
	root, err := os.OpenRoot(details.LocalPath)
	if err != nil {
		return
	}
	defer deferutil.Close(root, "Read/descriptor teardown only; primary operations handle their own I/O result")
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
		deferutil.Close(local, "Read/descriptor teardown only; primary operations handle their own I/O result")
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
	if checkedErr := e.transition(ctx, id, "local_complete", "download.completed", details); checkedErr != nil {
		deferutil.Report(checkedErr, "internal/modules/acquisition/downloads.go:reconcileTask")
		return
	}
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
