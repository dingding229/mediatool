package library

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"media-tool/internal/modules/jobs"
)

type ArchivePolicy struct {
	RetentionDays    int   `json:"retentionDays"`
	Revision         int64 `json:"revision"`
	AutomaticCleanup bool  `json:"automaticCleanup"`
}
type RetiredArchive struct {
	ID            string     `json:"id"`
	AssetID       string     `json:"assetId"`
	OriginalPath  string     `json:"originalPath"`
	ArchivePath   string     `json:"archivePath"`
	Kind          string     `json:"kind"`
	ArchivedAt    time.Time  `json:"archivedAt"`
	EligibleAfter time.Time  `json:"eligibleAfter"`
	State         string     `json:"state"`
	CleanupID     *string    `json:"cleanupId,omitempty"`
	Message       string     `json:"message"`
	DeletedAt     *time.Time `json:"deletedAt,omitempty"`
}
type ArchiveCleanupPreview struct {
	PlanDigest           string           `json:"planDigest"`
	Policy               ArchivePolicy    `json:"policy"`
	Items                []RetiredArchive `json:"items"`
	SourceFilesUntouched bool             `json:"sourceFilesUntouched"`
	PhysicalDeletion     bool             `json:"physicalDeletion"`
}
type ArchiveCleanupRequest struct {
	RequestID  string   `json:"requestId"`
	ArchiveIDs []string `json:"archiveIds"`
	PlanDigest string   `json:"planDigest"`
}
type ArchiveCleanupResult struct {
	ID        string `json:"id"`
	ArchiveID string `json:"archiveId"`
	State     string `json:"state"`
	Message   string `json:"message"`
}

func (e Engine) ArchivePolicy(ctx context.Context) (ArchivePolicy, error) {
	var p ArchivePolicy
	err := e.DB.QueryRow(ctx, `SELECT retention_days,revision FROM media_archive_policy WHERE singleton`).Scan(&p.RetentionDays, &p.Revision)
	return p, err
}
func (e Engine) SaveArchivePolicy(ctx context.Context, in ArchivePolicy) (ArchivePolicy, error) {
	if in.RetentionDays < 1 || in.RetentionDays > 3650 || in.AutomaticCleanup {
		return in, errors.New("retention must be 1..3650 days; automatic archive deletion is not supported")
	}
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return in, err
	}
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, `UPDATE media_archive_policy SET retention_days=$1,revision=revision+1,updated_at=now() WHERE singleton AND revision=$2 RETURNING revision`, in.RetentionDays, in.Revision).Scan(&in.Revision)
	if err != nil {
		return in, errors.New("archive policy changed; reload before editing")
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','archive.retention_changed',jsonb_build_object('days',$1::int,'revision',$2::bigint))`, in.RetentionDays, in.Revision); err != nil {
		return in, err
	}
	return in, tx.Commit(ctx)
}

var archiveUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func ValidArchiveID(id string) bool { return archiveUUID.MatchString(id) }

func archiveIDs(ids []string) ([]string, error) {
	if len(ids) == 0 || len(ids) > 20 {
		return nil, errors.New("select 1..20 explicit archives")
	}
	out := append([]string(nil), ids...)
	for i := range out {
		out[i] = strings.ToLower(out[i])
	}
	sort.Strings(out)
	for i, id := range out {
		if !archiveUUID.MatchString(id) || i > 0 && id == out[i-1] {
			return nil, errors.New("invalid/duplicate archive selection")
		}
	}
	return out, nil
}
func (e Engine) Archives(ctx context.Context, limit, offset int) ([]RetiredArchive, int, error) {
	if limit < 1 || limit > 100 || offset < 0 {
		return nil, 0, errors.New("invalid archive page")
	}
	rows, err := e.DB.Query(ctx, `SELECT j.id::text,j.asset_id::text,j.relative_path,j.archive_relative,j.artifact_kind,j.updated_at,j.updated_at+p.retention_days*interval '1 day',coalesce(c.state,'retained'),c.id::text,coalesce(c.message,''),j.archive_deleted_at FROM media_artifact_journal j CROSS JOIN media_archive_policy p LEFT JOIN media_archive_cleanup_operations c ON c.journal_id=j.id WHERE j.operation='archive' AND j.state='applied' ORDER BY j.updated_at DESC,j.id LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []RetiredArchive{}
	for rows.Next() {
		var a RetiredArchive
		if err = rows.Scan(&a.ID, &a.AssetID, &a.OriginalPath, &a.ArchivePath, &a.Kind, &a.ArchivedAt, &a.EligibleAfter, &a.State, &a.CleanupID, &a.Message, &a.DeletedAt); err != nil {
			return nil, 0, err
		}
		items = append(items, a)
	}
	if err = rows.Err(); err != nil {
		return nil, 0, err
	}
	var count int
	err = e.DB.QueryRow(ctx, `SELECT count(*) FROM media_artifact_journal WHERE operation='archive' AND state='applied'`).Scan(&count)
	return items, count, err
}

func (e Engine) cleanupPlan(ctx context.Context, tx pgx.Tx, ids []string) (ArchiveCleanupPreview, error) {
	var p ArchiveCleanupPreview
	if err := tx.QueryRow(ctx, `SELECT retention_days,revision FROM media_archive_policy WHERE singleton FOR SHARE`).Scan(&p.Policy.RetentionDays, &p.Policy.Revision); err != nil {
		return p, err
	}
	p.SourceFilesUntouched = true
	p.PhysicalDeletion = true
	root, err := filepath.Abs(e.Connections.Config.STRMOutputDir)
	if err != nil {
		return p, err
	}
	fs, err := os.OpenRoot(root)
	if err != nil {
		return p, err
	}
	defer fs.Close()
	type proof struct {
		ID, Asset, Root, Path, Archive, Kind, Digest string
		Archived                                     time.Time
	}
	proofs := []proof{}
	for _, id := range ids {
		var a RetiredArchive
		var recordRoot, scope, state, operation string
		var digest []byte
		var approved bool
		var artifactState string
		err = tx.QueryRow(ctx, `SELECT j.id::text,j.asset_id::text,j.relative_path,j.archive_relative,j.artifact_kind,j.updated_at,j.archive_deleted_at,j.root_path,j.owner_scope,j.state,j.operation,j.expected_digest,a.artifacts_state,EXISTS(SELECT 1 FROM media_archive_cleanup_operations c WHERE c.journal_id=j.id) FROM media_artifact_journal j JOIN media_assets a ON a.id=j.asset_id WHERE j.id=$1 FOR UPDATE OF j FOR SHARE OF a`, id).Scan(&a.ID, &a.AssetID, &a.OriginalPath, &a.ArchivePath, &a.Kind, &a.ArchivedAt, &a.DeletedAt, &recordRoot, &scope, &state, &operation, &digest, &artifactState, &approved)
		if err != nil {
			return p, errors.New("selected archive unavailable")
		}
		expectedPath := path.Join(".media-tool-retired", a.ID, a.OriginalPath+".retired")
		if operation != "archive" || state != "applied" || a.DeletedAt != nil || approved || recordRoot != root || scope != "asset:"+a.AssetID || a.ArchivePath != expectedPath || path.Base(a.OriginalPath) == "tvshow.nfo" || artifactState != "ready" {
			return p, errors.New("archive is not a retained, owned, completed output")
		}
		a.EligibleAfter = a.ArchivedAt.Add(time.Duration(p.Policy.RetentionDays) * 24 * time.Hour)
		a.State = "retained"
		if time.Now().Before(a.EligibleAfter) {
			return p, errors.New("archive retention has not elapsed")
		}
		var pending bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM media_artifact_journal WHERE asset_id=$1 AND state<>'applied')`, a.AssetID).Scan(&pending); err != nil {
			return p, err
		}
		if pending {
			return p, ErrArtifactDependency
		}
		if err = activeArchiveOutputs(ctx, tx, a.AssetID, root); err != nil {
			return p, err
		}
		raw, readErr := readRegular(fs, a.ArchivePath)
		if readErr != nil || !digestEqual(raw, digest) {
			return p, ErrArtifactConflict
		}
		p.Items = append(p.Items, a)
		proofs = append(proofs, proof{a.ID, a.AssetID, recordRoot, a.OriginalPath, a.ArchivePath, a.Kind, hex.EncodeToString(digest), a.ArchivedAt})
	}
	raw, err := json.Marshal(struct {
		Policy ArchivePolicy
		Proofs []proof
	}{p.Policy, proofs})
	if err != nil {
		return p, err
	}
	sum := sha256.Sum256(raw)
	p.PlanDigest = hex.EncodeToString(sum[:])
	return p, nil
}
func (e Engine) PreviewArchiveCleanup(ctx context.Context, ids []string) (ArchiveCleanupPreview, error) {
	ordered, err := archiveIDs(ids)
	if err != nil {
		return ArchiveCleanupPreview{}, err
	}
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return ArchiveCleanupPreview{}, err
	}
	defer tx.Rollback(ctx)
	return e.cleanupPlan(ctx, tx, ordered)
}

func (e Engine) ConfirmArchiveCleanup(ctx context.Context, in ArchiveCleanupRequest) ([]ArchiveCleanupResult, error) {
	ordered, err := archiveIDs(in.ArchiveIDs)
	if err != nil || !ValidArchiveID(in.RequestID) || len(in.PlanDigest) != 64 {
		return nil, errors.New("explicit archive cleanup preview/request ID required")
	}
	in.RequestID = strings.ToLower(in.RequestID)
	if _, err := hex.DecodeString(in.PlanDigest); err != nil {
		return nil, errors.New("invalid archive preview digest")
	}
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,719))`, in.RequestID); err != nil {
		return nil, err
	}
	var oldDigest string
	var oldIDs []string
	err = tx.QueryRow(ctx, `SELECT plan_digest,archive_ids FROM media_archive_cleanup_requests WHERE id=$1`, in.RequestID).Scan(&oldDigest, &oldIDs)
	if err == nil {
		if oldDigest != in.PlanDigest || stringsJoinIDs(oldIDs) != stringsJoinIDs(ordered) {
			return nil, errors.New("cleanup request ID was used for a different plan")
		}
		return e.cleanupResults(ctx, tx, in.RequestID)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	plan, err := e.cleanupPlan(ctx, tx, ordered)
	if err != nil {
		return nil, err
	}
	if plan.PlanDigest != in.PlanDigest {
		return nil, errors.New("archive cleanup preview changed; preview again")
	}
	if _, err = tx.Exec(ctx, `INSERT INTO media_archive_cleanup_requests(id,plan_digest,archive_ids,policy_revision,retention_days)VALUES($1,$2,$3::uuid[],$4,$5)`, in.RequestID, in.PlanDigest, ordered, plan.Policy.Revision, plan.Policy.RetentionDays); err != nil {
		return nil, err
	}
	operations := []string{}
	for _, a := range plan.Items {
		id := jobs.UUID()
		quarantine := path.Join(path.Dir(a.ArchivePath), ".cleanup-"+id+".retired")
		if _, err = tx.Exec(ctx, `INSERT INTO media_archive_cleanup_operations(id,request_id,journal_id,expected_digest,quarantine_relative)SELECT $1,$2,id,expected_digest,$4 FROM media_artifact_journal WHERE id=$3`, id, in.RequestID, a.ID, quarantine); err != nil {
			return nil, err
		}
		operations = append(operations, id)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','archive.cleanup_approved',jsonb_build_object('requestId',$1::text,'count',$2::int))`, in.RequestID, len(ordered)); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	// Only this explicit human request executes removals. There is no periodic
	// cleanup worker: an interrupted approval is listed for explicit retry.
	for _, id := range operations {
		if ctx.Err() != nil {
			break
		}
		_ = e.ApplyArchiveCleanup(ctx, id)
	}
	return e.ArchiveCleanupResults(ctx, in.RequestID)
}
func stringsJoinIDs(ids []string) string {
	var s string
	for _, id := range ids {
		s += id + ";"
	}
	return s
}
func (e Engine) cleanupResults(ctx context.Context, tx pgx.Tx, request string) ([]ArchiveCleanupResult, error) {
	rows, err := tx.Query(ctx, `SELECT id::text,journal_id::text,state,message FROM media_archive_cleanup_operations WHERE request_id=$1 ORDER BY journal_id`, request)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []ArchiveCleanupResult{}
	for rows.Next() {
		var r ArchiveCleanupResult
		if err = rows.Scan(&r.ID, &r.ArchiveID, &r.State, &r.Message); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	return list, rows.Err()
}
func (e Engine) ArchiveCleanupResults(ctx context.Context, request string) ([]ArchiveCleanupResult, error) {
	if !ValidArchiveID(request) {
		return nil, errors.New("invalid archive cleanup request ID")
	}
	request = strings.ToLower(request)
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	return e.cleanupResults(ctx, tx, request)
}

// Keep the last known working output, not just a database 'ready' label.
// File-Key rotation is allowed when the current manifest and content agree.
func activeArchiveOutputs(ctx context.Context, tx pgx.Tx, asset, root string) error {
	rows, err := tx.Query(ctx, `SELECT s.output_path,m.content_digest,m.owner_scope,m.asset_id::text FROM strm_entries s LEFT JOIN managed_media_artifacts m ON m.relative_path=s.output_path AND m.artifact_kind='strm' WHERE s.asset_id=$1 ORDER BY s.output_path`, asset)
	if err != nil {
		return err
	}
	type output struct {
		p            string
		digest       []byte
		scope, owner *string
	}
	list := []output{}
	for rows.Next() {
		var o output
		if err = rows.Scan(&o.p, &o.digest, &o.scope, &o.owner); err != nil {
			rows.Close()
			return err
		}
		list = append(list, o)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(list) == 0 || len(list) > 100 {
		return ErrArtifactDependency
	}
	fs, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer fs.Close()
	for _, o := range list {
		if o.scope == nil || o.owner == nil || *o.scope != "asset:"+asset || *o.owner != asset {
			return ErrArtifactDependency
		}
		raw, err := readRegular(fs, o.p)
		if err != nil || !digestEqual(raw, o.digest) {
			return ErrArtifactDependency
		}
	}
	return nil
}
