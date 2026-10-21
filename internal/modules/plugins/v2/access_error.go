package v2

import (
	"context"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/jackc/pgx/v5"
)

type accessReader interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func accessError(ctx context.Context, db accessReader, id string) error {
	var desired, reason string
	if db.QueryRow(ctx, `SELECT desired_state,reason FROM plugin_v2_instances WHERE id=$1`, id).Scan(&desired, &reason) == nil {
		if reason == string(PermissionReview) {
			return contracts.AccessError("permission_review_required")
		}
		if desired != string(Enabled) {
			return contracts.AccessError("disabled")
		}
	}
	return contracts.AccessError("read_only")
}
