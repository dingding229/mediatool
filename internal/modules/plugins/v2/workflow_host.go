package v2

import (
	"context"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	dbutil "media-tool/internal/platform/database"
	"slices"
	"strings"
)

func (s Store) bridgeCall(ctx context.Context, call contracts.HostCall, p contracts.Permission, admit func() error) (any, error) {
	if !strings.HasPrefix(call.Method, "workflows.") {
		return s.Business.Call(ctx, call, p, admit)
	}
	var q contracts.WorkflowRequest
	if contracts.Decode(call.Args, &q) != nil || !slices.Contains(p.Resources["workflowIds"], q.WorkflowID) {
		return nil, contracts.NewError(contracts.PermissionDenied)
	}
	if e := admit(); e != nil {
		return nil, e
	}
	runner := WorkflowRunner{s}
	switch call.Method {
	case "workflows.run":
		return runner.Start(ctx, "plugin", q.WorkflowID, call.OperationKey)
	case "workflows.status":
		v, e := runner.Status(ctx, q.WorkflowID, q.RunID)
		if e == nil {
			e = dbutil.CheckPrepared(ctx, s.DB)
		}
		return v, e
	case "workflows.cancel":
		return runner.Cancel(ctx, "plugin", q.WorkflowID, q.RunID)
	}
	return nil, contracts.NewError(contracts.Unsupported)
}
