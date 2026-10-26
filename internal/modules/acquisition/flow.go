package acquisition

import (
	"fmt"
	"media-tool/internal/platform/flow"
	"strings"
)

type downloadStage struct{ state, phase, prefix string }

var downloadRetryStages = []downloadStage{{"pending", "download", "download.add:"}}

func retryStage(state string) (downloadStage, bool) {
	for _, v := range downloadRetryStages {
		if v.state == state {
			return v, true
		}
	}
	return downloadStage{}, false
}
func DownloadRetryKey(state, id string) string {
	if v, ok := retryStage(state); ok {
		return v.prefix + id
	}
	return ""
}

// Built only from the private stage catalog, never from request strings.
var downloadRetryCase = func() string {
	parts := []string{"CASE d.download_state"}
	for _, v := range downloadRetryStages {
		parts = append(parts, fmt.Sprintf(" WHEN '%s' THEN '%s'||d.id::text", v.state, v.prefix))
	}
	return strings.Join(parts, "") + " END"
}()

func retryFlow(phase, job string) flow.Status {
	out := flow.Status{Phase: phase, NextAction: "wait", Reason: "job_active"}
	switch job {
	case "failed":
		out.Retryable = true
		out.NextAction = "retry_current_phase"
		out.Reason = "job_failed"
	case "":
		out.NextAction = "inspect_task"
		out.Reason = "job_missing"
	case "succeeded":
		out.NextAction = "refresh_status"
		out.Reason = "job_finished"
	case "paused":
		out.NextAction = "resume"
		out.Reason = "job_paused"
	case "cancelled":
		out.NextAction = "inspect_task"
		out.Reason = "job_cancelled"
	}
	return out
}
func DownloadFlow(state string, sourceRequired bool, job string) flow.Status {
	if sourceRequired {
		return flow.Status{Phase: "download", NextAction: "preview_source", Reason: "source_reconfirm_required"}
	}
	if v, ok := retryStage(state); ok {
		return retryFlow(v.phase, job)
	}
	switch state {
	case "strm_ready", "refresh_requested":
		return flow.Status{Phase: "inspect", NextAction: "inspect_task", Reason: "import_evidence_incomplete"}
	case "imported":
		return flow.Status{Phase: "complete", NextAction: "none", Reason: "artifacts_applied"}
	case "local_complete":
		return flow.Status{Phase: "organize", NextAction: "inspect_transfers", Reason: "download_complete"}
	case "downloading", "added":
		return flow.Status{Phase: "download", NextAction: "wait", Reason: "downloader_active"}
	default:
		return flow.Status{Phase: "download", NextAction: "inspect_task", Reason: "state_requires_inspection"}
	}
}
func TransferFlow(state, job string) flow.Status {
	switch state {
	case "pending":
		return retryFlow("organize", job)
	case "verified", "artifacts_pending":
		return retryFlow("strm", job)
	case "needs_output_reconciliation", "output_resume_approved":
		return flow.Status{Phase: "strm", NextAction: "preview_output", Reason: "output_reconciliation_required"}
	case "starting", "uploading", "uncertain":
		return flow.Status{Phase: "organize", NextAction: "reconcile_upload", Reason: "upload_outcome_unknown"}
	case "paused":
		return flow.Status{Phase: "organize", NextAction: "resume", Reason: "upload_paused"}
	case "imported":
		return flow.Status{Phase: "complete", NextAction: "none", Reason: "artifacts_applied"}
	default:
		return flow.Status{Phase: "organize", NextAction: "inspect_task", Reason: "state_requires_inspection"}
	}
}
