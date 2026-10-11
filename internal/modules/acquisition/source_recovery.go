package acquisition

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path"
	"strings"

	"media-tool/internal/adapters/qbittorrent"
	"media-tool/internal/modules/downloads"
	"media-tool/internal/modules/search"
	"media-tool/internal/modules/subscriptionpolicy"
)

var ErrSourceRecoveryRequired = errors.New("下载来源需要专用重新确认，通用重试不会更换来源")
var ErrSourceRecoveryConflict = errors.New("下载任务或来源预览已变化，请重新预览后确认")
var ErrDownloadIdentityConflict = errors.New("该资源已有不同媒体、季集或种子身份的任务，不能自动复用或改绑")

type SourcePreview struct {
	TaskID                string   `json:"taskId"`
	ResourceID            string   `json:"resourceId"`
	SearchRunID           string   `json:"searchRunId"`
	CurrentSearchRunID    string   `json:"currentSearchRunId"`
	ControlRevision       int64    `json:"controlRevision"`
	PlanDigest            string   `json:"planDigest"`
	Mode                  string   `json:"mode"`
	OriginalHash          string   `json:"originalHash"`
	CandidateHash         string   `json:"candidateHash"`
	CandidateTitle        string   `json:"candidateTitle"`
	DownloaderObservation string   `json:"downloaderObservation"`
	Eligible              bool     `json:"eligible"`
	Reasons               []string `json:"reasons"`
}
type SourceReconfirmInput struct {
	SearchRunID            string `json:"searchRunId"`
	ControlRevision        int64  `json:"controlRevision"`
	PlanDigest             string `json:"planDigest"`
	ConfirmSourceBinding   bool   `json:"confirmSourceBinding"`
	ConfirmInitialIdentity bool   `json:"confirmInitialIdentity"`
}
type SourceReconfirmResult struct {
	TaskID                string `json:"taskId"`
	ControlRevision       int64  `json:"controlRevision"`
	AlreadyApplied        bool   `json:"alreadyApplied"`
	RecoveryStillRequired bool   `json:"recoveryStillRequired"`
}
type recoveryTask struct {
	ID, ResourceID, ConnectionID, Hash, State, JobID, JobState string
	Revision                                                   int64
	Required                                                   bool
	Details                                                    taskDetails
	RawDetails                                                 []byte
}
type recoveryPlan struct {
	Preview              SourcePreview
	Task                 recoveryTask
	Torrent              downloads.TorrentMetadata
	SnapshotDigest       string
	ConnectionRevision   int64
	SubscriptionRevision int64
}

func (e Engine) recoveryTask(ctx context.Context, id string) (recoveryTask, error) {
	t := recoveryTask{ID: id}
	err := e.DB.QueryRow(ctx, `SELECT resource_id::text,integration_id::text,coalesce(info_hash,''),download_state,details,source_control_revision,source_recovery_required FROM download_tasks WHERE id=$1`, id).Scan(&t.ResourceID, &t.ConnectionID, &t.Hash, &t.State, &t.RawDetails, &t.Revision, &t.Required)
	if err != nil || json.Unmarshal(t.RawDetails, &t.Details) != nil {
		return t, ErrSourceRecoveryConflict
	}
	err = e.DB.QueryRow(ctx, `SELECT id::text,state FROM jobs WHERE idempotency_key=$1 AND kind='download.add'`, "download.add:"+id).Scan(&t.JobID, &t.JobState)
	if err != nil {
		return t, ErrSourceRecoveryConflict
	}
	return t, nil
}

func (e Engine) recoveryPolicy(ctx context.Context, t recoveryTask, candidate search.Resource) (int64, []string, error) {
	if t.Details.SubscriptionRunID == "" {
		return 0, nil, nil
	}
	var revision, runRevision int64
	var enabled bool
	var raw []byte
	err := e.DB.QueryRow(ctx, `SELECT s.revision,s.enabled,s.rules,r.revision FROM subscription_runs r JOIN subscriptions s ON s.id=r.subscription_id WHERE r.id=$1`, t.Details.SubscriptionRunID).Scan(&revision, &enabled, &raw, &runRevision)
	if err != nil {
		return 0, nil, ErrSourceRecoveryConflict
	}
	if !enabled || revision != runRevision {
		return revision, []string{"原订阅已停用或规则已修改；请重新评估，不能恢复旧自动下载"}, nil
	}
	var rules subscriptionpolicy.Rules
	if json.Unmarshal(raw, &rules) != nil {
		return 0, nil, ErrSourceRecoveryConflict
	}
	_, reasons := subscriptionpolicy.Filter(candidate, t.Details.Media, rules)
	return revision, reasons, nil
}

// Preview reads local state, the selected PT torrent metadata, and qB's filtered
// hash lookup. It never adds/pauses/removes a torrent, writes a binding, or exposes
// signed URLs. PT token generation/lookup may update that provider's counters.
func (e Engine) SourcePreview(ctx context.Context, id, runID string) (SourcePreview, error) {
	plan, err := e.sourcePlan(ctx, id, runID)
	return plan.Preview, err
}
func (e Engine) sourcePlan(ctx context.Context, id, runID string) (recoveryPlan, error) {
	p := recoveryPlan{}
	t, err := e.recoveryTask(ctx, id)
	if err != nil {
		return p, err
	}
	p.Task = t
	if runID == "" {
		runID, err = e.Search.LatestRunForResource(ctx, t.ResourceID)
		if err != nil {
			return p, err
		}
	}
	p.Preview = SourcePreview{TaskID: id, ResourceID: t.ResourceID, SearchRunID: runID, CurrentSearchRunID: t.Details.SearchRunID, ControlRevision: t.Revision, Reasons: []string{}}
	if t.State != "pending" || !t.Required {
		p.Preview.Reasons = append(p.Preview.Reasons, "仅已标记来源恢复的待提交任务可重新确认")
		return p, nil
	}
	if t.JobState != "paused" && t.JobState != "failed" {
		p.Preview.Reasons = append(p.Preview.Reasons, "下载任务仍在执行或排队，请等待来源暂停完成")
		return p, nil
	}
	if err = e.Search.ValidateRunResource(ctx, runID, t.ResourceID); err != nil {
		return p, err
	}
	originalSnapshotDigest, err := e.Search.SourceSnapshotDigest(ctx, runID, t.ResourceID)
	if err != nil {
		return p, err
	}
	candidate, err := e.Search.RunResource(ctx, runID, t.ResourceID)
	if err != nil {
		return p, err
	}
	p.Preview.CandidateTitle = candidate.Title
	raw, err := e.Search.TorrentFromRun(ctx, runID, t.ResourceID)
	if err != nil {
		return p, err
	}
	p.Torrent, err = downloads.ParseTorrent(raw)
	if err != nil {
		return p, errors.New("新来源种子元数据无效，不能确认")
	}
	p.Preview.CandidateHash = p.Torrent.InfoHash
	original := t.Hash
	if original == "" {
		original = t.Details.ExpectedHash
	}
	p.Preview.OriginalHash = original
	p.Preview.Mode = "same_identity"
	if original == "" {
		p.Preview.Mode = "initial_identity"
	} else if original != p.Torrent.InfoHash {
		p.Preview.Reasons = append(p.Preview.Reasons, "新来源InfoHash与原任务不同；不允许更换种子身份")
	}
	if t.Hash != "" && t.Details.ExpectedHash != "" && t.Hash != t.Details.ExpectedHash {
		p.Preview.Reasons = append(p.Preview.Reasons, "原任务内部种子身份冲突，需要人工核对")
	}
	p.SubscriptionRevision, p.Preview.Reasons, err = e.policyReasons(ctx, t, candidate, p.Preview.Reasons)
	if err != nil {
		return p, err
	}
	connection, err := e.Connections.Repository.Get(ctx, t.ConnectionID)
	if err != nil {
		return p, err
	}
	if !connection.Record.Enabled || connection.Record.Kind != "qbittorrent" || connection.String("endpoint") != t.Details.Endpoint || connection.String("downloadPath") != t.Details.DownloadPath || connection.String("localPath") != t.Details.LocalPath {
		p.Preview.Reasons = append(p.Preview.Reasons, "下载器连接或路径已变化，不自动改写原任务")
	}
	p.ConnectionRevision = connection.Record.Revision
	if len(p.Preview.Reasons) > 0 {
		return p, nil
	}
	client, err := e.client(ctx, t.ConnectionID, t.Details)
	if err != nil {
		return p, err
	}
	actual, err := client.InspectAliases(ctx, p.Torrent.InfoHash, p.Torrent.Aliases)
	if errors.Is(err, qbittorrent.ErrNotFound) {
		p.Preview.DownloaderObservation = "absent"
	} else if err != nil {
		return p, errors.New("下载器状态未确认，不能恢复")
	} else {
		p.Preview.DownloaderObservation = "present"
		if actual.ID != p.Torrent.InfoHash && !containsHash(p.Torrent.Aliases, actual.ID) {
			return p, errors.New("下载器返回不匹配的种子，不能确认")
		}
		if path.Clean(actual.SavePath) != path.Clean(t.Details.DownloadPath) {
			p.Preview.Reasons = append(p.Preview.Reasons, "下载器已有同Hash任务，但保存位置不同；需人工核对")
		}
	}
	// PT/qB reads occur without write locks. Their duration is not permission
	// to approve origin evidence changed while either request was in flight.
	if err = e.Search.ValidateRunResource(ctx, runID, t.ResourceID); err != nil {
		return p, err
	}
	p.SnapshotDigest, err = e.Search.SourceSnapshotDigest(ctx, runID, t.ResourceID)
	if err != nil {
		return p, err
	}
	if p.SnapshotDigest != originalSnapshotDigest {
		return p, ErrSourceRecoveryConflict
	}
	// Approval binds every local/remote observation, but only opaque hashes escape.
	rawProof, _ := json.Marshal(struct {
		Task                     recoveryTask
		Snapshot                 string
		Hash                     string
		Files                    []downloads.TorrentFile
		Downloader               string
		Connection, Subscription int64
	}{t, p.SnapshotDigest, p.Torrent.InfoHash, p.Torrent.Files, p.Preview.DownloaderObservation, p.ConnectionRevision, p.SubscriptionRevision})
	digest := sha256.Sum256(rawProof)
	p.Preview.PlanDigest = hex.EncodeToString(digest[:])
	p.Preview.Eligible = len(p.Preview.Reasons) == 0
	return p, nil
}
func (e Engine) policyReasons(ctx context.Context, t recoveryTask, candidate search.Resource, existing []string) (int64, []string, error) {
	revision, reasons, err := e.recoveryPolicy(ctx, t, candidate)
	return revision, append(existing, reasons...), err
}
func containsHash(values []string, value string) bool {
	for _, v := range values {
		if strings.EqualFold(v, value) {
			return true
		}
	}
	return false
}

func (e Engine) ReconfirmSource(ctx context.Context, id string, in SourceReconfirmInput) (SourceReconfirmResult, error) {
	answer := SourceReconfirmResult{TaskID: id}
	if !in.ConfirmSourceBinding || in.ControlRevision < 1 || len(in.PlanDigest) != 64 || in.SearchRunID == "" {
		return answer, ErrSourceRecoveryConflict
	}
	// Exact accepted requests are receipts, not permission to reset a newer job.
	if receipt, found, err := e.sourceReceipt(ctx, id, in); err != nil || found {
		return receipt, err
	}
	plan, err := e.sourcePlan(ctx, id, in.SearchRunID)
	if err != nil {
		return answer, err
	}
	if !plan.Preview.Eligible || plan.Preview.ControlRevision != in.ControlRevision || plan.Preview.PlanDigest != in.PlanDigest {
		if receipt, found, err := e.sourceReceipt(ctx, id, in); err != nil || found {
			return receipt, err
		}
		return answer, ErrSourceRecoveryConflict
	}
	if plan.Preview.Mode == "initial_identity" && !in.ConfirmInitialIdentity {
		return answer, ErrSourceRecoveryConflict
	}
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return answer, err
	}
	defer tx.Rollback(ctx)
	// Per-task advisory serialization matches Add; it does not hold network locks.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "download-task:"+id); err != nil {
		return answer, err
	}
	var current recoveryTask
	current.ID = id
	if err = tx.QueryRow(ctx, `SELECT resource_id::text,integration_id::text,coalesce(info_hash,''),download_state,details,source_control_revision,source_recovery_required FROM download_tasks WHERE id=$1 FOR UPDATE`, id).Scan(&current.ResourceID, &current.ConnectionID, &current.Hash, &current.State, &current.RawDetails, &current.Revision, &current.Required); err != nil {
		return answer, ErrSourceRecoveryConflict
	}
	if current.Revision != plan.Task.Revision || current.State != "pending" || !current.Required || string(current.RawDetails) != string(plan.Task.RawDetails) || current.Hash != plan.Task.Hash {
		if receipt, found, err := e.sourceReceipt(ctx, id, in); err != nil || found {
			return receipt, err
		}
		return answer, ErrSourceRecoveryConflict
	}
	var jobState string
	if err = tx.QueryRow(ctx, `SELECT state FROM jobs WHERE id=$1 FOR UPDATE`, plan.Task.JobID).Scan(&jobState); err != nil || jobState != plan.Task.JobState {
		return answer, ErrSourceRecoveryConflict
	}
	if err = e.Search.ValidateRunResourceTx(ctx, tx, in.SearchRunID, current.ResourceID); err != nil {
		return answer, err
	}
	snapshotDigest, err := e.Search.SourceSnapshotDigest(ctx, in.SearchRunID, current.ResourceID)
	if err != nil || snapshotDigest != plan.SnapshotDigest {
		return answer, ErrSourceRecoveryConflict
	}
	var connectionRevision int64
	if err = tx.QueryRow(ctx, `SELECT revision FROM integrations WHERE id=$1 AND enabled FOR SHARE`, current.ConnectionID).Scan(&connectionRevision); err != nil || connectionRevision != plan.ConnectionRevision {
		return answer, ErrSourceRecoveryConflict
	}
	if plan.Task.Details.SubscriptionRunID != "" {
		var enabled bool
		var subRevision, runRevision int64
		if err = tx.QueryRow(ctx, `SELECT s.enabled,s.revision,r.revision FROM subscription_runs r JOIN subscriptions s ON s.id=r.subscription_id WHERE r.id=$1 FOR SHARE OF s`, plan.Task.Details.SubscriptionRunID).Scan(&enabled, &subRevision, &runRevision); err != nil || !enabled || subRevision != plan.SubscriptionRevision || runRevision != subRevision {
			return answer, ErrSourceRecoveryConflict
		}
	}
	details := plan.Task.Details
	details.SearchRunID = in.SearchRunID
	details.SourceSnapshotDigest = plan.SnapshotDigest
	details.ExpectedHash = plan.Torrent.InfoHash
	details.HashAliases = plan.Torrent.Aliases
	details.Message = "来源已明确确认，原任务等待下载器核对；媒体/路径/做种源不变"
	rawDetails, _ := json.Marshal(details)
	// A known hash is never replaced; initial identity is explicit and separate.
	if _, err = tx.Exec(ctx, `UPDATE download_tasks SET details=$2,source_control_revision=source_control_revision+1,source_recovery_required=false,source_resume_digest=$3,source_resume_previous_revision=$4,updated_at=now()WHERE id=$1`, id, rawDetails, in.PlanDigest, in.ControlRevision); err != nil {
		return answer, err
	}
	if _, err = tx.Exec(ctx, `UPDATE jobs SET state='pending',attempts=0,available_at=now(),last_error=NULL,lease_until=NULL,lease_token=NULL,locked_by=NULL,updated_at=now()WHERE id=$1 AND state IN('paused','failed')`, plan.Task.JobID); err != nil {
		return answer, err
	}
	audit, _ := json.Marshal(map[string]any{"taskId": id, "searchRunId": in.SearchRunID, "controlRevision": in.ControlRevision + 1, "mode": plan.Preview.Mode})
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','download.source_reconfirmed',$1)`, audit); err != nil {
		return answer, err
	}
	if err = tx.Commit(ctx); err != nil {
		return answer, err
	}
	answer.ControlRevision = in.ControlRevision + 1
	return answer, nil
}

func (e Engine) sourceReceipt(ctx context.Context, id string, in SourceReconfirmInput) (SourceReconfirmResult, bool, error) {
	result := SourceReconfirmResult{TaskID: id}
	var prior *int64
	var digest *string
	var revision int64
	var required bool
	var runID string
	if err := e.DB.QueryRow(ctx, `SELECT source_resume_previous_revision,source_resume_digest,source_control_revision,source_recovery_required,coalesce(details->>'searchRunId','') FROM download_tasks WHERE id=$1`, id).Scan(&prior, &digest, &revision, &required, &runID); err != nil {
		return result, false, ErrSourceRecoveryConflict
	}
	if prior == nil || digest == nil || *prior != in.ControlRevision || *digest != in.PlanDigest {
		return result, false, nil
	}
	if runID != in.SearchRunID {
		return result, false, ErrSourceRecoveryConflict
	}
	result.ControlRevision = revision
	result.AlreadyApplied = true
	result.RecoveryStillRequired = required
	return result, true, nil
}
