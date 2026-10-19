package acquisition

import (
	"context"
	"errors"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"media-tool/internal/modules/downloads"
	"strings"
)

// CleanupObservation is a credential-free projection. The path and raw adapter
// never leave the host. Eligibility is only a preview and is rechecked on delete.
type CleanupObservation struct {
	Task            downloads.Task
	ID, SiteID      string
	Tags            []string
	Owned, Eligible bool
}

func (s DownloaderService) ObserveCleanup(ctx context.Context, hash string) (CleanupObservation, error) {
	t, err := s.Inspect(ctx, hash)
	if err != nil {
		return CleanupObservation{}, err
	}
	out := CleanupObservation{Task: t, Tags: []string{}}
	id, a, _, err := s.owned(ctx, hash)
	if err != nil {
		return out, nil
	} // foreign/unknown is read-only
	out.Owned = true
	out.ID = id
	tags, _, err := a.CleanupInfo(ctx, hash)
	if err != nil {
		return out, err
	}
	for _, tag := range strings.Split(tags, ",") {
		tag = strings.TrimSpace(tag)
		if len(tag) > 200 || len(out.Tags) >= 200 {
			return out, errors.New("tag projection exceeds bound")
		}
		if tag != "" {
			out.Tags = append(out.Tags, tag)
		}
	}
	if err = s.Engine.DB.QueryRow(ctx, `SELECT coalesce(r.account_id::text,'') FROM download_tasks d LEFT JOIN resources r ON r.id=d.resource_id WHERE d.id=$1`, id).Scan(&out.SiteID); err != nil {
		return out, err
	}
	_, _, out.Eligible, err = s.PreviewCleanup(ctx, id)
	return out, err
}

// Protected tags are conservative vetoes, not positive ownership evidence.
// A different task UUID or plugin/rule marker must never be overridden by a rule.
func protectedCleanupTags(tags, id string) bool {
	for _, tag := range strings.Split(tags, ",") {
		tag = strings.TrimSpace(tag)
		lower := strings.ToLower(tag)
		if tag == id {
			continue
		}
		if contracts.OpaqueID(tag) || lower == "keep" || lower == "protect" || lower == "no-cleanup" || strings.HasPrefix(lower, "plugin:") || strings.HasPrefix(lower, "rule:") {
			return true
		}
	}
	return false
}
func (s DownloaderService) cleanupProtected(ctx context.Context, id, hash string) (bool, error) {
	_, a, _, err := s.owned(ctx, id)
	if err != nil {
		return true, err
	}
	tags, _, err := a.CleanupInfo(ctx, hash)
	if err != nil {
		return true, err
	}
	if protectedCleanupTags(tags, id) {
		return true, nil
	}
	var unsafe bool
	err = s.Engine.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM download_tasks WHERE id=$1 AND seeding_state='removed') OR EXISTS(SELECT 1 FROM download_tasks WHERE id<>$1 AND info_hash=$2) OR EXISTS(SELECT 1 FROM transfer_tasks t JOIN download_files f ON f.id=t.download_file_id WHERE f.task_id=$1 AND t.state<>'imported') OR EXISTS(SELECT 1 FROM download_cleanup_requests WHERE task_id=$1)`, id, hash).Scan(&unsafe)
	return unsafe, err
}
