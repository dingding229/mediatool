package manualdownloads

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"media-tool/internal/adapters/qbittorrent"
	"media-tool/internal/modules/downloads"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"path"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Engine struct {
	DB          *pgxpool.Pool
	Connections *integrations.Manager
}
type Task struct {
	ID            string     `json:"id"`
	InfoHash      string     `json:"infoHash"`
	Title         string     `json:"title"`
	SourceKind    string     `json:"sourceKind"`
	Source        string     `json:"source"`
	Endpoint      string     `json:"endpoint"`
	SavePath      string     `json:"savePath"`
	Category      string     `json:"category"`
	State         string     `json:"state"`
	Progress      *float64   `json:"progress"`
	QBState       string     `json:"qbState"`
	DownloadSpeed int64      `json:"downloadSpeed"`
	UploadSpeed   int64      `json:"uploadSpeed"`
	TotalBytes    int64      `json:"totalBytes"`
	Message       string     `json:"message"`
	SyncedAt      *time.Time `json:"syncedAt"`
	CreatedAt     time.Time  `json:"createdAt"`
}

func (e Engine) Create(ctx context.Context, s Source) (string, error) {
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
	encrypted, err := e.Connections.Repository.Vault.Seal("manual-download:"+id, map[string]string{"torrent": base64.StdEncoding.EncodeToString(s.Torrent), "url": s.URL})
	if err != nil {
		return "", err
	}
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
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
	return existing, tx.Commit(ctx)
}
func (e Engine) List(ctx context.Context) ([]Task, error) {
	rows, err := e.DB.Query(ctx, `SELECT id::text,info_hash,title,source_kind,source_label,endpoint,save_path,category,state,progress,qb_state,download_speed,upload_speed,total_bytes,message,synced_at,created_at FROM manual_download_tasks ORDER BY created_at DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Task{}
	for rows.Next() {
		var t Task
		if err = rows.Scan(&t.ID, &t.InfoHash, &t.Title, &t.SourceKind, &t.Source, &t.Endpoint, &t.SavePath, &t.Category, &t.State, &t.Progress, &t.QBState, &t.DownloadSpeed, &t.UploadSpeed, &t.TotalBytes, &t.Message, &t.SyncedAt, &t.CreatedAt); err != nil {
			return nil, err
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
		e.DB.Exec(bounded, `UPDATE manual_download_tasks SET state=CASE WHEN submit_attempted THEN 'uncertain' ELSE 'blocked' END,message=$2,updated_at=now() WHERE id=$1 AND state NOT IN('downloading','completed','uncertain')`, in.ID, err.Error())
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
		conn.Exec(c, `SELECT pg_advisory_unlock(hashtextextended($1,913))`, id)
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
		conn.Exec(ctx, `UPDATE manual_download_tasks SET state='uncertain',message='此前已提交，下载器尚未发现匹配hash；仅允许刷新核对，不自动重复添加',updated_at=now() WHERE id=$1`, id)
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
		conn.Exec(ctx, `UPDATE manual_download_tasks SET state='uncertain',message='下载器提交结果未知，请刷新核对；不会自动重新添加',updated_at=now() WHERE id=$1`, id)
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
	_, err := e.DB.Exec(ctx, `UPDATE manual_download_tasks SET state=$2,progress=$3,qb_state=$4,download_speed=$5,upload_speed=$6,total_bytes=$7,synced_at=now(),updated_at=now(),message='仅下载，不自动整理或清理本地文件' WHERE id=$1`, id, state, t.Progress, t.State, t.DownloadSpeed, t.UploadSpeed, t.Size)
	return err
}
func (e Engine) Sync(ctx context.Context) error {
	rows, err := e.DB.Query(ctx, `SELECT id::text,integration_id::text,info_hash,endpoint,save_path,category FROM manual_download_tasks WHERE state IN('submitting','uncertain','downloading','completed') ORDER BY updated_at LIMIT 200`)
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
			e.DB.Exec(ctx, `UPDATE manual_download_tasks SET message=$2,updated_at=now() WHERE id=$1`, x.id, err.Error())
			problems = append(problems, err)
		}
	}
	return errors.Join(problems...)
}
