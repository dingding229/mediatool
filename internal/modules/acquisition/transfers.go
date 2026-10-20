package acquisition

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"media-tool/internal/modules/classification"
	"media-tool/internal/modules/library"
	"media-tool/internal/modules/metadata"
	"media-tool/internal/modules/playbackgrants"
	"media-tool/internal/modules/settings"
	deferutil "media-tool/internal/platform/cleanup"
	dbutil "media-tool/internal/platform/database"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"media-tool/internal/adapters/clouddrive2"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/storage"
)

func (e Engine) PrepareTransfers(ctx context.Context) error {
	cloud, err := e.Connections.Repository.Default(ctx, "clouddrive2")
	if err != nil {
		return nil
	}
	root := cloud.String("cloudRoot")
	if root == "" {
		root = e.Connections.Config.STRMAllowedRoots[0]
	}
	rows, err := e.DB.Query(ctx, `SELECT f.id::text,f.relative_path,d.id::text,d.details FROM download_files f JOIN download_tasks d ON d.id=f.task_id LEFT JOIN transfer_tasks t ON t.download_file_id=f.id WHERE d.download_state='local_complete' AND t.id IS NULL AND f.media_state='pending' LIMIT 100`)
	if err != nil {
		return err
	}
	type candidate struct {
		fileID, name, taskID string
		details              []byte
	}
	items := []candidate{}
	for rows.Next() {
		var item candidate
		if err = rows.Scan(&item.fileID, &item.name, &item.taskID, &item.details); err != nil {
			rows.Close()
			return err
		}
		items = append(items, item)
	}
	rows.Close()
	for _, item := range items {
		extension := strings.ToLower(path.Ext(item.name))
		if extension != ".mkv" && extension != ".mp4" && extension != ".avi" && extension != ".mov" && extension != ".m4v" && extension != ".ts" {
			if _, checkedErr := e.DB.Exec(ctx, `UPDATE download_files SET media_state='non_media' WHERE id=$1`, item.fileID); checkedErr != nil {
				return checkedErr
			}
			continue
		}
		if strings.EqualFold(strings.TrimSuffix(path.Base(item.name), path.Ext(item.name)), "sample") || strings.Contains(strings.ToLower(item.name), "/sample/") {
			if _, checkedErr := e.DB.Exec(ctx, `UPDATE download_files SET media_state='sample' WHERE id=$1`, item.fileID); checkedErr != nil {
				return checkedErr
			}
			continue
		}
		var details taskDetails
		if json.Unmarshal(item.details, &details) != nil {
			continue
		}
		prefs, prefErr := (settings.Repository{DB: e.DB}).Get(ctx)
		if prefErr != nil {
			return prefErr
		}
		policy, policyErr := (classification.Repository{DB: e.DB}).Get(ctx)
		if policyErr != nil {
			return policyErr
		}
		season, episodes, parseErr := coverage(item.name, details.Media.MediaType)
		if parseErr != nil {
			if _, checkedErr := e.DB.Exec(ctx, `UPDATE download_files SET media_state='needs_confirmation'WHERE id=$1`, item.fileID); checkedErr != nil {
				return checkedErr
			}
			continue
		}
		rel, match, layoutErr := classification.Render(policy.Value, details.Media, item.name, prefs.Value.MovieTemplate, prefs.Value.SeriesTemplate, extension, season, episodes)
		if layoutErr != nil {
			return layoutErr
		}
		destination := path.Join(root, rel)
		layout, _ := json.Marshal(map[string]any{"movieTemplate": prefs.Value.MovieTemplate, "seriesTemplate": prefs.Value.SeriesTemplate, "classification": policy.Value, "category": match, "source": item.name, "media": details.Media, "revision": policy.Revision, "settingsRevision": prefs.Revision})
		tx, err := e.DB.Begin(ctx)
		if err != nil {
			return err
		}
		id := jobs.UUID()
		tag, err := tx.Exec(ctx, `INSERT INTO transfer_tasks(id,download_file_id,integration_id,destination,device_id,endpoint_snapshot,layout_snapshot)VALUES($1,$2,$3,$4,$5,$6,$7)ON CONFLICT DO NOTHING`, id, item.fileID, cloud.Record.ID, destination, "media-tool-"+cloud.Record.ID, cloud.String("endpoint"), layout)
		if err == nil && tag.RowsAffected() == 0 {
			_, err = tx.Exec(ctx, `UPDATE download_files SET media_state='needs_confirmation'WHERE id=$1`, item.fileID)
		}
		if err == nil && tag.RowsAffected() > 0 {
			payload, _ := json.Marshal(map[string]string{"transferId": id})
			_, err = tx.Exec(ctx, `INSERT INTO jobs(id,kind,payload,queue,idempotency_key)VALUES($1,'transfer.upload',$2,'transfer',$3)`, jobs.UUID(), payload, "transfer:"+id)
		}
		if err != nil {
			deferutil.Rollback(ctx, tx)
			return err
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}
func (e Engine) Transfer(ctx context.Context, job jobs.Job) (result error) {
	var payload struct {
		ID string `json:"transferId"`
	}
	if json.Unmarshal(job.Payload, &payload) != nil {
		return errors.New("invalid transfer job")
	}
	var fileID, connectionID, destination, deviceID, session, storedHash, state, relative, taskID string
	var endpointSnapshot string
	var controlRevision int64
	var size int64
	var raw []byte
	err := e.DB.QueryRow(ctx, `SELECT t.download_file_id::text,t.integration_id::text,t.destination,t.device_id,coalesce(t.upload_id,''),coalesce(t.source_sha1,''),t.state,t.control_revision,coalesce(t.endpoint_snapshot,''),f.relative_path,f.size_bytes,d.id::text,d.details FROM transfer_tasks t JOIN download_files f ON f.id=t.download_file_id JOIN download_tasks d ON d.id=f.task_id WHERE t.id=$1`, payload.ID).Scan(&fileID, &connectionID, &destination, &deviceID, &session, &storedHash, &state, &controlRevision, &endpointSnapshot, &relative, &size, &taskID, &raw)
	if err != nil {
		return err
	}
	if state == "needs_output_reconciliation" || state == outputApprovedState {
		return errors.Join(jobs.ErrOperatorRequired, library.ErrOutputLayoutRequired)
	}
	if state == "paused" || state == "cancelled" {
		return errors.New("upload paused or cancelled by operator")
	}
	if state == "imported" || state == "verified" || state == "artifacts_pending" {
		return e.ImportFile(ctx, payload.ID)
	}

	workCtx, cancelWork := context.WithCancel(ctx)
	defer cancelWork()
	ctx = workCtx
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				bounded, stop := context.WithTimeout(ctx, 2*time.Second)
				var revision int64
				err := e.DB.QueryRow(bounded, `SELECT control_revision FROM transfer_tasks WHERE id=$1`, payload.ID).Scan(&revision)
				stop()
				if err != nil || revision != controlRevision {
					cancelWork()
					return
				}
			}
		}
	}()

	var details taskDetails
	if json.Unmarshal(raw, &details) != nil {
		return errors.New("invalid media details")
	}
	connection, err := e.Connections.Repository.Get(ctx, connectionID)
	if err != nil {
		return err
	}
	if endpointSnapshot != "" && connection.String("endpoint") != endpointSnapshot {
		return errors.New("upload endpoint changed; manual reconciliation required")
	}
	client, err := e.Connections.CloudByID(ctx, connectionID)
	if err != nil {
		return err
	}
	localRoot, err := os.OpenRoot(details.LocalPath)
	if err != nil {
		return errors.New("shared download root unavailable")
	}
	defer deferutil.Close(localRoot, "Read/descriptor teardown only; primary operations handle their own I/O result")
	file, err := localRoot.Open(filepath.FromSlash(relative))
	if err != nil {
		return errors.New("upload source outside allowed root or missing")
	}
	defer deferutil.Close(file, "Read/descriptor teardown only; primary operations handle their own I/O result")
	stat, err := file.Stat()
	if err != nil || !stat.Mode().IsRegular() || stat.Size() != size {
		return errors.New("upload source size changed")
	}
	defer func() {
		if result != nil && !errors.Is(result, jobs.ErrOperatorRequired) {
			failure := context.WithoutCancel(ctx)
			bounded, cancel := context.WithTimeout(failure, 3*time.Second)
			defer cancel()
			if _, checkedErr := e.DB.Exec(bounded, `UPDATE transfer_tasks SET last_error=$2,updated_at=now() WHERE id=$1`, payload.ID, "上传或入库失败，请检查任务和连接；敏感上游详情已隐藏"); checkedErr != nil {
				result = errors.Join(result, checkedErr)
				return
			}
		}
	}()
	hashValue := storedHash
	{
		h := sha1.New()
		buffer := make([]byte, 1<<20)
		for {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			n, readErr := file.Read(buffer)
			if n > 0 {
				h.Write(buffer[:n])
			}
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				return errors.New("source hash read failed")
			}
		}
		computed := hex.EncodeToString(h.Sum(nil))
		if hashValue != "" && !strings.EqualFold(hashValue, computed) {
			return errors.New("source content changed since session was created; do not resume")
		}
		hashValue = computed
		if _, err = e.DB.Exec(ctx, `UPDATE transfer_tasks SET source_sha1=$2 WHERE id=$1`, payload.ID, hashValue); err != nil {
			return err
		}
	}
	{
		remote, lookupErr := client.Find(ctx, destination)
		if lookupErr == nil {
			if remote.Size != size || !strings.EqualFold(remote.Hashes[uint32(2)], hashValue) {
				return errors.New("remote destination conflict; do not overwrite existing files")
			}
			release, err := e.uploadFence(ctx, payload.ID, controlRevision, job)
			if err != nil {
				return err
			}
			defer release()
			if err = e.markVerifiedRecord(ctx, payload.ID, remote); err != nil {
				return err
			}
			release()
			return e.ImportFile(ctx, payload.ID)
		}
		if !errors.Is(lookupErr, storage.ErrNotFound) {
			return lookupErr
		}
		if (state == "uncertain" || state == "starting") && session == "" {
			return errors.New("upload start outcome unknown; reconcile remote tasks before retry")
		}
	}
	root := connection.String("cloudRoot")
	if root == "" {
		root = e.Connections.Config.STRMAllowedRoots[0]
	}
	release, err := e.uploadFence(ctx, payload.ID, controlRevision, job)
	if err != nil {
		return err
	}
	err = client.EnsureFolder(ctx, path.Dir(destination), root)
	release()
	if err != nil {
		return err
	}
	if session == "" {
		if _, err = e.DB.Exec(ctx, `UPDATE transfer_tasks SET state='starting',last_error='',updated_at=now() WHERE id=$1 AND control_revision=$2`, payload.ID, controlRevision); err != nil {
			return err
		}
	} else {
		release, err = e.uploadFence(ctx, payload.ID, controlRevision, job)
		if err != nil {
			return err
		}
		err = client.ResumeUpload(ctx, session)
		release()
		if err != nil {
			return err
		}
	}
	remote, err := client.UploadFile(ctx, clouddrive2.UploadRequest{Fence: func(c context.Context) (func(), error) { return e.uploadFence(c, payload.ID, controlRevision, job) }, File: file, Destination: destination, DeviceID: deviceID, SessionID: session, ExpectedSHA1: hashValue, OnStart: func(id string) error {
		session = id
		_, err := e.DB.Exec(ctx, `UPDATE transfer_tasks SET upload_id=$2,state='uploading',updated_at=now() WHERE id=$1 AND control_revision=$3`, payload.ID, id, controlRevision)
		return err
	}, OnChunk: func(start, end int64) error {
		_, err := e.DB.Exec(ctx, `UPDATE transfer_tasks SET transferred_ranges=transferred_ranges+int8multirange(int8range($2,$3,'[)')),updated_at=now()WHERE id=$1 AND control_revision=$4`, payload.ID, start, end, controlRevision)
		if err != nil {
			return err
		}
		_, err = e.DB.Exec(ctx, `UPDATE transfer_tasks SET bytes_transferred=(SELECT coalesce(sum(upper(r)-lower(r)),0) FROM unnest(transferred_ranges) AS r)WHERE id=$1 AND control_revision=$2`, payload.ID, controlRevision)
		return err
	}, OnProgress: func(bytes int64, phase string) error {
		_, err := e.DB.Exec(ctx, `UPDATE transfer_tasks SET hash_bytes=CASE WHEN $3='hashing' THEN $2 ELSE hash_bytes END,phase=$3,updated_at=now() WHERE id=$1 AND control_revision=$4`, payload.ID, bytes, phase, controlRevision)
		return err
	}})
	if err != nil {
		if session == "" {
			if _, checkedErr := e.DB.Exec(context.WithoutCancel(ctx), `UPDATE transfer_tasks SET state='uncertain' WHERE id=$1 AND control_revision=$2`, payload.ID, controlRevision); checkedErr != nil {
				return checkedErr
			}
		}
		return err
	}
	if remote.Hashes[2] != "" && !strings.EqualFold(remote.Hashes[2], hashValue) {
		return errors.New("remote file hash mismatch")
	}
	release, err = e.uploadFence(ctx, payload.ID, controlRevision, job)
	if err != nil {
		return err
	}
	if err = e.markVerifiedRecord(ctx, payload.ID, remote); err != nil {
		release()
		return err
	}
	release()
	return e.ImportFile(ctx, payload.ID)
}
func (e Engine) markVerified(ctx context.Context, id string, remote storage.File) error {
	if err := e.markVerifiedRecord(ctx, id, remote); err != nil {
		return err
	}
	return e.ImportFile(ctx, id)
}
func (e Engine) markVerifiedRecord(ctx context.Context, id string, remote storage.File) error {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer deferutil.Rollback(ctx, tx)
	var fileID, connectionID string
	if err = tx.QueryRow(ctx, `SELECT download_file_id::text,integration_id::text FROM transfer_tasks WHERE id=$1 FOR UPDATE`, id).Scan(&fileID, &connectionID); err != nil {
		return err
	}
	c, err := e.Connections.Repository.Get(ctx, connectionID)
	if err != nil {
		return err
	}
	settings, _ := json.Marshal(c.Record.Settings)
	if _, err = tx.Exec(ctx, `INSERT INTO storage_connections(id,name,provider,settings,enabled)VALUES($1,$2,'clouddrive2',$3,true)ON CONFLICT(id)DO NOTHING`, connectionID, c.Record.Name, settings); err != nil {
		return err
	}
	metadata, _ := json.Marshal(remote.Hashes)
	tag, err := tx.Exec(ctx, `INSERT INTO cloud_files(id,storage_id,provider_file_id,full_path,size_bytes,metadata)VALUES($1,$2,$3,$4,$5,$6)ON CONFLICT(storage_id,full_path)DO UPDATE SET provider_file_id=excluded.provider_file_id,size_bytes=excluded.size_bytes,metadata=excluded.metadata,state='present',updated_at=now() WHERE NOT EXISTS(SELECT 1 FROM media_assets WHERE cloud_file_id=cloud_files.id) OR (cloud_files.provider_file_id=excluded.provider_file_id AND cloud_files.size_bytes=excluded.size_bytes AND (coalesce(cloud_files.metadata->>'2','')='' OR lower(cloud_files.metadata->>'2')=lower(coalesce(excluded.metadata->>'2',''))))`, jobs.UUID(), connectionID, remote.ID, remote.Path, remote.Size, metadata)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("cloud path already belongs to different media; original playback grant not rebound")
	}
	if _, err = tx.Exec(ctx, `UPDATE transfer_tasks SET state='verified',phase='verified',last_error='',updated_at=now()WHERE id=$1`, id); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}
func (e Engine) ImportFile(ctx context.Context, transferID string) error {
	var fileID, connectionID, cloudPath, taskID, relative, state string
	var raw []byte
	err := e.DB.QueryRow(ctx, `SELECT t.download_file_id::text,t.integration_id::text,t.destination,d.id::text,f.relative_path,d.details,t.state FROM transfer_tasks t JOIN download_files f ON f.id=t.download_file_id JOIN download_tasks d ON d.id=f.task_id WHERE t.id=$1`, transferID).Scan(&fileID, &connectionID, &cloudPath, &taskID, &relative, &raw, &state)
	if err != nil {
		return err
	}
	if state == "needs_output_reconciliation" || state == outputApprovedState {
		return errors.Join(jobs.ErrOperatorRequired, library.ErrOutputLayoutRequired)
	}
	if state == "imported" {
		return e.finishTask(ctx, taskID)
	}
	if state != "verified" && state != "artifacts_pending" {
		return errors.Join(jobs.ErrOperatorRequired, errors.New("只有已校验云端文件可进入入库；暂停/取消/未知上传阶段不能绕过控制"))
	}

	var existing bool
	if err = e.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM strm_entries WHERE asset_id=$1)`, fileID).Scan(&existing); err != nil {
		return err
	}
	if existing {
		return e.resumePersistedImport(ctx, transferID, fileID)
	}
	var details taskDetails
	if json.Unmarshal(raw, &details) != nil {
		return errors.New("media identity unavailable")
	}
	var season, episode *int
	episodes := []int{}
	if details.Media.MediaType == "series" {
		s, ep, ok := metadata.Episodes(relative)
		if !ok {
			return errors.New("series episode unrecognized; manual mapping required")
		}
		season = &s
		episodes = ep
		episode = &episodes[0]
	}

	// Detect old output geometry before calling any metadata/network provider.
	preliminary, err := e.TransferProjections(ctx, transferID, cloudPath, fileID, details.Media, season, episodes)
	if err != nil {
		return err
	}
	guard, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	layoutErr := library.CheckOutputLayout(ctx, guard, fileID, preliminary)
	if rollbackErr := guard.Rollback(ctx); rollbackErr != nil {
		return rollbackErr
	}
	if errors.Is(layoutErr, library.ErrOutputLayoutRequired) {
		return e.blockOutputImport(ctx, transferID, fileID)
	}
	if layoutErr != nil {
		return layoutErr
	}

	if details.Media.MediaType == "series" {
		tmdb, err := e.Connections.TMDB(ctx)
		if err != nil {
			return err
		}
		enriched, err := tmdb.WithEpisodes(ctx, details.Media, *season, episodes)
		if err != nil {
			return err
		}
		details.Media = enriched
	}

	details.Media, err = (library.Engine{DB: e.DB, Connections: e.Connections}).EnrichScraping(ctx, details.Media, season)
	if err != nil {
		return err
	}

	ctx, err = (library.Engine{DB: e.DB, Connections: e.Connections}).PrepareOutputSettings(ctx)
	if err != nil {
		return err
	}
	projections, err := e.TransferProjections(ctx, transferID, cloudPath, fileID, details.Media, season, episodes)
	if err != nil {
		return err
	}
	ctx, projections, err = (library.Engine{DB: e.DB, Connections: e.Connections}).PrepareOutputs(ctx, transferID, fileID, projections)
	if err != nil {
		return err
	}
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer deferutil.Rollback(ctx, tx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,714))`, transferID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,715))`, fileID); err != nil {
		return err
	}
	var liveState string
	if err = tx.QueryRow(ctx, `SELECT state FROM transfer_tasks WHERE id=$1 FOR UPDATE`, transferID).Scan(&liveState); err != nil {
		return err
	}
	if liveState != state || liveState == outputBlockedState || liveState == outputApprovedState {
		return errors.Join(jobs.ErrOperatorRequired, library.ErrOutputLayoutRequired)
	}
	var mediaID string
	mediaJSON, _ := json.Marshal(details.Media.ShowMetadata())
	assetJSON, _ := json.Marshal(details.Media)
	err = tx.QueryRow(ctx, `INSERT INTO media_items(id,media_type,tmdb_id,title,release_year,metadata)VALUES($1,$2,$3,$4,$5,$6)ON CONFLICT(media_type,tmdb_id)DO UPDATE SET metadata=excluded.metadata RETURNING id::text`, jobs.UUID(), details.Media.MediaType, details.Media.ID, details.Media.Title, details.Media.Year, mediaJSON).Scan(&mediaID)
	if err != nil {
		return err
	}
	var cloudFileID string
	if err = tx.QueryRow(ctx, `SELECT id::text FROM cloud_files WHERE storage_id=$1 AND full_path=$2`, connectionID, cloudPath).Scan(&cloudFileID); err != nil {
		return err
	}
	assetID := fileID
	if _, err = tx.Exec(ctx, `INSERT INTO media_assets(id,cloud_file_id,media_identity,edition)VALUES($1,$2,$3,$4)ON CONFLICT(id)DO NOTHING`, assetID, cloudFileID, assetJSON, path.Base(relative)); err != nil {
		return err
	}
	if err = library.PersistScrapeReport(ctx, tx, assetID, details.Media); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO media_asset_bindings(asset_id,media_id,download_file_id,season,episode,episodes)VALUES($1,$2,$3,$4,$5,$6)ON CONFLICT(asset_id)DO NOTHING`, assetID, mediaID, fileID, season, episode, jsonEpisodes(episodes)); err != nil {
		return err
	}
	grantService := playbackgrants.Service{DB: e.DB, Vault: e.Connections.Repository.Vault, PublicURL: e.Connections.Config.PublicURL}
	link, grantID, err := grantService.Prepare(ctx, tx, assetID, cloudPath)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(e.Connections.Config.STRMOutputDir, 0750); err != nil {
		return err
	}
	journal := library.Journal{DB: e.DB, Vault: e.Connections.Repository.Vault}
	intents, err := library.StageOutputProjections(ctx, journal, tx, e.Connections.Config.STRMOutputDir, assetID, details.Media, season, projections, []byte(link+"\n"), playbackgrants.SecretFreeKeyRef(grantID))
	if err != nil {
		if errors.Is(err, library.ErrOutputLayoutRequired) {
			if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
				return rollbackErr
			}
			return e.blockOutputImport(ctx, transferID, fileID)
		}
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE download_files SET media_state='artifacts_pending'WHERE id=$1`, fileID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE transfer_tasks SET state='artifacts_pending',updated_at=now()WHERE id=$1`, transferID); err != nil {
		return err
	}
	event, _ := json.Marshal(map[string]string{"taskId": taskID, "title": details.Media.Title, "assetId": assetID})
	if _, err = tx.Exec(ctx, `INSERT INTO outbox_events(id,event_type,aggregate_id,payload)VALUES($1,'media.artifacts_staged',$2,$3)`, jobs.UUID(), assetID, event); err != nil {
		return err
	}
	if err = dbutil.Commit(ctx, tx); err != nil {
		return err
	}
	for _, intent := range intents {
		if err = journal.Apply(ctx, intent); err != nil {
			return err
		}
	}
	outputs := []string{}
	for _, projection := range projections {
		outputs = append(outputs, projection.Path)
	}
	if err = library.CopySubtitlesOutputs(ctx, e.DB, e.Connections.Repository.Vault, details.LocalPath, relative, e.Connections.Config.STRMOutputDir, outputs, assetID); err != nil {
		return errors.New("STRM saved but subtitle projection conflict must be resolved")
	}
	final, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer deferutil.Rollback(ctx, final)
	var finalState string
	if err = final.QueryRow(ctx, `SELECT state FROM transfer_tasks WHERE id=$1 FOR UPDATE`, transferID).Scan(&finalState); err != nil {
		return err
	}
	if finalState == "needs_output_reconciliation" || finalState == "paused" || finalState == "cancelled" {
		return errors.Join(jobs.ErrOperatorRequired, library.ErrOutputLayoutRequired)
	}
	if finalState != "imported" {
		if _, err = final.Exec(ctx, `UPDATE download_files SET media_state='imported'WHERE id=$1`, fileID); err != nil {
			return err
		}
		if _, err = final.Exec(ctx, `UPDATE transfer_tasks SET state='imported',last_error='',updated_at=now()WHERE id=$1`, transferID); err != nil {
			return err
		}
		finalEvent, _ := json.Marshal(map[string]string{"taskId": taskID, "title": details.Media.Title, "assetId": assetID})
		if _, err = final.Exec(ctx, `INSERT INTO outbox_events(id,event_type,aggregate_id,payload)VALUES($1,'media.imported',$2,$3)`, jobs.UUID(), assetID, finalEvent); err != nil {
			return err
		}
	}
	if err = dbutil.Commit(ctx, final); err != nil {
		return err
	}
	return e.finishTask(ctx, taskID)
}
func (e Engine) finishTask(ctx context.Context, id string) error {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer deferutil.Rollback(ctx, tx)
	var state string
	var raw []byte
	if err = tx.QueryRow(ctx, `SELECT download_state,details FROM download_tasks WHERE id=$1 FOR UPDATE`, id).Scan(&state, &raw); err != nil {
		return err
	}
	if state == "imported" || state == "refresh_requested" {
		return nil
	}
	var total, pending, imported int
	if err = tx.QueryRow(ctx, `SELECT count(*),count(*)FILTER(WHERE media_state NOT IN ('imported','non_media','sample')),count(*)FILTER(WHERE media_state='imported')FROM download_files WHERE task_id=$1`, id).Scan(&total, &pending, &imported); err != nil {
		return err
	}
	if total == 0 || pending > 0 || imported == 0 {
		return nil
	}
	if state != "local_complete" && state != "strm_ready" {
		return nil
	}
	if state == "local_complete" {
		var details taskDetails
		if json.Unmarshal(raw, &details) != nil {
			return errors.New("invalid batch details")
		}
		if _, err = tx.Exec(ctx, `UPDATE download_tasks SET download_state='strm_ready',updated_at=now()WHERE id=$1`, id); err != nil {
			return err
		}
		event, _ := json.Marshal(map[string]string{"taskId": id, "title": details.Media.Title})
		if _, err = tx.Exec(ctx, `INSERT INTO outbox_events(id,event_type,aggregate_id,payload)VALUES($1,'media.batch_ready',$2,$3)`, jobs.UUID(), id, event); err != nil {
			return err
		}
	}
	payload, _ := json.Marshal(map[string]string{"taskId": id})
	if _, err = tx.Exec(ctx, `INSERT INTO jobs(id,kind,payload,idempotency_key)VALUES($1,'emby.refresh',$2,$3)ON CONFLICT(idempotency_key)WHERE idempotency_key IS NOT NULL DO NOTHING`, jobs.UUID(), payload, "emby.refresh:"+id); err != nil {
		return err
	}
	return dbutil.Commit(ctx, tx)
}
func jsonEpisodes(v []int) []byte { raw, _ := json.Marshal(v); return raw }

func coverage(name, kind string) (int, []int, error) {
	if kind == "movie" {
		return 0, nil, nil
	}
	season, episodes, ok := metadata.Episodes(name)
	if !ok {
		return 0, nil, errors.New("剧集未识别明确季集")
	}
	return season, episodes, nil
}
