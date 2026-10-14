package acquisition

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/library"
	"media-tool/internal/modules/metadata"
	"media-tool/internal/modules/playbackgrants"
)

const outputBlockedState = "needs_output_reconciliation"
const outputApprovedState = "output_resume_approved"

func (e Engine) blockOutputImport(ctx context.Context, id, fileID string) error {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,714))`, id); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE transfer_tasks SET state=$2,phase='output_layout',last_error=$3,updated_at=now() WHERE id=$1 AND state IN ('verified','artifacts_pending')`, id, outputBlockedState, library.ErrOutputLayoutRequired.Error())
	if err != nil {
		return err
	}
	if tag.RowsAffected() > 0 {
		if _, err = tx.Exec(ctx, `UPDATE download_files SET media_state=$2 WHERE id=$1`, fileID, outputBlockedState); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('worker','transfer.output_layout_blocked',jsonb_build_object('transferId',$1::text))`, id); err != nil {
			return err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	return errors.Join(jobs.ErrOperatorRequired, library.ErrOutputLayoutRequired)
}

type OutputResumePreview struct {
	TransferID            string   `json:"transferId"`
	AssetID               string   `json:"assetId"`
	ControlRevision       int64    `json:"controlRevision"`
	AssetRevision         int64    `json:"assetRevision"`
	State                 string   `json:"state"`
	ExistingPaths         []string `json:"existingPaths"`
	ExpectedPaths         []string `json:"expectedPaths"`
	NeedsMigration        bool     `json:"needsMigration"`
	NeedsArtifactRecovery bool     `json:"needsArtifactRecovery"`
	Ready                 bool     `json:"ready"`
	PlanDigest            string   `json:"planDigest"`
	Message               string   `json:"message"`
	NoUploadRecreation    bool     `json:"noUploadRecreation"`
	SourceFilesPreserved  bool     `json:"sourceFilesPreserved"`
}

type recoverySnapshot struct {
	Preview                                           OutputResumePreview
	LocalRoot, Relative, TaskID, Phase, SubtitleProof string
	Item                                              metadata.Item
	Season                                            *int
	Episodes                                          []int
	Projection                                        []library.OutputProjection
}

func (e Engine) outputSnapshot(ctx context.Context, tx pgx.Tx, id string, allowActive ...bool) (recoverySnapshot, error) {
	var s recoverySnapshot
	s.Preview = OutputResumePreview{TransferID: id, NoUploadRecreation: true, SourceFilesPreserved: true, ExistingPaths: []string{}, ExpectedPaths: []string{}}
	var detailsRaw, itemRaw, episodeRaw []byte
	var fileState, artifactState string
	var verifiedSize, fileSize int64
	var cloudUpdated time.Time
	var cloudPath, destination, connection, cloudConnection string
	err := tx.QueryRow(ctx, `SELECT t.state,t.phase,t.control_revision,d.id::text,d.details,f.relative_path,f.size_bytes,a.id::text,a.media_identity,a.revision,a.artifacts_state,c.full_path,c.size_bytes,c.updated_at,c.state,b.season,b.episodes,t.destination,t.integration_id::text,c.storage_id::text FROM transfer_tasks t JOIN download_files f ON f.id=t.download_file_id JOIN download_tasks d ON d.id=f.task_id JOIN media_assets a ON a.id=f.id JOIN media_asset_bindings b ON b.asset_id=a.id AND b.download_file_id=f.id JOIN cloud_files c ON c.id=a.cloud_file_id WHERE t.id=$1`, id).Scan(&s.Preview.State, &s.Phase, &s.Preview.ControlRevision, &s.TaskID, &detailsRaw, &s.Relative, &fileSize, &s.Preview.AssetID, &itemRaw, &s.Preview.AssetRevision, &artifactState, &cloudPath, &verifiedSize, &cloudUpdated, &fileState, &s.Season, &episodeRaw, &destination, &connection, &cloudConnection)
	if err != nil {
		return s, errors.New("传输或已确认资产绑定不存在")
	}
	var details taskDetails
	if json.Unmarshal(detailsRaw, &details) != nil || json.Unmarshal(itemRaw, &s.Item) != nil || json.Unmarshal(episodeRaw, &s.Episodes) != nil {
		return s, errors.New("恢复元数据无效")
	}
	s.LocalRoot = details.LocalPath
	if s.Item.ID < 1 || s.Item.MediaType != "movie" && s.Item.MediaType != "series" {
		return s, errors.New("已确认资产身份无效")
	}
	if s.Item.MediaType == "series" {
		if s.Season == nil {
			return s, errors.New("已确认资产缺少季集")
		}
		normalized, err := metadata.NormalizeEpisodes(*s.Season, s.Episodes)
		if err != nil {
			return s, err
		}
		s.Episodes = normalized
	}
	s.Projection, err = e.TransferProjections(ctx, id, cloudPath, s.Preview.AssetID, s.Item, s.Season, s.Episodes, true)
	if err != nil {
		return s, err
	}
	desired := map[string]bool{}
	for _, p := range s.Projection {
		desired[p.Path] = true
		s.Preview.ExpectedPaths = append(s.Preview.ExpectedPaths, p.Path)
	}
	rows, err := tx.Query(ctx, `SELECT s.output_path,s.key_reference,m.content_digest,m.owner_scope,m.asset_id::text FROM strm_entries s LEFT JOIN managed_media_artifacts m ON m.relative_path=s.output_path AND m.artifact_kind='strm' WHERE s.asset_id=$1 ORDER BY s.output_path`, s.Preview.AssetID)
	if err != nil {
		return s, err
	}
	type proofOutput struct{ Path, KeyReference, Digest string }
	proofs := []proofOutput{}
	type output struct {
		path, key    string
		digest       []byte
		scope, owner *string
	}
	outputs := []output{}
	for rows.Next() {
		var o output
		if err = rows.Scan(&o.path, &o.key, &o.digest, &o.scope, &o.owner); err != nil {
			rows.Close()
			return s, err
		}
		outputs = append(outputs, o)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return s, err
	}
	fs, err := os.OpenRoot(e.Connections.Config.STRMOutputDir)
	if err != nil {
		return s, err
	}
	defer fs.Close()

	// Include every active asset-owned NFO/subtitle, not only the playback URL.
	extra, err := tx.Query(ctx, `SELECT relative_path,content_digest,owner_scope FROM managed_media_artifacts WHERE asset_id=$1 AND owner_scope=$2 AND artifact_kind<>'strm' ORDER BY relative_path`, s.Preview.AssetID, "asset:"+s.Preview.AssetID)
	if err != nil {
		return s, err
	}
	type artifactProof struct {
		path   string
		digest []byte
		scope  string
	}
	artifacts := []artifactProof{}
	for extra.Next() {
		var p artifactProof
		if err = extra.Scan(&p.path, &p.digest, &p.scope); err != nil {
			extra.Close()
			return s, err
		}
		artifacts = append(artifacts, p)
	}
	err = extra.Err()
	extra.Close()
	if err != nil {
		return s, err
	}
	total := int64(0)
	for _, p := range artifacts {
		if len(outputs)+len(artifacts) > 500 {
			s.Preview.NeedsArtifactRecovery = true
			break
		}
		info, statErr := fs.Lstat(p.path)
		if statErr != nil || !info.Mode().IsRegular() || info.Size() > 20<<20 {
			s.Preview.NeedsArtifactRecovery = true
			continue
		}
		total += info.Size()
		if total > 64<<20 {
			s.Preview.NeedsArtifactRecovery = true
			break
		}
		f, readErr := fs.Open(p.path)
		if readErr != nil {
			s.Preview.NeedsArtifactRecovery = true
			continue
		}
		actual, readErr := f.Stat()
		if readErr != nil || !os.SameFile(info, actual) {
			f.Close()
			s.Preview.NeedsArtifactRecovery = true
			continue
		}
		raw, readErr := io.ReadAll(io.LimitReader(f, (20<<20)+1))
		f.Close()
		sum := sha256.Sum256(raw)
		if readErr != nil || !reflect.DeepEqual(sum[:], p.digest) {
			s.Preview.NeedsArtifactRecovery = true
			continue
		}
		proofs = append(proofs, proofOutput{p.path, "", hex.EncodeToString(p.digest)})
	}
	var common []byte
	for _, o := range outputs {
		s.Preview.ExistingPaths = append(s.Preview.ExistingPaths, o.path)
		if !desired[o.path] {
			s.Preview.NeedsMigration = true
		}
		if o.scope == nil || o.owner == nil || *o.scope != "asset:"+s.Preview.AssetID || *o.owner != s.Preview.AssetID {
			s.Preview.NeedsArtifactRecovery = true
			continue
		}
		info, statErr := fs.Lstat(o.path)
		if statErr != nil || !info.Mode().IsRegular() || info.Size() > 8192 {
			s.Preview.NeedsArtifactRecovery = true
			continue
		}
		f, readErr := fs.Open(o.path)
		if readErr != nil {
			s.Preview.NeedsArtifactRecovery = true
			continue
		}
		raw, readErr := func() ([]byte, error) {
			defer f.Close()
			actual, err := f.Stat()
			if err != nil || !os.SameFile(info, actual) {
				return nil, errors.New("文件身份变化")
			}
			data, err := io.ReadAll(io.LimitReader(f, 8193))
			if err != nil || int64(len(data)) != info.Size() {
				return nil, errors.New("文件读取未完成")
			}
			return data, nil
		}()
		sum := sha256.Sum256(raw)
		if readErr != nil || !reflect.DeepEqual(sum[:], o.digest) {
			s.Preview.NeedsArtifactRecovery = true
			continue
		}
		if common == nil {
			common = raw
		} else if !reflect.DeepEqual(common, raw) {
			s.Preview.NeedsArtifactRecovery = true
		}
		proofs = append(proofs, proofOutput{o.path, o.key, hex.EncodeToString(o.digest)})
	}
	ownedNFO := map[string]bool{}
	for _, p := range artifacts {
		ownedNFO[p.path] = true
	}
	for _, p := range s.Preview.ExistingPaths {
		if !ownedNFO[strings.TrimSuffix(p, filepath.Ext(p))+".nfo"] {
			s.Preview.NeedsArtifactRecovery = true
		}
	}
	existingProjection, layoutErr := library.ExistingOutputProjections(s.Preview.ExistingPaths, s.Item.MediaType, s.Season, s.Episodes)
	if layoutErr != nil {
		s.Preview.NeedsMigration = true
	} else {
		s.Preview.NeedsMigration = false
		s.Projection = existingProjection
		s.Preview.ExpectedPaths = append([]string(nil), s.Preview.ExistingPaths...)
	}
	var pending bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM media_artifact_journal WHERE asset_id=$1 AND state<>'applied')`, s.Preview.AssetID).Scan(&pending); err != nil {
		return s, err
	}
	s.Preview.NeedsArtifactRecovery = s.Preview.NeedsArtifactRecovery || pending || artifactState != "ready"
	switch {
	case fileState != "present" || verifiedSize != fileSize || cloudPath != destination || connection != cloudConnection:
		s.Preview.Message = "云端已校验文件状态或大小变化；停止恢复"
	case s.Preview.NeedsArtifactRecovery:
		s.Preview.Message = "先在云盘索引恢复已有托管输出，解决手工修改/缺失/待恢复，不覆盖文件"
	case s.Preview.NeedsMigration:
		s.Preview.Message = "在云盘索引查看该资产，明确预览并确认输出迁移；不自动混用新旧布局"
	case s.Preview.State != outputBlockedState && s.Preview.State != outputApprovedState && !(len(allowActive) > 0 && allowActive[0] && (s.Preview.State == "verified" || s.Preview.State == "artifacts_pending")):
		s.Preview.Message = "当前阶段不需要布局恢复"
	case s.Preview.State == outputApprovedState:
		s.Preview.Ready = true
		s.Preview.Message = "已有明确恢复批准，请等待/核对入库任务状态，不创建第二个上传或恢复请求"
	default:
		s.Preview.Ready = true
		s.Preview.Message = "输出与已确认布局一致，可明确恢复入库；不重建上传、不改STRM正文"
	}
	if s.Preview.Ready {
		grants := playbackgrants.Service{DB: e.DB, Vault: e.Connections.Repository.Vault, PublicURL: e.Connections.Config.PublicURL}
		ref, err := grants.VerifyManagedURL(ctx, strings.TrimSpace(string(common)))
		if err != nil || ref.AssetID != s.Preview.AssetID {
			s.Preview.Ready = false
			s.Preview.NeedsArtifactRecovery = true
			s.Preview.Message = "托管STRM的文件级授权无效/撤销，请明确核对或轮换文件Key，不自动重写"
			return s, nil
		}
		outputs := []string{}
		for _, p := range s.Projection {
			outputs = append(outputs, p.Path)
		}
		sourceFS, sourceErr := os.OpenRoot(s.LocalRoot)
		if sourceErr != nil {
			s.Preview.Ready = false
			s.Preview.Message = "共享做种源目录缺失，请先恢复映射，不重新上传"
			return s, nil
		}
		sourceInfo, sourceErr := sourceFS.Lstat(filepath.FromSlash(s.Relative))
		sourceFS.Close()
		if sourceErr != nil || !sourceInfo.Mode().IsRegular() || sourceInfo.Size() != fileSize {
			s.Preview.Ready = false
			s.Preview.Message = "原做种源文件缺失/大小或身份变化，请先核对，不猜测恢复"
			return s, nil
		}
		sourceProof, sourceErr := library.SubtitleOutputsProof(s.LocalRoot, s.Relative, outputs)
		if sourceErr != nil {
			s.Preview.Ready = false
			s.Preview.Message = "源字幕清单不完整/冲突，请先核对，再明确恢复入库"
			return s, nil
		}
		s.SubtitleProof = sourceProof
		raw, err := json.Marshal(struct {
			Asset                                                        string
			Revision                                                     int64
			Item                                                         metadata.Item
			Season                                                       *int
			Episodes                                                     []int
			CloudPath                                                    string
			Size                                                         int64
			Updated                                                      time.Time
			Outputs                                                      []proofOutput
			LocalRoot, Relative, TaskID, OutputRoot, SubtitleSourceProof string
		}{s.Preview.AssetID, s.Preview.AssetRevision, s.Item, s.Season, s.Episodes, cloudPath, fileSize, cloudUpdated, proofs, s.LocalRoot, s.Relative, s.TaskID, filepath.Clean(e.Connections.Config.STRMOutputDir), sourceProof})
		if err != nil {
			return s, err
		}
		sum := sha256.Sum256(raw)
		s.Preview.PlanDigest = hex.EncodeToString(sum[:])
	}
	return s, nil
}
func (e Engine) PreviewOutputResume(ctx context.Context, id string) (OutputResumePreview, error) {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return OutputResumePreview{}, err
	}
	defer tx.Rollback(ctx)
	s, err := e.outputSnapshot(ctx, tx, id)
	return s.Preview, err
}
func (e Engine) ResumeOutputImport(ctx context.Context, id string, revision int64, digest string) error {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,714))`, id); err != nil {
		return err
	}
	var state string
	var current int64
	if err = tx.QueryRow(ctx, `SELECT state,control_revision FROM transfer_tasks WHERE id=$1 FOR UPDATE`, id).Scan(&state, &current); err != nil {
		return err
	}
	// Safe replay of the same approval does not queue another operation.
	if state == outputApprovedState {
		var phase string
		tx.QueryRow(ctx, `SELECT phase FROM transfer_tasks WHERE id=$1`, id).Scan(&phase)
		if current == revision+1 && phase == "output_resume:"+digest {
			return nil
		}
		return errors.New("已有不同恢复批准，请核对当前状态")
	}
	if state != outputBlockedState || current != revision {
		return errors.New("传输阶段/修订变化，请重新核对")
	}
	var assetID string
	if err = tx.QueryRow(ctx, `SELECT download_file_id::text FROM transfer_tasks WHERE id=$1`, id).Scan(&assetID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,715))`, assetID); err != nil {
		return err
	}
	s, err := e.outputSnapshot(ctx, tx, id)
	if err != nil {
		return err
	}
	if !s.Preview.Ready || digest == "" || s.Preview.PlanDigest != digest {
		return errors.New("输出恢复预览不完整或已变化，请重新核对布局/文件")
	}
	if _, err = tx.Exec(ctx, `UPDATE transfer_tasks SET state=$2,phase=$3,control_revision=control_revision+1,last_error='',updated_at=now()WHERE id=$1`, id, outputApprovedState, "output_resume:"+digest); err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{"transferId": id, "controlRevision": revision + 1, "planDigest": digest})
	if _, err = tx.Exec(ctx, `INSERT INTO jobs(id,kind,queue,payload,idempotency_key,max_attempts)VALUES($1,'transfer.import.resume','transfer',$2,$3,5) ON CONFLICT(idempotency_key)WHERE idempotency_key IS NOT NULL DO NOTHING`, jobs.UUID(), payload, "transfer.import.resume:"+id+":"+strconv.FormatInt(revision+1, 10)+":"+digest); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','transfer.output_resume_approved',jsonb_build_object('transferId',$1::text,'assetId',$2::text))`, id, s.Preview.AssetID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (e Engine) RunOutputImport(ctx context.Context, job jobs.Job) error {
	var p struct {
		ID       string `json:"transferId"`
		Revision int64  `json:"controlRevision"`
		Digest   string `json:"planDigest"`
	}
	if json.Unmarshal(job.Payload, &p) != nil {
		return errors.New("invalid approved import payload")
	}
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,714))`, p.ID); err != nil {
		return err
	}
	var state, phase string
	var revision int64
	if err = tx.QueryRow(ctx, `SELECT state,phase,control_revision FROM transfer_tasks WHERE id=$1 FOR UPDATE`, p.ID).Scan(&state, &phase, &revision); err != nil {
		return err
	}
	if state == "imported" {
		var taskID string
		if err = tx.QueryRow(ctx, `SELECT f.task_id::text FROM transfer_tasks t JOIN download_files f ON f.id=t.download_file_id WHERE t.id=$1`, p.ID).Scan(&taskID); err != nil {
			return err
		}
		if err = tx.Rollback(ctx); err != nil {
			return err
		}
		return e.finishTask(ctx, taskID)
	}
	if state != outputApprovedState || revision != p.Revision || phase != "output_resume:"+p.Digest {
		return errors.Join(jobs.ErrOperatorRequired, library.ErrOutputLayoutRequired)
	}
	var assetID string
	if err = tx.QueryRow(ctx, `SELECT download_file_id::text FROM transfer_tasks WHERE id=$1`, p.ID).Scan(&assetID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,715))`, assetID); err != nil {
		return err
	}
	s, err := e.outputSnapshot(ctx, tx, p.ID)
	if err != nil {
		return err
	}
	if !s.Preview.Ready || s.Preview.PlanDigest != p.Digest {
		if _, err = tx.Exec(ctx, `UPDATE transfer_tasks SET state=$2,phase='output_layout',last_error=$3 WHERE id=$1`, p.ID, outputBlockedState, library.ErrOutputLayoutRequired.Error()); err != nil {
			return err
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
		return errors.Join(jobs.ErrOperatorRequired, library.ErrOutputLayoutRequired)
	}
	outputs := []string{}
	for _, o := range s.Projection {
		outputs = append(outputs, o.Path)
	}
	// Existing STRM/NFO/metadata binding remain exactly as explicitly migrated.
	// Only missing/conflicting subtitle manifests use the existing owned journal.
	if err = library.CopySubtitlesOutputsVerified(ctx, e.DB, e.Connections.Repository.Vault, s.LocalRoot, s.Relative, e.Connections.Config.STRMOutputDir, outputs, s.Preview.AssetID, s.SubtitleProof); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE transfer_tasks SET state='imported',last_error='',updated_at=now()WHERE id=$1`, p.ID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE download_files SET media_state='imported'WHERE id=$1`, s.Preview.AssetID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('worker','transfer.output_resume_completed',jsonb_build_object('transferId',$1::text,'assetId',$2::text))`, p.ID, s.Preview.AssetID); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	return e.finishTask(ctx, s.TaskID)
}

// An already committed import manifest is authoritative. Normal retry replays
// its old encrypted intents and current binding, never refetches templates or
// regenerates names. Geometry conflicts still require explicit migration.
func (e Engine) resumePersistedImport(ctx context.Context, id, fileID string) error {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,714))`, id); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,715))`, fileID); err != nil {
		return err
	}
	var state string
	if err = tx.QueryRow(ctx, `SELECT state FROM transfer_tasks WHERE id=$1 FOR UPDATE`, id).Scan(&state); err != nil {
		return err
	}
	if state != "verified" && state != "artifacts_pending" {
		return errors.Join(jobs.ErrOperatorRequired, library.ErrOutputLayoutRequired)
	}
	s, err := e.outputSnapshot(ctx, tx, id, true)
	if err != nil {
		return err
	}
	if s.Preview.NeedsMigration {
		tx.Rollback(ctx)
		return e.blockOutputImport(ctx, id, fileID)
	}
	// Apply only exact original pending intents, not a new provider schema.
	rows, err := tx.Query(ctx, `SELECT id::text,root_path FROM media_artifact_journal WHERE asset_id=$1 AND state<>'applied' ORDER BY CASE WHEN operation='write' THEN 0 ELSE 1 END,created_at,id`, fileID)
	if err != nil {
		return err
	}
	intents := []string{}
	root, err := filepath.Abs(e.Connections.Config.STRMOutputDir)
	if err != nil {
		rows.Close()
		return err
	}
	for rows.Next() {
		var intent, recordRoot string
		if err = rows.Scan(&intent, &recordRoot); err != nil {
			rows.Close()
			return err
		}
		if recordRoot != root {
			rows.Close()
			return errors.New("原恢复意图输出根目录不同，请人工核对，不在旧根目录自动写入")
		}
		intents = append(intents, intent)
		if len(intents) > 500 {
			rows.Close()
			return errors.New("原恢复清单超过边界")
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	j := library.Journal{DB: e.DB, Vault: e.Connections.Repository.Vault}
	for _, intent := range intents {
		if err = j.Apply(ctx, intent); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE media_assets SET artifacts_state='ready'WHERE id=$1 AND NOT EXISTS(SELECT 1 FROM media_artifact_journal WHERE asset_id=$1 AND state<>'applied')`, fileID); err != nil {
		return err
	}
	s, err = e.outputSnapshot(ctx, tx, id, true)
	if err != nil {
		return err
	}
	if !s.Preview.Ready {
		return errors.New("原输出、授权、做种源或字幕仍有冲突，请先核对恢复，不重新生成元数据")
	}
	outputs := []string{}
	for _, p := range s.Projection {
		outputs = append(outputs, p.Path)
	}
	if err = library.CopySubtitlesOutputsVerified(ctx, e.DB, e.Connections.Repository.Vault, s.LocalRoot, s.Relative, e.Connections.Config.STRMOutputDir, outputs, fileID, s.SubtitleProof); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE transfer_tasks SET state='imported',last_error='',updated_at=now()WHERE id=$1`, id); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE download_files SET media_state='imported'WHERE id=$1`, fileID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('worker','transfer.persisted_output_resumed',jsonb_build_object('transferId',$1::text,'assetId',$2::text))`, id, fileID); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	return e.finishTask(ctx, s.TaskID)
}
