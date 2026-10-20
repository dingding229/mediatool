package acquisition

import "testing"

func TestPipelineFlowNeverSuggestsBlindReplay(t *testing.T) {
	for _, state := range []string{"starting", "uncertain", "uploading", "paused", "cancelled", "imported", "needs_output_reconciliation", "output_resume_approved", "future_unknown"} {
		f := TransferFlow(state, "failed")
		if f.Retryable || f.NextAction == "retry_current_phase" {
			t.Fatal(state, f)
		}
	}
	for _, state := range []string{"pending", "strm_ready", "refresh_requested"} {
		for _, job := range []string{"", "running", "pending", "succeeded", "paused", "cancelled", "failed"} {
			f := DownloadFlow(state, false, job)
			if f.Retryable != (job == "failed") || DownloadRetryKey(state, "owned") == "" {
				t.Fatal(state, job, f)
			}
			if f = DownloadFlow(state, true, job); f.Retryable || f.NextAction != "preview_source" {
				t.Fatal(f)
			}
		}
	}
	if f := DownloadFlow("local_complete", false, ""); f.Phase != "organize" {
		t.Fatal(f)
	}
	if f := DownloadFlow("imported", false, ""); f.Phase != "complete" || f.NextAction != "none" {
		t.Fatal(f)
	}
	for _, state := range []string{"verified", "artifacts_pending"} {
		if f := TransferFlow(state, "failed"); !f.Retryable || f.Phase != "strm" {
			t.Fatal(f)
		}
	}
}
