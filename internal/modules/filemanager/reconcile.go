package filemanager

import (
	"context"
	"errors"
	deferutil "media-tool/internal/platform/cleanup"
	"time"
)

// Reconcile only observes and repairs the SQL path journal. It never sends a
// second mutating RPC or replays an unknown filesystem operation.
func (e Engine) Reconcile(ctx context.Context, id string) (Result, error) {
	out, err := e.Get(ctx, id)
	if err != nil {
		return out, err
	}
	if out.State != "running" && out.State != "needs_reconcile" {
		return out, nil
	}
	root, err := e.root(ctx, out.Plan.ConnectionID)
	if err != nil || root.Origin != out.Plan.OriginFingerprint || root.Path != out.Plan.Root {
		return out, ErrChanged
	}
	unlock, err := e.lock(ctx, root.ID)
	if err != nil {
		return out, err
	}
	defer unlock()
	var updated time.Time
	if err = e.DB.QueryRow(ctx, `SELECT updated_at FROM file_operations WHERE id=$1`, id).Scan(&updated); err != nil {
		return out, ErrUnavailable
	}
	if out.State == "running" && time.Since(updated) < 2*time.Minute {
		return out, ErrBusy
	}
	b, err := e.backend(ctx, root)
	if err != nil {
		return out, err
	}
	defer deferutil.Close(b, "Read/descriptor teardown only; primary operations handle their own I/O result")
	completed := out.CompletedSteps
	for completed < len(out.Plan.Steps) {
		step := out.Plan.Steps[completed]
		target, targetErr := b.Find(ctx, step.Target)
		if step.Action == "mkdir" {
			if targetErr != nil || !target.Directory || target.Unsupported {
				break
			}
		} else {
			source, sourceErr := b.Find(ctx, step.Source.Path)
			if targetErr == nil && Same(step.Source, target) && errors.Is(sourceErr, ErrMissing) {
				// Confirmed same file at the expected destination: SQL repair only.
			} else if sourceErr == nil && Same(step.Source, source) && errors.Is(targetErr, ErrMissing) {
				break
			} else {
				return out, ErrChanged
			}
		}
		if err = e.recordStep(ctx, out.Plan, step, completed+1, target); err != nil {
			return out, ErrUnavailable
		}
		completed++
	}
	state, message := "succeeded", "已核对全部目标文件；没有重复执行重命名或移动"
	if completed != len(out.Plan.Steps) {
		state = "failed"
		message = "已核对当前路径；部分步骤未完成且未自动重放，请重新选择当前文件预览剩余操作"
	}
	if err = e.finish(ctx, id, state, message, completed); err != nil {
		return out, ErrUnavailable
	}
	return e.Get(ctx, id)
}
