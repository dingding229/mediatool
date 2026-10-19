package acquisition

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"math"
	"media-tool/internal/adapters/qbittorrent"
	dbutil "media-tool/internal/platform/database"
	"media-tool/internal/platform/pluginactor"
	"slices"
	"strings"
	"unicode"
)

type Assistance struct {
	ID, Tag, Category          string
	DownloadLimit, UploadLimit *int64
	RatioLimit                 *float64
	SeedingMinutes             *int64
}

func labelOK(s string) bool {
	if len(s) > 64 {
		return false
	}
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-' && r != ' ' {
			return false
		}
	}
	return true
}
func validAssistance(v Assistance) bool {
	if !labelOK(v.Tag) || !labelOK(v.Category) || v.Tag == "media-tool" || len(v.Tag) == 36 {
		return false
	}
	for _, n := range []*int64{v.DownloadLimit, v.UploadLimit} {
		if n != nil && (*n < 1 || *n > 100<<20) {
			return false
		}
	}
	if v.RatioLimit != nil && (math.IsNaN(*v.RatioLimit) || math.IsInf(*v.RatioLimit, 0) || *v.RatioLimit < .1 || *v.RatioLimit > 1000) {
		return false
	}
	if v.SeedingMinutes != nil && (*v.SeedingMinutes < 60 || *v.SeedingMinutes > 87600*60) {
		return false
	}
	return v.Tag != "" || v.Category != "" || v.DownloadLimit != nil || v.UploadLimit != nil || v.RatioLimit != nil || v.SeedingMinutes != nil
}
func (s DownloaderService) assistLock(ctx context.Context, id string) (*pgxpool.Conn, error) {
	c, e := s.Engine.DB.Acquire(ctx)
	if e != nil {
		return nil, e
	}
	var ok bool
	e = c.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended($1,0))`, "download-task:"+id).Scan(&ok)
	if e != nil || !ok {
		c.Release()
		return nil, ErrCleanupPrerequisites
	}
	return c, nil
}
func (s DownloaderService) assistSafe(ctx context.Context, id, hash, owner string, a *qbittorrent.Adapter) ([]string, int64, error) {
	tags, _, e := a.CleanupInfo(ctx, hash)
	if e != nil {
		return nil, 0, e
	}
	parts := []string{}
	for _, v := range strings.Split(tags, ",") {
		v = strings.TrimSpace(v)
		if v != "" {
			parts = append(parts, v)
		}
	}
	check := []string{}
	for _, v := range parts {
		if v != "plugin:"+owner {
			check = append(check, v)
		}
	}
	if len(parts) > 200 || protectedCleanupTags(strings.Join(check, ","), id) {
		return nil, 0, ErrCleanupPrerequisites
	}
	var rev int64
	var unsafe bool
	e = s.Engine.DB.QueryRow(ctx, `SELECT source_control_revision,details?'pluginAssistantIntent' OR source_recovery_required OR seeding_state='removed' OR EXISTS(SELECT 1 FROM download_tasks WHERE id<>$1 AND info_hash=$2) OR EXISTS(SELECT 1 FROM transfer_tasks t JOIN download_files f ON f.id=t.download_file_id WHERE f.task_id=$1 AND t.state<>'imported') OR EXISTS(SELECT 1 FROM download_cleanup_requests WHERE task_id=$1)FROM download_tasks WHERE id=$1`, id, hash).Scan(&rev, &unsafe)
	if e != nil {
		return nil, 0, e
	}
	if unsafe {
		return nil, 0, ErrCleanupPrerequisites
	}
	return parts, rev, nil
}
func (s DownloaderService) noHR(ctx context.Context, id string) bool {
	var raw []byte
	e := s.Engine.DB.QueryRow(ctx, `SELECT r.public_snapshot FROM download_tasks d JOIN search_run_resources r ON r.run_id::text=d.details->>'searchRunId' AND r.resource_id=d.resource_id WHERE d.id=$1`, id).Scan(&raw)
	if e != nil {
		return false
	}
	var v struct {
		Metadata map[string]any `json:"metadata"`
	}
	if json.Unmarshal(raw, &v) != nil {
		return false
	}
	hr, known := v.Metadata["hr"].(bool)
	return known && !hr
}
func (s DownloaderService) Configure(ctx context.Context, v Assistance, admit func() error) error {
	owner := pluginactor.InstanceID(ctx)
	if owner == "" || !validAssistance(v) {
		return ErrCleanupPrerequisites
	}
	id, _, _, e := s.owned(ctx, v.ID)
	if e != nil {
		return e
	}
	conn, e := s.assistLock(ctx, id)
	if e != nil {
		return e
	}
	defer conn.Release()
	defer dbutil.Unlock(context.Background(), conn, `SELECT pg_advisory_unlock(hashtextextended($1,0))`, "download-task:"+id)
	_, a, t, e := s.owned(ctx, id)
	if e != nil {
		return e
	}
	tags, rev, e := s.assistSafe(ctx, id, t.ID, owner, a)
	if e != nil {
		return e
	}
	if v.Category != "" && (t.AutoManaged == nil || *t.AutoManaged || !a.CategorySafe(ctx, v.Category, t.SavePath)) {
		return ErrCleanupPrerequisites
	}
	// Unknown effective policy is never reset to unlimited. All existing dimensions
	// are preserved; positive existing limits may only be tightened.
	for _, p := range []struct{ new, old *int64 }{{v.DownloadLimit, t.DownloadLimit}, {v.UploadLimit, t.UploadLimit}} {
		if p.new != nil && (p.old == nil || *p.old > 0 && *p.new > *p.old) {
			return ErrCleanupPrerequisites
		}
	}
	share := v.RatioLimit != nil || v.SeedingMinutes != nil
	if share {
		if !s.noHR(ctx, id) || t.RatioLimit == nil || t.SeedingMinutes == nil || t.InactiveMinutes == nil {
			return ErrCleanupPrerequisites
		}
		ratio, minutes := t.RatioLimit, t.SeedingMinutes
		if *ratio == -2 {
			ratio = t.EffectiveRatioLimit
		}
		if *minutes == -2 {
			minutes = t.EffectiveSeedingMinutes
		}
		if ratio == nil || minutes == nil {
			return ErrCleanupPrerequisites
		}
		if v.RatioLimit != nil && *ratio > 0 && *v.RatioLimit > *ratio || v.SeedingMinutes != nil && *minutes > 0 && *v.SeedingMinutes > *minutes {
			return ErrCleanupPrerequisites
		}
		if v.RatioLimit != nil && *t.RatioLimit > 0 && *v.RatioLimit > *t.RatioLimit {
			return ErrCleanupPrerequisites
		}
		if v.SeedingMinutes != nil && *t.SeedingMinutes > 0 && *v.SeedingMinutes > *t.SeedingMinutes {
			return ErrCleanupPrerequisites
		}
		if v.RatioLimit == nil {
			v.RatioLimit = t.RatioLimit
		}
		if v.SeedingMinutes == nil {
			v.SeedingMinutes = t.SeedingMinutes
		}
	}
	add := []string{}
	for _, tag := range []string{v.Tag, "plugin:" + owner} {
		if tag != "" && !slices.Contains(tags, tag) {
			add = append(add, tag)
		}
	}
	if e = admit(); e != nil {
		return e
	}
	intent, e := json.Marshal(struct {
		Owner   string     `json:"owner"`
		Request Assistance `json:"request"`
	}{owner, v})
	if e != nil {
		return e
	}
	tag, e := dbutil.Exec(ctx, s.Engine.DB, `UPDATE download_tasks SET details=details||jsonb_build_object('pluginAssistantIntent',$2::jsonb),updated_at=now() WHERE id=$1 AND source_control_revision=$3 AND NOT(details?'pluginAssistantIntent')`, id, intent, rev)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return ErrCleanupPrerequisites
	}
	inactive := int64(-2)
	if t.InactiveMinutes != nil {
		inactive = *t.InactiveMinutes
	}
	if e = a.ApplyAssistance(ctx, t.ID, qbittorrent.Assistance{Tag: strings.Join(add, ","), Category: v.Category, DownloadLimit: v.DownloadLimit, UploadLimit: v.UploadLimit, RatioLimit: v.RatioLimit, SeedingMinutes: v.SeedingMinutes, InactiveMinutes: inactive}, admit); e != nil {
		return e
	}
	actual, e := a.Inspect(ctx, t.ID)
	if e != nil || actual.SavePath != t.SavePath || actual.ID != t.ID {
		return ErrCleanupPrerequisites
	}
	after, save, e := a.CleanupInfo(ctx, t.ID)
	if e != nil || save != t.SavePath || !ownsTag(after, id) {
		return ErrCleanupPrerequisites
	}
	for _, tag := range add {
		if !ownsTag(after, tag) {
			return ErrCleanupPrerequisites
		}
	}
	if v.Category != "" && actual.Category != v.Category {
		return ErrCleanupPrerequisites
	}
	for _, p := range []struct{ want, got *int64 }{{v.DownloadLimit, actual.DownloadLimit}, {v.UploadLimit, actual.UploadLimit}, {v.SeedingMinutes, actual.SeedingMinutes}} {
		if p.want != nil && (p.got == nil || *p.want != *p.got) {
			return ErrCleanupPrerequisites
		}
	}
	if v.RatioLimit != nil && (actual.RatioLimit == nil || *v.RatioLimit != *actual.RatioLimit) {
		return ErrCleanupPrerequisites
	}
	tag, e = dbutil.Exec(ctx, s.Engine.DB, `UPDATE download_tasks SET details=jsonb_set(details-'pluginAssistantIntent','{category}',to_jsonb($2::text)),source_control_revision=source_control_revision+1,updated_at=now()WHERE id=$1 AND source_control_revision=$3 AND details->'pluginAssistantIntent'->>'owner'=$4`, id, actual.Category, rev, owner)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return ErrCleanupPrerequisites
	}
	return nil
}
func (s DownloaderService) Recheck(ctx context.Context, id string, admit func() error) error {
	owner := pluginactor.InstanceID(ctx)
	if owner == "" {
		return errors.New("host identity required")
	}
	conn, e := s.assistLock(ctx, id)
	if e != nil {
		return e
	}
	defer conn.Release()
	defer dbutil.Unlock(context.Background(), conn, `SELECT pg_advisory_unlock(hashtextextended($1,0))`, "download-task:"+id)
	_, a, t, e := s.owned(ctx, id)
	if e != nil {
		return e
	}
	if _, _, e = s.assistSafe(ctx, id, t.ID, owner, a); e != nil {
		return e
	}
	if e = admit(); e != nil {
		return e
	}
	if e = a.RecheckOwned(ctx, t.ID); e != nil {
		return e
	}
	got, e := a.Inspect(ctx, t.ID)
	if e != nil {
		return e
	}
	switch got.State {
	case "checkingDL", "checkingUP", "queuedForChecking":
		return nil
	}
	return ErrCleanupPrerequisites
}
