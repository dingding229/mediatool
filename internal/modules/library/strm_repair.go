package library

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/playbackgrants"
	"media-tool/internal/modules/strm"
	deferutil "media-tool/internal/platform/cleanup"
	dbutil "media-tool/internal/platform/database"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

type STRMRepairItem struct {
	Ordinal   int    `json:"ordinal"`
	AssetID   string `json:"assetId"`
	Revision  int64  `json:"revision"`
	Path      string `json:"path"`
	Digest    string `json:"digest"`
	State     string `json:"state"`
	Message   string `json:"message"`
	JournalID string `json:"journalId,omitempty"`
}
type STRMRepairBatch struct {
	ID        string           `json:"id"`
	State     string           `json:"state"`
	Digest    string           `json:"digest"`
	ExpiresAt time.Time        `json:"expiresAt"`
	UpdatedAt time.Time        `json:"updatedAt"`
	Items     []STRMRepairItem `json:"items"`
}

// Prefer exact original encrypted body. Old erased journal rows may be restored
// only if an existing file grant rebuild has precisely the owned SHA256.
func (e Engine) repairBody(ctx context.Context, tx pgx.Tx, asset, relative string, expected []byte) ([]byte, error) {
	var id string
	var encrypted []byte
	err := tx.QueryRow(ctx, `SELECT id::text,content_ciphertext FROM media_artifact_journal WHERE asset_id=$1 AND relative_path=$2 AND artifact_kind='strm' AND operation='write' AND content_digest=$3 AND content_ciphertext IS NOT NULL ORDER BY created_at DESC,id DESC LIMIT 1`, asset, relative, expected).Scan(&id, &encrypted)
	if err == nil {
		values, err := e.Connections.Repository.Vault.Open(id, encrypted)
		if err != nil {
			return nil, err
		}
		raw, err := base64.StdEncoding.DecodeString(values["content"])
		sum := sha256.Sum256(raw)
		if err != nil || !bytes.Equal(sum[:], expected) {
			return nil, ErrArtifactConflict
		}
		if err = verifyRepairBody(ctx, tx, asset, raw); err != nil {
			return nil, err
		}
		return raw, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	service := playbackgrants.Service{DB: e.DB, Vault: e.Connections.Repository.Vault, PublicURL: e.Connections.Config.PublicURL}
	raw, err := service.ExistingBody(ctx, tx, asset)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(raw)
	if !bytes.Equal(sum[:], expected) {
		return nil, ErrArtifactConflict
	}
	return raw, nil
}
func verifyRepairBody(ctx context.Context, tx pgx.Tx, asset string, raw []byte) error {
	if len(raw) > 16384 {
		return ErrArtifactConflict
	}
	u, err := url.Parse(strings.TrimSpace(string(raw)))
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.Path != strm.Endpoint {
		return ErrArtifactConflict
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil || len(q["apiKey"]) != 1 || len(q["assetId"]) != 1 || q.Get("assetId") != asset {
		return ErrArtifactConflict
	}
	sum := sha256.Sum256([]byte(q.Get("apiKey")))
	var expected []byte
	if err = tx.QueryRow(ctx, `SELECT token_digest FROM playback_grants WHERE asset_id=$1 AND revoked_at IS NULL FOR SHARE`, asset).Scan(&expected); err != nil || !bytes.Equal(sum[:], expected) {
		return ErrArtifactConflict
	}
	return nil
}
func (e Engine) inspectSTRM(ctx context.Context, tx pgx.Tx, item STRMRepairItem) (STRMRepairItem, []byte, error) {
	item.State = "blocked"
	item.Message = "文件归属、在库状态、原文或修订证据无法确认，不自动覆盖"
	if !validRelativeArtifact(item.Path) || path.Ext(item.Path) != ".strm" {
		return item, nil, nil
	}
	var revision int64
	var expected []byte
	var owner, state string
	var revoked bool
	err := tx.QueryRow(ctx, `SELECT a.revision,m.content_digest,m.owner_scope,f.state,g.revoked_at IS NOT NULL FROM media_assets a JOIN cloud_files f ON f.id=a.cloud_file_id JOIN strm_entries s ON s.asset_id=a.id JOIN managed_media_artifacts m ON m.asset_id=a.id AND m.relative_path=s.output_path JOIN playback_grants g ON g.asset_id=a.id WHERE a.id=$1 AND s.output_path=$2 AND m.artifact_kind='strm'`, item.AssetID, item.Path).Scan(&revision, &expected, &owner, &state, &revoked)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return item, nil, nil
		}
		return item, nil, err
	}
	if item.Revision > 0 && item.Revision != revision || owner != "asset:"+item.AssetID || state != "present" || revoked {
		return item, nil, nil
	}
	item.Revision = revision
	item.Digest = hex.EncodeToString(expected)
	var pending bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM media_artifact_journal WHERE asset_id=$1 AND state IN('pending','conflict'))`, item.AssetID).Scan(&pending); err != nil {
		return item, nil, err
	}
	if pending {
		return item, nil, nil
	}
	fs, err := os.OpenRoot(e.Connections.Config.STRMOutputDir)
	if err != nil {
		return item, nil, err
	}
	defer deferutil.Close(fs, "Read/descriptor teardown only; primary operations handle their own I/O result")
	raw, err := readRegular(fs, item.Path)
	if err == nil {
		sum := sha256.Sum256(raw)
		if bytes.Equal(sum[:], expected) {
			item.State = "ready"
			item.Message = "STRM原内容完整，无需写入"
		} else {
			item.Message = "当前文件已被修改，保留原文件并跳过"
		}
		return item, nil, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		item.Message = "路径不是安全常规文件，不覆盖"
		return item, nil, nil
	}
	// Lstat also rejects dangling symlinks; journal writes can never adopt one.
	if _, err = fs.Lstat(item.Path); err == nil || !errors.Is(err, os.ErrNotExist) {
		return item, nil, nil
	}
	raw, err = e.repairBody(ctx, tx, item.AssetID, item.Path, expected)
	if err != nil {
		item.Message = "缺失STRM但无法取得同摘要原文，需人工核对"
		return item, nil, nil
	}
	item.State = "missing"
	item.Message = "可恢复缺失STRM，原正文与文件Key保持不变"
	return item, raw, nil
}
func (e Engine) PreviewSTRMRepair(ctx context.Context, assets []string) (STRMRepairBatch, error) {
	out := STRMRepairBatch{ID: jobs.UUID(), State: "preview", ExpiresAt: time.Now().Add(10 * time.Minute), Items: []STRMRepairItem{}}
	if len(assets) < 1 || len(assets) > 100 {
		return out, errors.New("每批选择1–100个托管资产")
	}
	seen := map[string]bool{}
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer deferutil.Rollback(ctx, tx)
	for _, asset := range assets {
		if !ValidArchiveID(asset) || seen[asset] {
			return out, errors.New("资产ID无效或重复")
		}
		seen[asset] = true
		rows, err := tx.Query(ctx, `SELECT output_path FROM strm_entries WHERE asset_id=$1 ORDER BY output_path LIMIT 101`, asset)
		if err != nil {
			return out, err
		}
		paths := []string{}
		for rows.Next() {
			var p string
			if err = rows.Scan(&p); err != nil {
				rows.Close()
				return out, err
			}
			paths = append(paths, p)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return out, err
		}
		if len(paths) == 0 {
			out.Items = append(out.Items, STRMRepairItem{Ordinal: len(out.Items), AssetID: asset, State: "blocked", Message: "此资产没有托管STRM输出"})
		}
		for _, p := range paths {
			if len(out.Items) >= 100 {
				return out, errors.New("每批最多100个STRM文件，请缩小选择范围")
			}
			item, _, err := e.inspectSTRM(ctx, tx, STRMRepairItem{Ordinal: len(out.Items), AssetID: asset, Path: p})
			if err != nil {
				return out, err
			}
			out.Items = append(out.Items, item)
		}
	}
	raw, _ := json.Marshal(out.Items)
	sum := sha256.Sum256(raw)
	out.Digest = hex.EncodeToString(sum[:])
	if _, err = tx.Exec(ctx, `INSERT INTO strm_repair_batches(id,digest,expires_at)VALUES($1,$2,$3)`, out.ID, out.Digest, out.ExpiresAt); err != nil {
		return out, err
	}
	for _, item := range out.Items {
		digest, _ := hex.DecodeString(item.Digest)
		if _, err = tx.Exec(ctx, `INSERT INTO strm_repair_items(batch_id,ordinal,asset_id,asset_revision,relative_path,content_digest,state,message)VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, out.ID, item.Ordinal, item.AssetID, item.Revision, item.Path, digest, item.State, item.Message); err != nil {
			return out, err
		}
	}
	if err = dbutil.Commit(ctx, tx); err != nil {
		return out, err
	}
	return e.STRMRepairBatch(ctx, out.ID)
}
func (e Engine) STRMRepairBatch(ctx context.Context, id string) (STRMRepairBatch, error) {
	out := STRMRepairBatch{Items: []STRMRepairItem{}}
	if !ValidArchiveID(id) {
		return out, errors.New("批次ID无效")
	}
	if err := e.DB.QueryRow(ctx, `SELECT id::text,state,digest,expires_at,updated_at FROM strm_repair_batches WHERE id=$1`, id).Scan(&out.ID, &out.State, &out.Digest, &out.ExpiresAt, &out.UpdatedAt); err != nil {
		return out, err
	}
	if out.State == "running" {
		var failed bool
		if err := e.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM strm_repair_batches b JOIN jobs j ON j.id=b.job_id WHERE b.id=$1 AND j.state='failed')`, id).Scan(&failed); err != nil {
			return out, err
		}
		if failed {
			out.State = "needs_reconcile"
		}
	}
	rows, err := e.DB.Query(ctx, `SELECT ordinal,asset_id::text,asset_revision,relative_path,coalesce(encode(content_digest,'hex'),''),state,message,coalesce(journal_id::text,'') FROM strm_repair_items WHERE batch_id=$1 ORDER BY ordinal`, id)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var i STRMRepairItem
		if err = rows.Scan(&i.Ordinal, &i.AssetID, &i.Revision, &i.Path, &i.Digest, &i.State, &i.Message, &i.JournalID); err != nil {
			return out, err
		}
		out.Items = append(out.Items, i)
	}
	return out, rows.Err()
}
func (e Engine) ApproveSTRMRepair(ctx context.Context, id, digest string, confirm bool) (STRMRepairBatch, error) {
	if !ValidArchiveID(id) || len(digest) != 64 || !confirm {
		return STRMRepairBatch{}, errors.New("请先预览并明确批准该批次")
	}
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return STRMRepairBatch{}, err
	}
	defer deferutil.Rollback(ctx, tx)
	var state, stored string
	var expires time.Time
	if err = tx.QueryRow(ctx, `SELECT state,digest,expires_at FROM strm_repair_batches WHERE id=$1 FOR UPDATE`, id).Scan(&state, &stored, &expires); err != nil {
		return STRMRepairBatch{}, err
	}
	if stored != digest {
		return STRMRepairBatch{}, ErrArtifactConflict
	}
	if state != "preview" {
		return e.STRMRepairBatch(ctx, id)
	}
	if !expires.After(time.Now()) {
		return STRMRepairBatch{}, errors.New("预览已过期，请重新检查")
	}
	jobID := jobs.UUID()
	if _, err = tx.Exec(ctx, `INSERT INTO jobs(id,kind,queue,payload,idempotency_key)VALUES($1,'library.strm.repair','library',jsonb_build_object('batchId',$2::text),$3)`, jobID, id, "strm-repair:"+id); err != nil {
		return STRMRepairBatch{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE strm_repair_batches SET state='running',job_id=$2,updated_at=now()WHERE id=$1`, id, jobID); err != nil {
		return STRMRepairBatch{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','strm.batch_repair_approved',jsonb_build_object('batchId',$1::text,'digest',$2::text))`, id, digest); err != nil {
		return STRMRepairBatch{}, err
	}
	if err = dbutil.Commit(ctx, tx); err != nil {
		return STRMRepairBatch{}, err
	}
	return e.STRMRepairBatch(ctx, id)
}
func (e Engine) ProcessSTRMRepair(ctx context.Context, job jobs.Job) error {
	var in struct {
		ID string `json:"batchId"`
	}
	if json.Unmarshal(job.Payload, &in) != nil || !ValidArchiveID(in.ID) {
		return errors.New("无效STRM修复任务")
	}
	conn, err := e.DB.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err = conn.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended($1,746))`, in.ID); err != nil {
		return err
	}
	defer func() {
		end, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if _, err = conn.Exec(end, `SELECT pg_advisory_unlock(hashtextextended($1,746))`, in.ID); err != nil {
			_ = conn.Conn().Close(end)
		}
	}()
	batch, err := e.STRMRepairBatch(ctx, in.ID)
	if err != nil {
		return err
	}
	if batch.State != "running" {
		return nil
	}
	for _, i := range batch.Items {
		if i.State != "missing" && i.State != "staged" {
			continue
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		journal := Journal{DB: e.DB, Vault: e.Connections.Repository.Vault}
		jid := i.JournalID
		if jid == "" {
			tx, err := e.DB.Begin(ctx)
			if err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,715))`, i.AssetID); err != nil {
				deferutil.Rollback(ctx, tx)
				return err
			}
			checked, raw, err := e.inspectSTRM(ctx, tx, i)
			if err == nil && checked.Digest != i.Digest {
				checked.State = "blocked"
				checked.Message = "归属摘要已变化，请重新预览"
			}
			if err != nil {
				deferutil.Rollback(ctx, tx)
				return err
			}
			if checked.State != "missing" {
				_, err = tx.Exec(ctx, `UPDATE strm_repair_items SET state=$3,message=$4 WHERE batch_id=$1 AND ordinal=$2`, in.ID, i.Ordinal, checked.State, checked.Message)
				if err == nil {
					err = dbutil.Commit(ctx, tx)
				} else {
					deferutil.Rollback(ctx, tx)
				}
				if err != nil {
					return err
				}
				continue
			}
			jid, err = journal.Stage(ctx, tx, e.Connections.Config.STRMOutputDir, i.Path, i.AssetID, "asset:"+i.AssetID, "strm", raw)
			if err == nil {
				cipher, sealingErr := e.Connections.Repository.Vault.Seal(jid, map[string]string{"content": base64.StdEncoding.EncodeToString(raw), "restoreOnly": "true"})
				if sealingErr != nil {
					err = sealingErr
				} else {
					_, err = tx.Exec(ctx, `UPDATE media_artifact_journal SET content_ciphertext=$2 WHERE id=$1`, jid, cipher)
				}
			}

			if err == nil {
				_, err = tx.Exec(ctx, `UPDATE strm_repair_items SET state='staged',journal_id=$3 WHERE batch_id=$1 AND ordinal=$2`, in.ID, i.Ordinal, jid)
			}
			if err == nil {
				err = dbutil.Commit(ctx, tx)
			} else {
				deferutil.Rollback(ctx, tx)
			}
			if err != nil {
				return err
			}
		}
		err = journal.Apply(ctx, jid)
		state, message := "repaired", "已恢复原始STRM正文，文件Key未改变"
		if errors.Is(err, ErrArtifactConflict) {
			state = "blocked"
			message = "执行前文件已变化，未覆盖"
		} else if err != nil {
			return err
		}
		if _, err = e.DB.Exec(ctx, `UPDATE strm_repair_items SET state=$3,message=$4 WHERE batch_id=$1 AND ordinal=$2`, in.ID, i.Ordinal, state, message); err != nil {
			return err
		}
	}
	_, err = e.DB.Exec(ctx, `UPDATE strm_repair_batches SET state=CASE WHEN EXISTS(SELECT 1 FROM strm_repair_items WHERE batch_id=$1 AND state='blocked')THEN 'partial' ELSE 'succeeded' END,updated_at=now()WHERE id=$1`, in.ID)
	return err
}

// Retry only an already-approved failed job. Existing journal IDs and immutable
// body digests remain bound to the batch; a lost response does not queue twice.
func (e Engine) RetrySTRMRepair(ctx context.Context, id, digest string, confirm bool) (STRMRepairBatch, error) {
	if !ValidArchiveID(id) || !confirm || len(digest) != 64 {
		return STRMRepairBatch{}, errors.New("请核对并明确恢复当前修复批次")
	}
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return STRMRepairBatch{}, err
	}
	defer deferutil.Rollback(ctx, tx)
	var state, stored, jobState string
	if err = tx.QueryRow(ctx, `SELECT b.state,b.digest,coalesce(j.state,'') FROM strm_repair_batches b LEFT JOIN jobs j ON j.id=b.job_id WHERE b.id=$1 FOR UPDATE OF b`, id).Scan(&state, &stored, &jobState); err != nil {
		return STRMRepairBatch{}, err
	}
	if stored != digest {
		return STRMRepairBatch{}, ErrArtifactConflict
	}
	if state != "running" || jobState != "failed" {
		return e.STRMRepairBatch(ctx, id)
	}
	next := jobs.UUID()
	if _, err = tx.Exec(ctx, `INSERT INTO jobs(id,kind,queue,payload,idempotency_key)VALUES($1,'library.strm.repair','library',jsonb_build_object('batchId',$2::text),$3)`, next, id, "strm-repair:"+id+":"+next); err != nil {
		return STRMRepairBatch{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE strm_repair_batches SET job_id=$2,updated_at=now()WHERE id=$1`, id, next); err != nil {
		return STRMRepairBatch{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','strm.batch_repair_resumed',jsonb_build_object('batchId',$1::text))`, id); err != nil {
		return STRMRepairBatch{}, err
	}
	if err = dbutil.Commit(ctx, tx); err != nil {
		return STRMRepairBatch{}, err
	}
	return e.STRMRepairBatch(ctx, id)
}
