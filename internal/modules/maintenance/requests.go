package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	deferutil "media-tool/internal/platform/cleanup"
	"path/filepath"
	"strings"
	"time"

	"media-tool/internal/platform/buildinfo"
)

type Release struct {
	Revision string `json:"revision"`
	Image    string `json:"image"`
	Schema   int64  `json:"schema"`
	Digest   string `json:"digest"`
	Verified bool   `json:"verified"`
	Current  bool   `json:"current"`
}
type Payload struct {
	ImageDigest     string            `json:"imageDigest,omitempty"`
	BackupID        string            `json:"backupId,omitempty"`
	TargetRevision  string            `json:"targetRevision,omitempty"`
	Image           string            `json:"image,omitempty"`
	Schema          int64             `json:"schema"`
	CurrentRevision string            `json:"currentRevision"`
	Protected       map[string]string `json:"protected"`
	ManifestDigest  string            `json:"manifestDigest,omitempty"`
}
type Request struct {
	VerifiedAt *time.Time `json:"verifiedAt,omitempty"`
	ID         string     `json:"id"`
	Kind       string     `json:"kind"`
	State      string     `json:"state"`
	Payload    Payload    `json:"payload"`
	Digest     string     `json:"digest"`
	ExpiresAt  time.Time  `json:"expiresAt"`
	CreatedAt  time.Time  `json:"createdAt"`
	Message    string     `json:"message"`
}

func (e Engine) RegisterRelease(ctx context.Context, r Release) error {
	if !SHA.MatchString(r.Revision) || r.Image != "dingding229/mediatool:sha-"+r.Revision || r.Schema < 1 || r.Schema > 100000 || len(r.Digest) != 71 || r.Digest[:7] != "sha256:" || !Digest.MatchString(r.Digest[7:]) {
		return ErrUnsafe
	}
	_, err := e.DB.Exec(ctx, `INSERT INTO ops_releases(revision,image,schema_version,digest,verified)VALUES($1,$2,$3,$4,true)ON CONFLICT(revision)DO UPDATE SET image=excluded.image,schema_version=excluded.schema_version,digest=excluded.digest,verified=true`, r.Revision, r.Image, r.Schema, r.Digest)
	return err
}
func (e Engine) Releases(ctx context.Context) ([]Release, error) {
	rows, err := e.DB.Query(ctx, `SELECT revision,image,schema_version,digest,verified FROM ops_releases ORDER BY created_at DESC LIMIT 50`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Release{}
	for rows.Next() {
		var r Release
		if err = rows.Scan(&r.Revision, &r.Image, &r.Schema, &r.Digest, &r.Verified); err != nil {
			return nil, err
		}
		r.Current = r.Revision == e.Revision
		out = append(out, r)
	}
	if SHA.MatchString(e.Revision) {
		found := false
		for _, r := range out {
			found = found || r.Current
		}
		if !found {
			out = append([]Release{{Revision: e.Revision, Image: "dingding229/mediatool:sha-" + e.Revision, Schema: buildinfo.Schema, Current: true}}, out...)
		}
	}
	return out, rows.Err()
}

const requestColumns = `SELECT id::text,kind,state,payload,digest,expires_at,created_at,message,(SELECT verified_at FROM ops_request_verifications v WHERE v.request_id=ops_requests.id AND v.request_digest=ops_requests.digest AND v.revision=$1) FROM ops_requests`

func scanRequest(row interface{ Scan(...any) error }) (Request, error) {
	var r Request
	var raw []byte
	e := row.Scan(&r.ID, &r.Kind, &r.State, &raw, &r.Digest, &r.ExpiresAt, &r.CreatedAt, &r.Message, &r.VerifiedAt)
	if e == nil {
		e = json.Unmarshal(raw, &r.Payload)
	}
	return r, e
}
func (e Engine) Request(ctx context.Context, id string) (Request, error) {
	if !UUID.MatchString(id) {
		return Request{}, ErrUnsafe
	}
	return scanRequest(e.DB.QueryRow(ctx, requestColumns+` WHERE id=$2`, e.Revision, id))
}
func (e Engine) Requests(ctx context.Context) ([]Request, error) {
	rows, err := e.DB.Query(ctx, requestColumns+` ORDER BY created_at DESC,id DESC LIMIT 50`, e.Revision)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Request{}
	for rows.Next() {
		r, err := scanRequest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (e Engine) Preview(ctx context.Context, kind, id string) (Request, error) {
	r := Request{ID: newID(), Kind: kind, State: "preview", CreatedAt: time.Now(), ExpiresAt: time.Now().Add(10 * time.Minute)}
	p := Payload{Schema: buildinfo.Schema, CurrentRevision: e.Revision}
	var actualSchema int64
	if err := e.DB.QueryRow(ctx, SchemaSQL).Scan(&actualSchema); err != nil || actualSchema != buildinfo.Schema {
		return r, errors.New("程序和数据库迁移版本不一致，禁止恢复/回滚")
	}
	var err error
	p.Protected, err = Protected(ctx, e.DB)
	if err != nil {
		return r, err
	}
	switch kind {
	case "restore":
		m, _, err := e.VerifyBundle(ctx, id)
		if err != nil {
			return r, err
		}
		if m.Schema != buildinfo.Schema {
			return r, errors.New("备份数据库版本不同，禁止跨版本自动恢复")
		}
		var verified bool
		if err = e.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM ops_drills WHERE backup_id=$1 AND state='verified' AND completed_at>now()-interval '24 hours')`, id).Scan(&verified); err != nil || !verified {
			return r, errors.New("请先完成24小时内的恢复演练")
		}
		if !sameProtected(m.Protected, p.Protected) {
			return r, errors.New("账号、授权或媒体归属已变化，恢复将回退保护数据，已阻止")
		}
		p.BackupID = id
		var digest string
		if checkedErr := e.DB.QueryRow(ctx, `SELECT digest FROM ops_backups WHERE id=$1`, id).Scan(&digest); checkedErr != nil {
			return Request{}, checkedErr
		}
		p.ManifestDigest = digest
	case "rollback":
		if id == e.Revision || !SHA.MatchString(id) {
			return r, ErrUnsafe
		}
		var target Release
		if err = e.DB.QueryRow(ctx, `SELECT revision,image,schema_version,verified,digest FROM ops_releases WHERE revision=$1`, id).Scan(&target.Revision, &target.Image, &target.Schema, &target.Verified, &target.Digest); err != nil || !target.Verified {
			return r, errors.New("版本须由宿主工具核验登记")
		}
		if target.Schema != buildinfo.Schema {
			return r, errors.New("数据库迁移版本不匹配，禁止仅回滚镜像")
		}
		p.TargetRevision = id
		p.Image = target.Image
		p.ImageDigest = target.Digest
	default:
		return r, ErrUnsafe
	}
	r.Payload = p
	raw, _ := json.Marshal(p)
	r.Digest = hash(raw)
	_, err = e.DB.Exec(ctx, `INSERT INTO ops_requests(id,kind,payload,digest,expires_at)VALUES($1,$2,$3,$4,$5)`, r.ID, kind, raw, r.Digest, r.ExpiresAt)
	return r, err
}
func sameProtected(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
func (e Engine) CheckRequest(ctx context.Context, id string) (Request, error) {
	r, err := e.Request(ctx, id)
	if err != nil {
		return r, err
	}
	raw, _ := json.Marshal(r.Payload)
	if hash(raw) != r.Digest {
		return r, ErrUnsafe
	}
	var schema int64
	if err = e.DB.QueryRow(ctx, SchemaSQL).Scan(&schema); err != nil || schema != r.Payload.Schema {
		return r, ErrUnsafe
	}
	if !r.ExpiresAt.After(time.Now()) || r.Payload.CurrentRevision != e.Revision || r.Payload.Schema != buildinfo.Schema {
		return r, ErrConflict
	}
	current, err := Protected(ctx, e.DB)
	if err != nil {
		return r, err
	}
	if !sameProtected(current, r.Payload.Protected) {
		return r, ErrConflict
	}
	if r.Kind == "restore" {
		_, _, err := e.VerifyBundle(ctx, r.Payload.BackupID)
		if err != nil {
			return r, err
		}
		var digest string
		if checkedErr := e.DB.QueryRow(ctx, `SELECT digest FROM ops_backups WHERE id=$1`, r.Payload.BackupID).Scan(&digest); checkedErr != nil {
			return Request{}, checkedErr
		}
		if digest != r.Payload.ManifestDigest {
			return r, ErrConflict
		}
	}
	return r, nil
}
func (e Engine) Approve(ctx context.Context, id, digest, text string) error {
	r, err := e.CheckRequest(ctx, id)
	if err != nil {
		return err
	}
	if r.State != "preview" || r.Digest != digest || text != ConfirmText(r) {
		return ErrUnsafe
	}
	tag, err := e.DB.Exec(ctx, `UPDATE ops_requests SET state='approved',approved_at=now(),message='已审批，等待宿主机受控执行器；当前未恢复或回滚' WHERE id=$1 AND state='preview' AND digest=$2 AND expires_at>now()`, id, digest)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrConflict
	}
	return nil
}
func ConfirmText(r Request) string {
	if r.Kind == "restore" {
		return "恢复备份 " + r.Payload.BackupID
	}
	return "回滚版本 " + r.Payload.TargetRevision
}
func (e Engine) Claim(ctx context.Context, id string) (Request, error) {
	r, err := e.CheckRequest(ctx, id)
	if err != nil {
		return r, err
	}
	if r.State != "approved" {
		return r, ErrConflict
	}
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return r, err
	}
	defer deferutil.Rollback(ctx, tx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(91220880)`); err != nil {
		return r, err
	}
	var paused bool
	if err = tx.QueryRow(ctx, `SELECT paused FROM ops_mode WHERE singleton FOR UPDATE`).Scan(&paused); err != nil {
		return r, err
	}
	if active, checkErr := e.WebRestoreActive(ctx); checkErr != nil || active {
		return r, ErrConflict
	}
	if paused {
		return r, errors.New("已有维护窗口，不能并发执行")
	}
	tag, err := tx.Exec(ctx, `UPDATE ops_requests SET state='executing',message='宿主执行器已领取，不自动重放'WHERE id=$1 AND state='approved'AND expires_at>now()`, id)
	if err != nil || tag.RowsAffected() != 1 {
		return r, ErrConflict
	}
	if _, err = tx.Exec(ctx, `UPDATE ops_mode SET paused=true,reason=$1,revision=revision+1 WHERE singleton`, "维护单 "+id); err != nil {
		return r, err
	}
	r.State = "executing"
	return r, tx.Commit(ctx)
}
func requireOneAffected(tag pgconn.CommandTag) error {
	if tag.RowsAffected() != 1 {
		return ErrConflict
	}
	return nil
}

func (e Engine) Finish(ctx context.Context, id, message string, success bool) error {
	state := "failed"
	if success {
		state = "completed"
	}
	tag, err := e.DB.Exec(ctx, `UPDATE ops_requests SET state=$2,message=$3,completed_at=now() WHERE id=$1 AND state='executing'`, id, state, message)
	if err != nil {
		return err
	}
	if err = requireOneAffected(tag); err != nil {
		return err
	}
	return nil
}
func (e Engine) Resume(ctx context.Context, revision int64) error {
	if active, err := e.WebRestoreActive(ctx); err != nil || active {
		return ErrConflict
	}
	tx, err := e.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return err
	}
	defer deferutil.Rollback(ctx, tx)
	var reason string
	if err = tx.QueryRow(ctx, `SELECT reason FROM ops_mode WHERE singleton AND revision=$1 FOR UPDATE`, revision).Scan(&reason); err != nil {
		return ErrConflict
	}
	if strings.HasPrefix(reason, "维护单 ") {
		id := strings.TrimPrefix(reason, "维护单 ")
		var state string
		if err = tx.QueryRow(ctx, `SELECT state FROM ops_requests WHERE id=$1 FOR UPDATE`, id).Scan(&state); err != nil {
			return err
		}
		if state == "completed" {
			var proof string
			err = tx.QueryRow(ctx, `SELECT v.protected_digest FROM ops_request_verifications v JOIN ops_requests r ON r.id=v.request_id WHERE r.id=$1 AND v.request_digest=r.digest AND v.revision=$2 AND v.schema_version=$3`, id, e.Revision, buildinfo.Schema).Scan(&proof)
			if err != nil {
				return ErrConflict
			}
			current, e := Protected(ctx, tx)
			if e != nil {
				return e
			}
			if protectedDigest(current) != proof {
				return ErrConflict
			}
		}
	}
	tag, err := tx.Exec(ctx, `UPDATE ops_mode SET paused=false,reason='',revision=revision+1 WHERE singleton AND revision=$1 AND NOT EXISTS(SELECT 1 FROM ops_requests WHERE state='executing')`, revision)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrConflict
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	e.notice(ctx, "ops.maintenance.resumed", "维护窗口已解除", "业务Cron和队列重新启用")
	return nil
}
func (e Engine) PrepareCandidate(ctx context.Context, id string) (candidate string, returnErr error) {
	r, err := e.Request(ctx, id)
	if err != nil || r.State != "executing" || r.Kind != "restore" {
		return "", ErrUnsafe
	}
	m, dir, err := e.VerifyBundle(ctx, r.Payload.BackupID)
	if err != nil {
		return "", err
	}
	current, err := Protected(ctx, e.DB)
	if err != nil || !sameProtected(current, r.Payload.Protected) {
		return "", ErrConflict
	}
	name := "media_tool_restore_" + stripID(id)[:12]
	dsn, cleanupCandidate, err := e.restoreClone(ctx, name, dir, m)
	if err != nil {
		return "", err
	}
	candidateCommitted := false
	defer cleanupFailedCandidate(&candidateCommitted, &returnErr, cleanupCandidate)
	db, err := databaseOpen(ctx, dsn)
	if err != nil {
		return "", err
	}
	defer db.Close()
	if err = e.copyCurrentMetadata(ctx, db); err != nil {
		return "", err
	}
	_, err = db.Exec(ctx, `UPDATE jobs SET state='paused',lease_until=NULL,lease_token=NULL,locked_by=NULL,last_error='数据库恢复后需人工核对，禁止自动重放',updated_at=now() WHERE state IN('pending','retry','running');UPDATE administrator_sessions SET revoked_at=now();UPDATE media_user_sessions SET revoked_at=now();UPDATE outbox_events SET dispatched_at=now() WHERE dispatched_at IS NULL;UPDATE notification_deliveries SET state='outcome_unknown',last_error='恢复后旧投递需人工核对，不自动重发' WHERE state IN('pending','retry','sending');UPDATE ops_mode SET paused=true,reason='恢复后只读核对，须显式解除维护',revision=revision+1 WHERE singleton;`)
	if err != nil {
		return "", err
	}
	if _, err = db.Exec(ctx, `UPDATE ops_requests SET state='failed',message='恢复后旧批准无效' WHERE state IN('preview','approved','executing')AND id<>$1`, id); err != nil {
		return "", err
	}
	if err = e.restoreOutputs(ctx, filepath.Join(dir, "outputs.tar"), m.Files); err != nil {
		return "", err
	}
	candidateCommitted = true
	return name, nil
}
func stripID(s string) string { return strings.ReplaceAll(s, "-", "") }
