package acquisition

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"media-tool/internal/adapters/qbittorrent"
	"media-tool/internal/modules/jobs"
	"os"
	"path"
	"strings"
	"time"
)

func (e Engine) ApproveCleanup(ctx context.Context, id string, confirm, hrSatisfied bool) error {
	if !confirm {
		return errors.New("请明确确认删除此任务的本地下载文件和种子；此操作不可逆")
	}
	raw, _ := json.Marshal(map[string]any{"hrSatisfied": hrSatisfied})
	_, err := e.DB.Exec(ctx, `INSERT INTO download_cleanup_requests(task_id,evidence)VALUES($1,$2) ON CONFLICT(task_id)DO UPDATE SET state=CASE WHEN download_cleanup_requests.state IN('blocked','pending')THEN 'pending' ELSE download_cleanup_requests.state END,evidence=CASE WHEN download_cleanup_requests.state IN('blocked','pending')THEN excluded.evidence ELSE download_cleanup_requests.evidence END,updated_at=now()`, id, raw)
	return err
}
func (e Engine) ScheduleCleanup(ctx context.Context) error {
	// Intent is frozen on new tasks; enabling a setting never deletes old history.
	if _, err := e.DB.Exec(ctx, `INSERT INTO download_cleanup_requests(task_id,evidence) SELECT id,jsonb_build_object('automatic',true,'hrSatisfied',false) FROM download_tasks WHERE details->>'cleanupAfterImport'='true' AND download_state='imported' ON CONFLICT DO NOTHING`); err != nil {
		return err
	}
	rows, err := e.DB.Query(ctx, `SELECT c.task_id::text FROM download_cleanup_requests c JOIN download_tasks d ON d.id=c.task_id WHERE c.state='pending' AND d.download_state='imported' ORDER BY c.approved_at LIMIT 20`)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		payload, _ := json.Marshal(map[string]string{"taskId": id})
		_, err = e.DB.Exec(ctx, `INSERT INTO jobs(id,kind,queue,payload,idempotency_key)VALUES($1,'download.cleanup','default',$2,$3)ON CONFLICT(idempotency_key)WHERE idempotency_key IS NOT NULL DO UPDATE SET state='pending',attempts=0,available_at=now() WHERE jobs.state='failed'`, jobs.UUID(), payload, "download.cleanup:"+id)
		if err != nil {
			return err
		}
	}
	return nil
}
func (e Engine) Cleanup(ctx context.Context, job jobs.Job) (result error) {
	var payload struct {
		TaskID string `json:"taskId"`
	}
	if json.Unmarshal(job.Payload, &payload) != nil || payload.TaskID == "" {
		return errors.New("invalid cleanup task")
	}
	id := payload.TaskID
	conn, err := e.DB.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err = conn.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended($1,0))`, "download-task:"+id); err != nil {
		return err
	}
	defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock(hashtextextended($1,0))`, "download-task:"+id)
	var cleanupState string
	var evidence []byte
	if err = e.DB.QueryRow(ctx, `SELECT state,evidence FROM download_cleanup_requests WHERE task_id=$1`, id).Scan(&cleanupState, &evidence); err != nil {
		return err
	}
	if cleanupState == "succeeded" || cleanupState == "blocked" {
		return nil
	}
	if cleanupState == "deleting" || cleanupState == "uncertain" {
		return jobs.ErrOperatorRequired
	}
	defer func() {
		if result != nil {
			bounded, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
			defer cancel()
			e.DB.Exec(bounded, `UPDATE download_cleanup_requests SET state=CASE WHEN state='deleting'THEN 'uncertain' ELSE 'blocked'END,message=$2,updated_at=now()WHERE task_id=$1`, id, result.Error())
		}
	}()
	resource, integration, hash, state, details, err := e.load(ctx, id)
	if err != nil {
		return err
	}
	if state != "imported" || hash == "" {
		return errors.New("入库尚未完成，不能清理")
	}
	var allowHR struct {
		Satisfied bool `json:"hrSatisfied"`
	}
	json.Unmarshal(evidence, &allowHR)
	var metadataRaw []byte
	// HR admission is bound to the original frozen search result, never a
	// mutable resource row changed by another search or promotion.
	metadataRaw = []byte("{}")
	if details.SearchRunID != "" {
		var snapshot []byte
		if err = e.DB.QueryRow(ctx, `SELECT public_snapshot FROM search_run_resources WHERE run_id=$1 AND resource_id=$2`, details.SearchRunID, resource).Scan(&snapshot); err == nil {
			var row struct {
				Metadata json.RawMessage `json:"metadata"`
			}
			if json.Unmarshal(snapshot, &row) == nil && len(row.Metadata) > 0 {
				metadataRaw = row.Metadata
			}
		} else if !allowHR.Satisfied {
			return errors.New("原搜索H&R证据缺失，不自动清理")
		}
	}
	var metadata map[string]any
	json.Unmarshal(metadataRaw, &metadata)
	hr, known := metadata["hr"].(bool)
	if (!known || hr) && !allowHR.Satisfied {
		return errors.New("H&R未知或尚未确认满足站点做种要求，不自动清理；完成做种后可逐项明确批准")
	}
	var unsafe bool
	if err = e.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM download_files f LEFT JOIN transfer_tasks t ON t.download_file_id=f.id LEFT JOIN media_asset_bindings b ON b.download_file_id=f.id LEFT JOIN media_assets a ON a.id=b.asset_id LEFT JOIN cloud_files c ON c.id=a.cloud_file_id WHERE f.task_id=$1 AND f.media_state NOT IN('non_media','sample') AND (t.id IS NULL OR t.state<>'imported' OR c.state IS DISTINCT FROM 'present' OR c.size_bytes IS DISTINCT FROM f.size_bytes OR t.source_sha1 IS NULL)) OR NOT EXISTS(SELECT 1 FROM download_files WHERE task_id=$1) OR EXISTS(SELECT 1 FROM download_tasks WHERE id<>$1 AND info_hash=$2)`, id, hash).Scan(&unsafe); err != nil {
		return err
	}
	if unsafe {
		return errors.New("云端校验/入库证据不完整，或其他任务共用此种子，不清理")
	}
	// Re-observe current provider identity and SHA1 immediately before any deletion.
	rows, verifyErr := e.DB.Query(ctx, `SELECT t.integration_id::text,c.full_path,t.source_sha1,f.relative_path,f.size_bytes,coalesce(c.provider_file_id,'') FROM transfer_tasks t JOIN download_files f ON f.id=t.download_file_id JOIN media_asset_bindings b ON b.download_file_id=f.id JOIN media_assets a ON a.id=b.asset_id JOIN cloud_files c ON c.id=a.cloud_file_id WHERE f.task_id=$1`, id)
	if verifyErr != nil {
		return verifyErr
	}
	type proof struct {
		connection, destination, sha, relative, provider string
		size                                             int64
	}
	proofs := []proof{}
	for rows.Next() {
		var p proof
		if err = rows.Scan(&p.connection, &p.destination, &p.sha, &p.relative, &p.size, &p.provider); err != nil {
			rows.Close()
			return err
		}
		proofs = append(proofs, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(proofs) == 0 {
		return errors.New("没有可验证的上传媒体，不清理")
	}
	fs, err := os.OpenRoot(details.LocalPath)
	if err != nil {
		return err
	}
	defer fs.Close()
	for _, p := range proofs {
		cloud, err := e.Connections.CloudByID(ctx, p.connection)
		if err != nil {
			return err
		}
		actual, err := cloud.Find(ctx, p.destination)
		if err != nil || actual.IsDirectory || actual.ID != p.provider || actual.Size != p.size || !strings.EqualFold(actual.Hashes[2], p.sha) {
			return errors.New("云端当前文件身份/大小/SHA1无法复核，保留本地源")
		}
		if p.relative == "" || path.IsAbs(p.relative) || strings.HasPrefix(p.relative, "../") || path.Clean(p.relative) != p.relative {
			return errors.New("本地路径不安全")
		}
		file, err := fs.Open(p.relative)
		if err != nil {
			return err
		}
		stat, err := file.Stat()
		if err != nil || !stat.Mode().IsRegular() || stat.Size() != p.size {
			file.Close()
			return errors.New("本地源文件已变化")
		}
		h := sha1.New()
		_, err = io.Copy(h, file)
		file.Close()
		if err != nil || !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), p.sha) {
			return errors.New("本地SHA1已变化，保留源文件")
		}
	}
	var shared bool
	if err = e.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM download_tasks WHERE id<>$1 AND (info_hash=$2 OR details->'hashAliases' ? $2 OR info_hash=ANY($3::text[])))`, id, hash, details.HashAliases).Scan(&shared); err != nil {
		return err
	}
	if shared {
		return errors.New("其他下载任务共享种子身份或别名，保留源文件")
	}
	client, err := e.client(ctx, integration, details)
	if err != nil {
		return err
	}
	task, err := client.InspectAliases(ctx, hash, details.HashAliases)
	if err != nil {
		return err
	}
	if !task.Complete || path.Clean(task.SavePath) != path.Clean(details.DownloadPath) {
		return errors.New("qB完成状态或保存目录与任务不匹配")
	}
	tags, save, err := client.CleanupInfo(ctx, task.ID)
	if err != nil {
		return err
	}
	owned := false
	for _, tag := range strings.Split(tags, ",") {
		if strings.TrimSpace(tag) == id {
			owned = true
		}
	}
	if !owned || path.Clean(save) != path.Clean(details.DownloadPath) {
		return errors.New("缺少本程序的任务标签或保存目录证据，不删除外部种子")
	}
	files, err := client.Files(ctx, task.ID)
	if err != nil || len(files) == 0 {
		return errors.New("种子文件列表无法核对")
	}
	ownedRows, inventoryErr := e.DB.Query(ctx, `SELECT relative_path,size_bytes FROM download_files WHERE task_id=$1`, id)
	if inventoryErr != nil {
		return inventoryErr
	}
	inventory := map[string]int64{}
	for ownedRows.Next() {
		var p string
		var size int64
		if err = ownedRows.Scan(&p, &size); err != nil {
			ownedRows.Close()
			return err
		}
		inventory[p] = size
	}
	err = ownedRows.Err()
	ownedRows.Close()
	if err != nil {
		return err
	}
	paths := map[string]bool{}
	for _, f := range files {
		if f.Name == "" || path.IsAbs(f.Name) || path.Clean(f.Name) != f.Name || strings.HasPrefix(f.Name, "../") || strings.Contains(f.Name, "\\") {
			return errors.New("种子文件路径不安全")
		}
		if size, ok := inventory[f.Name]; !ok || size != f.Size {
			if _, statErr := fs.Lstat(f.Name); !errors.Is(statErr, os.ErrNotExist) {
				return errors.New("qB文件清单包含本程序未登记的实际文件，不删除")
			}
		} else {
			stat, statErr := fs.Lstat(f.Name)
			if statErr != nil || !stat.Mode().IsRegular() || stat.Size() != size {
				return errors.New("本地文件清单变化或存在符号链接，不删除")
			}
		}
		paths[path.Join(save, f.Name)] = true
	}
	hashes, err := client.AllHashes(ctx)
	if err != nil {
		return err
	}
	for _, other := range hashes {
		if strings.EqualFold(other, task.ID) {
			continue
		}
		_, otherSave, err := client.CleanupInfo(ctx, other)
		if err != nil {
			return err
		}
		otherFiles, err := client.Files(ctx, other)
		if err != nil {
			return err
		}
		for _, f := range otherFiles {
			if paths[path.Join(otherSave, f.Name)] {
				return errors.New("其他种子共用下载文件，不能清理")
			}
		}
	}
	if _, err = e.DB.Exec(ctx, `UPDATE download_cleanup_requests SET state='deleting',message='已核对全部云端媒体、H&R声明、hash、目录、标签和共享文件',updated_at=now()WHERE task_id=$1`, id); err != nil {
		return err
	}
	if job.ID != "" {
		var owned bool
		if err = e.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM jobs WHERE id=$1 AND lease_token=$2 AND state='running' AND lease_until>now())`, job.ID, job.LeaseToken).Scan(&owned); err != nil || !owned {
			return errors.Join(jobs.ErrLeaseLost, jobs.ErrOperatorRequired)
		}
	}
	if err = client.DeleteOwned(ctx, task.ID, true); err != nil {
		return errors.Join(jobs.ErrOperatorRequired, err)
	}
	if _, err = client.Inspect(ctx, task.ID); !errors.Is(err, qbittorrent.ErrNotFound) {
		return errors.Join(jobs.ErrOperatorRequired, errors.New("删除响应后种子仍可见或状态未知，请只读核对，不重放删除"))
	}
	for _, f := range files {
		if _, statErr := fs.Stat(f.Name); !errors.Is(statErr, os.ErrNotExist) {
			return errors.Join(jobs.ErrOperatorRequired, errors.New("qB种子已移除，但本地文件删除尚未核实；不自动重放删除"))
		}
	}
	_, err = e.DB.Exec(ctx, `UPDATE download_cleanup_requests SET state='succeeded' ,message='qB种子已删除，对应下载文件由qB清理；云端媒体、STRM及播放Key保留',updated_at=now() WHERE task_id=$1`, id)
	if err == nil {
		_, err = e.DB.Exec(ctx, `UPDATE download_tasks SET seeding_state='removed' WHERE id=$1`, id)
	}
	if err == nil {
		_, err = e.DB.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','download.cleanup_complete',jsonb_build_object('taskId',$1::text,'cloudMediaPreserved',true,'strmKeyPreserved',true))`, id)
	}
	return err
}

func (e Engine) ReconcileCleanup(ctx context.Context, id string) error {
	var state string
	if err := e.DB.QueryRow(ctx, `SELECT state FROM download_cleanup_requests WHERE task_id=$1`, id).Scan(&state); err != nil {
		return err
	}
	if state != "uncertain" && state != "deleting" {
		return errors.New("仅核对结果未知的清理请求")
	}
	_, integration, hash, _, details, err := e.load(ctx, id)
	if err != nil {
		return err
	}
	client, err := e.client(ctx, integration, details)
	if err != nil {
		return err
	}
	if _, err = client.InspectAliases(ctx, hash, details.HashAliases); !errors.Is(err, qbittorrent.ErrNotFound) {
		return errors.New("种子仍存在或下载器不可核验；保留记录，不重放删除")
	}
	rows, err := e.DB.Query(ctx, `SELECT relative_path FROM download_files WHERE task_id=$1`, id)
	if err != nil {
		return err
	}
	paths := []string{}
	for rows.Next() {
		var p string
		if rows.Scan(&p) != nil {
			rows.Close()
			return errors.New("文件证据不可读")
		}
		paths = append(paths, p)
	}
	rows.Close()
	fs, err := os.OpenRoot(details.LocalPath)
	if err != nil {
		return err
	}
	defer fs.Close()
	for _, p := range paths {
		if _, err := fs.Stat(p); !errors.Is(err, os.ErrNotExist) {
			return errors.New("本地仍有文件或不可核验，不宣称删除完成")
		}
	}
	_, err = e.DB.Exec(ctx, `UPDATE download_cleanup_requests SET state='succeeded',message='只读核对：种子及已记录下载文件均不存在',updated_at=now()WHERE task_id=$1`, id)
	if err == nil {
		_, err = e.DB.Exec(ctx, `UPDATE download_tasks SET seeding_state='removed'WHERE id=$1`, id)
	}
	return err
}
