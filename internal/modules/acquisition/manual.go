package acquisition

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"media-tool/internal/modules/metadata"
	pluginv2 "media-tool/internal/modules/plugins/v2"
	deferutil "media-tool/internal/platform/cleanup"
	dbutil "media-tool/internal/platform/database"
)

// AdoptManual enters the existing downloader -> transfer -> verification ->
// STRM/import -> cleanup state machine. It NEVER re-adds an existing torrent.
// The nullable resource FK is deliberate: a manual source is not a PT result.
func (e Engine) AdoptManual(ctx context.Context, id string, media metadata.Item) error {
	if media.ID < 1 || media.MediaType != "movie" && media.MediaType != "series" {
		return errors.New("媒体身份无效")
	}
	var connection, hash, endpoint, save, category, state string
	var encrypted []byte
	if err := e.DB.QueryRow(ctx, `SELECT integration_id::text,info_hash,endpoint,save_path,category,state,payload_ciphertext FROM manual_download_tasks WHERE id=$1`, id).Scan(&connection, &hash, &endpoint, &save, &category, &state, &encrypted); err != nil {
		return err
	}
	options, err := e.Connections.Repository.Vault.Open("manual-download:"+id, encrypted)
	if err != nil {
		return err
	}
	if state != "completed" || options["autoImport"] != "true" || options["localPath"] == "" {
		return errors.New("任务未完成或未明确启用后处理，保留原文件")
	}
	current, err := e.Connections.Repository.Get(ctx, connection)
	if err != nil {
		return err
	}
	if !current.Record.Enabled || current.Record.Kind != "qbittorrent" || current.String("endpoint") != endpoint || current.String("downloadPath") != save || current.String("localPath") != options["localPath"] {
		return errors.New("共享目录或下载器配置已变化，需人工核对")
	}
	info, err := os.Stat(options["localPath"])
	if err != nil || !info.IsDir() {
		return errors.New("共享下载目录不可读取，保留原文件")
	}
	// This is now a managed import request; SDK manual-only calls never opt in.
	ctx, cleanup, err := (pluginv2.Store{DB: e.DB, Vault: e.Connections.Repository.Vault}).PrepareDownload(ctx, contracts.ResourceDownloadRequest{Kind: "managed", ResourceID: id, DownloaderID: connection, CleanupAfterImport: options["cleanupAfterImport"] == "true"})
	if err != nil {
		return err
	}
	details := taskDetails{ManualTaskID: id, Media: media.ShowMetadata(), Endpoint: endpoint, DownloadPath: save, LocalPath: options["localPath"], Category: category, CleanupAfterImport: cleanup}
	raw, err := json.Marshal(details)
	if err != nil {
		return err
	}
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer deferutil.Rollback(ctx, tx)
	var integrationCurrent bool
	if err = tx.QueryRow(ctx, `SELECT enabled AND revision=$2 FROM integrations WHERE id=$1 FOR SHARE`, connection, current.Record.Revision).Scan(&integrationCurrent); err != nil {
		return err
	}
	if !integrationCurrent {
		return errors.New("下载器配置已变化，请重新核对")
	}
	// Fence operator identity changes, source changes and competing adopters.
	var unchanged bool
	if err = tx.QueryRow(ctx, `SELECT state='completed' AND payload_ciphertext=$2 FROM manual_download_tasks WHERE id=$1 FOR UPDATE`, id, encrypted).Scan(&unchanged); err != nil {
		return err
	}
	if !unchanged {
		return errors.New("原任务已变化，请刷新核对")
	}
	var duplicate bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM download_tasks WHERE info_hash=$1 AND id<>$2 AND download_state NOT IN('failed','cancelled'))`, hash, id).Scan(&duplicate); err != nil {
		return err
	}
	if duplicate {
		return errors.New("此种子已有托管任务，请核对现有任务，不重复入库")
	}
	if _, err = tx.Exec(ctx, `INSERT INTO download_tasks(id,integration_id,info_hash,downloader_task_id,download_state,media_type,tmdb_id,details) VALUES($1,$2,$3,$3,'downloading',$4,$5,$6) ON CONFLICT(id) DO NOTHING`, id, connection, hash, media.MediaType, media.ID, raw); err != nil {
		return err
	}
	if err = tx.QueryRow(ctx, `SELECT details->>'manualTaskId'=($1::uuid)::text AND media_type=$2 AND tmdb_id=$3 FROM download_tasks WHERE id=$1 FOR UPDATE`, id, media.MediaType, media.ID).Scan(&unchanged); err != nil {
		return err
	}
	if !unchanged {
		return errors.New("原任务已绑定不同媒体身份，不能覆盖")
	}
	if _, err = tx.Exec(ctx, `UPDATE manual_download_tasks SET message='已接入托管后处理：整理、上传校验、STRM入库；清理仍受设置及做种保护约束',updated_at=now() WHERE id=$1`, id); err != nil {
		return err
	}
	return dbutil.Commit(ctx, tx)
}
