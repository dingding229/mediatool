package manualdownloads

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"media-tool/internal/adapters/qbittorrent"
	"media-tool/internal/modules/downloads"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	pluginv2 "media-tool/internal/modules/plugins/v2"
	"media-tool/internal/modules/settings"
	deferutil "media-tool/internal/platform/cleanup"
	dbutil "media-tool/internal/platform/database"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Engine struct {
	DB          *pgxpool.Pool
	Connections *integrations.Manager
}
type Task struct {
	ProcessingState string     `json:"processingState"`
	ManagedTaskID   string     `json:"managedTaskId,omitempty"`
	ID              string     `json:"id"`
	InfoHash        string     `json:"infoHash"`
	Title           string     `json:"title"`
	SourceKind      string     `json:"sourceKind"`
	Source          string     `json:"source"`
	Endpoint        string     `json:"endpoint"`
	SavePath        string     `json:"savePath"`
	Category        string     `json:"category"`
	State           string     `json:"state"`
	SubmitAttempted bool       `json:"submitAttempted"`
	Progress        *float64   `json:"progress"`
	QBState         string     `json:"qbState"`
	DownloadSpeed   int64      `json:"downloadSpeed"`
	UploadSpeed     int64      `json:"uploadSpeed"`
	TotalBytes      int64      `json:"totalBytes"`
	Message         string     `json:"message"`
	SyncedAt        *time.Time `json:"syncedAt"`
	CreatedAt       time.Time  `json:"createdAt"`
}

// Create retains the SDK manual-download-only contract. Administrator UI calls
// CreateWithImport explicitly; old/plugin tasks are never silently opted in.
func (e Engine) Create(ctx context.Context, s Source) (string, error) { return e.create(ctx, s, false) }
func (e Engine) CreateWithImport(ctx context.Context, s Source) (string, error) {
	return e.create(ctx, s, true)
}
func (e Engine) create(ctx context.Context, s Source, auto bool) (string, error) {
	if e.DB == nil || e.Connections == nil {
		return "", errors.New("数据库或下载器不可用")
	}
	_, c, err := e.Connections.QB(ctx)
	if err != nil {
		return "", errors.New("请先配置并启用qBittorrent")
	}
	save := c.String("downloadPath")
	if save == "" {
		return "", errors.New("请先配置下载器的下载目录")
	}
	category := c.String("category")
	if category == "" {
		category = "media-tool"
	}
	id := jobs.UUID()
	cleanup := false
	if auto {
		prefs, prefsErr := (settings.Repository{DB: e.DB}).Get(ctx)
		if prefsErr != nil {
			return "", prefsErr
		}
		cleanup = prefs.Value.CleanupAfterImport
	}
	requestKind := "manual"
	if auto {
		requestKind = "managed"
	}
	ctx, cleanup, err = (pluginv2.Store{DB: e.DB, Vault: e.Connections.Repository.Vault}).PrepareDownload(ctx, contracts.ResourceDownloadRequest{Kind: requestKind, ResourceID: id, DownloaderID: c.Record.ID, CleanupAfterImport: cleanup})
	if err != nil {
		return "", err
	}
	encrypted, err := e.Connections.Repository.Vault.Seal("manual-download:"+id, map[string]string{"torrent": base64.StdEncoding.EncodeToString(s.Torrent), "url": s.URL, "autoImport": strconv.FormatBool(auto), "cleanupAfterImport": strconv.FormatBool(cleanup), "localPath": c.String("localPath")})
	if err != nil {
		return "", err
	}
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer deferutil.Rollback(ctx, tx)
	var current bool
	if err = tx.QueryRow(ctx, `SELECT enabled AND revision=$2 FROM integrations WHERE id=$1 FOR SHARE`, c.Record.ID, c.Record.Revision).Scan(&current); err != nil || !current {
		return "", errors.New("下载器配置已变化，请重新确认")
	}
	var existing string
	err = tx.QueryRow(ctx, `INSERT INTO manual_download_tasks(id,integration_id,info_hash,title,source_kind,source_label,payload_ciphertext,endpoint,save_path,category) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(integration_id,info_hash) DO UPDATE SET info_hash=excluded.info_hash RETURNING id::text`, id, c.Record.ID, s.Meta.InfoHash, s.Meta.Name, s.Kind, s.Label, encrypted, c.String("endpoint"), save, category).Scan(&existing)
	if err != nil {
		return "", err
	}
	if existing == id {
		b, _ := json.Marshal(map[string]string{"id": id})
		_, err = tx.Exec(ctx, `INSERT INTO jobs(id,kind,payload,idempotency_key)VALUES($1,'download.manual.add',$2,$3)`, jobs.UUID(), b, "download.manual.add:"+id)
		if err != nil {
			return "", err
		}
	}
	return existing, dbutil.Commit(ctx, tx)
}
func (e Engine) List(ctx context.Context) ([]Task, error) { return e.Page(ctx, 0, 200, "", "", nil) }
func (e Engine) Page(ctx context.Context, start, limit int, query, id string, downloaderIDs []string) ([]Task, error) {
	if start < 0 || start > 200000 || limit < 1 || limit > 200 || len(query) > 800 {
		return nil, errors.New("invalid page")
	}
	rows, err := e.DB.Query(ctx, `SELECT id::text,info_hash,title,source_kind,source_label,endpoint,save_path,category,state,progress,qb_state,download_speed,upload_speed,total_bytes,message,synced_at,created_at,submit_attempted,payload_ciphertext,coalesce((SELECT download_state FROM download_tasks d WHERE d.id=manual_download_tasks.id AND d.details->>'manualTaskId'=manual_download_tasks.id::text),''),coalesce((SELECT state FROM jobs j WHERE j.idempotency_key='download.manual.import:'||manual_download_tasks.id::text),'') FROM manual_download_tasks WHERE ($3='' OR title ILIKE '%'||$3||'%') AND ($4='' OR id::text=$4) AND(coalesce(cardinality($5::text[]),0)=0 OR integration_id::text=ANY($5)) ORDER BY created_at DESC,id LIMIT $1 OFFSET $2`, limit, start, query, id, downloaderIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Task{}
	for rows.Next() {
		var t Task
		var managed, importJob string
		var sourceCipher []byte
		if err = rows.Scan(&t.ID, &t.InfoHash, &t.Title, &t.SourceKind, &t.Source, &t.Endpoint, &t.SavePath, &t.Category, &t.State, &t.Progress, &t.QBState, &t.DownloadSpeed, &t.UploadSpeed, &t.TotalBytes, &t.Message, &t.SyncedAt, &t.CreatedAt, &t.SubmitAttempted, &sourceCipher, &managed, &importJob); err != nil {
			return nil, err
		}
		t.ProcessingState = "download_only"
		if managed != "" {
			t.ProcessingState = managed
			t.ManagedTaskID = t.ID
		} else if importJob != "" {
			t.ProcessingState = "identifying"
			if importJob == "failed" || importJob == "paused" || importJob == "unknown" || importJob == "cancelled" {
				t.ProcessingState = "needs_confirmation"
			}
		} else if e.Connections != nil {
			options, err := e.Connections.Repository.Vault.Open("manual-download:"+t.ID, sourceCipher)
			if err == nil && options["autoImport"] == "true" {
				t.ProcessingState = "waiting_download"
			}
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
func (e Engine) client(ctx context.Context, id, endpoint string) (*qbittorrent.Adapter, error) {
	c, err := e.Connections.Repository.Get(ctx, id)
	if err != nil || !c.Record.Enabled || c.Record.Kind != "qbittorrent" || c.String("endpoint") != endpoint {
		return nil, errors.New("下载器已变更或停用，请人工核对原任务")
	}
	return &qbittorrent.Adapter{Endpoint: endpoint, Username: c.Values["username"], Password: c.Values["password"]}, nil
}
func (e Engine) Process(ctx context.Context, j jobs.Job) error {
	var in struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(j.Payload, &in) != nil {
		return jobs.ErrOperatorRequired
	}
	err := e.submit(ctx, in.ID)
	if err != nil {
		bounded, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if _, checkedErr := e.DB.Exec(bounded, `UPDATE manual_download_tasks SET state=CASE WHEN submit_attempted THEN 'uncertain' ELSE 'blocked' END,message=$2,updated_at=now() WHERE id=$1 AND state NOT IN('downloading','completed','uncertain')`, in.ID, err.Error()); checkedErr != nil {
			return checkedErr
		}
	}
	return err
}
func (e Engine) submit(ctx context.Context, id string) error {
	// Session advisory lock is held across the outbound side effect. A reclaimed
	// lease cannot issue another add; an attempted uncertain request is inspect-only.
	conn, err := e.DB.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	var locked bool
	if err = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended($1,913))`, id).Scan(&locked); err != nil {
		return err
	}
	if !locked {
		return errors.New("手动任务正在处理中")
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if _, checkedErr := conn.Exec(c, `SELECT pg_advisory_unlock(hashtextextended($1,913))`, id); checkedErr != nil {
			deferutil.Report(checkedErr, "internal/modules/manualdownloads/tasks.go:")
			return
		}
	}()
	var integration, hash, endpoint, save, category, state, kind string
	var encrypted []byte
	var attempted bool
	err = conn.QueryRow(ctx, `SELECT integration_id::text,info_hash,endpoint,save_path,category,state,source_kind,payload_ciphertext,submit_attempted FROM manual_download_tasks WHERE id=$1`, id).Scan(&integration, &hash, &endpoint, &save, &category, &state, &kind, &encrypted, &attempted)
	if err != nil {
		return err
	}
	if state == "completed" || state == "downloading" {
		return nil
	}
	if state == "absent_confirmed" {
		return jobs.ErrOperatorRequired
	}
	client, err := e.client(ctx, integration, endpoint)
	if err != nil {
		return err
	}
	task, inspectErr := client.Inspect(ctx, hash)
	if inspectErr == nil {
		return e.observe(ctx, id, task, save, category)
	}
	if !errors.Is(inspectErr, qbittorrent.ErrNotFound) {
		return inspectErr
	}
	if attempted {
		if _, checkedErr := conn.Exec(ctx, `UPDATE manual_download_tasks SET state='uncertain',message='此前已提交，下载器尚未发现匹配hash；仅允许刷新核对，不自动重复添加',updated_at=now() WHERE id=$1`, id); checkedErr != nil {
			return checkedErr
		}
		return jobs.ErrOperatorRequired
	}
	payload, err := e.Connections.Repository.Vault.Open("manual-download:"+id, encrypted)
	if err != nil {
		return jobs.ErrOperatorRequired
	}
	// Durable intent must precede the request. Unknown outcomes are never retried.
	if _, err = conn.Exec(ctx, `UPDATE manual_download_tasks SET submit_attempted=true,state='submitting',message='正在提交下载器',updated_at=now() WHERE id=$1`, id); err != nil {
		return err
	}
	if kind == "magnet" {
		_, err = client.AddMagnet(ctx, payload["url"], hash, save, category, []string{"media-tool-manual", id})
	} else {
		var b []byte
		b, err = base64.StdEncoding.DecodeString(payload["torrent"])
		if err == nil {
			_, err = client.Add(ctx, downloads.Request{Torrent: b, SavePath: save, Category: category, Tags: []string{"media-tool-manual", id}})
		}
	}
	if err != nil {
		if _, checkedErr := conn.Exec(ctx, `UPDATE manual_download_tasks SET state='uncertain',message='下载器提交结果未知，请刷新核对；不会自动重新添加',updated_at=now() WHERE id=$1`, id); checkedErr != nil {
			return checkedErr
		}
		return errors.Join(jobs.ErrOperatorRequired, err)
	}
	_, err = conn.Exec(ctx, `UPDATE manual_download_tasks SET state='downloading',message='下载器已接受，等待进度同步',updated_at=now() WHERE id=$1`, id)
	return err
}
func (e Engine) observe(ctx context.Context, id string, t downloads.Task, save, category string) error {
	if path.Clean(strings.ReplaceAll(t.SavePath, "\\", "/")) != path.Clean(save) || t.Category != category {
		return errors.New("同hash任务的下载目录不一致，不接管此任务")
	}
	state := "downloading"
	if t.Complete {
		state = "completed"
	}
	_, err := e.DB.Exec(ctx, `UPDATE manual_download_tasks SET state=$2,progress=$3,qb_state=$4,download_speed=$5,upload_speed=$6,total_bytes=$7,synced_at=now(),updated_at=now() WHERE id=$1`, id, state, t.Progress, t.State, t.DownloadSpeed, t.UploadSpeed, t.Size)
	if err == nil {
		err = e.trackImport(ctx, id, t.Complete)
	}
	return err
}
func (e Engine) Sync(ctx context.Context) error {
	rows, err := e.DB.Query(ctx, `SELECT id::text,integration_id::text,info_hash,endpoint,save_path,category FROM manual_download_tasks WHERE state IN('submitting','uncertain','absent_confirmed','downloading','completed') AND NOT EXISTS(SELECT 1 FROM download_tasks d WHERE d.id=manual_download_tasks.id AND d.details->>'manualTaskId'=manual_download_tasks.id::text) ORDER BY updated_at LIMIT 200`)
	if err != nil {
		return err
	}
	type row struct{ id, integration, hash, endpoint, save, category string }
	list := []row{}
	for rows.Next() {
		var x row
		if err = rows.Scan(&x.id, &x.integration, &x.hash, &x.endpoint, &x.save, &x.category); err != nil {
			rows.Close()
			return err
		}
		list = append(list, x)
	}
	rows.Close()
	var problems []error
	for _, x := range list {
		c, err := e.client(ctx, x.integration, x.endpoint)
		if err == nil {
			var t downloads.Task
			t, err = c.Inspect(ctx, x.hash)
			if err == nil {
				err = e.observe(ctx, x.id, t, x.save, x.category)
			}
		}
		if err != nil {
			if _, checkedErr := e.DB.Exec(ctx, `UPDATE manual_download_tasks SET message=$2,updated_at=now() WHERE id=$1`, x.id, err.Error()); checkedErr != nil {
				return checkedErr
			}
			problems = append(problems, err)
		}
	}
	return errors.Join(problems...)
}
