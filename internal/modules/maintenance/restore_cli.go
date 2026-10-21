package maintenance

import (
	"context"
	"fmt"
	"time"
)

// HTTP and owner CLI share admission, pre-snapshot, import, automatic rollback,
// commit reconciliation and retention. 202 remains an HTTP queue receipt only.
func (e Engine) WaitWebRestore(ctx context.Context, id string) (WebRestore, error) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		r, err := e.WebRestore(ctx, id)
		if err != nil {
			return r, err
		}
		switch r.State {
		case "completed":
			return r, nil
		case "failed", "rolled_back", "unknown":
			return r, fmt.Errorf("恢复状态%s：%s", r.State, r.Message)
		}
		if err = e.RunWebRestore(ctx); err != nil {
			return r, err
		}
		select {
		case <-ctx.Done():
			return r, ctx.Err()
		case <-ticker.C:
		}
	}
}
func (e Engine) RestoreApprovedRequest(ctx context.Context, id string) (WebRestore, error) {
	if existing, err := e.WebRestore(ctx, id); err == nil {
		return e.WaitWebRestore(ctx, existing.ID)
	}
	req, err := e.CheckRequest(ctx, id)
	if err != nil {
		return WebRestore{}, err
	}
	if req.Kind != "restore" || req.State != "approved" {
		return WebRestore{}, ErrUnsafe
	}
	r, err := e.previewWebRestore(ctx, req.Payload.BackupID, id)
	if err != nil {
		return r, err
	}
	r, err = e.QueueWebRestore(ctx, r.ID, r.Digest, "恢复 "+r.BackupID)
	if err != nil {
		return r, err
	}
	r, err = e.WaitWebRestore(ctx, r.ID)
	state := "completed"
	if err != nil {
		state = "failed"
	}
	_, recordErr := e.DB.Exec(context.WithoutCancel(ctx), `UPDATE ops_requests SET state=$2,message=$3,completed_at=now() WHERE id=$1`, id, state, r.Message)
	if err != nil {
		return r, err
	}
	return r, recordErr
}
