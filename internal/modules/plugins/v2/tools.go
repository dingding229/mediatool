package v2

import (
	"context"
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	b "github.com/dingding229/mediatool-plugin/sdk/host/business"
	"time"
)

// ToolCall uses the existing durable invocation lane, not an autonomous executor.
func (s Store) ToolCall(ctx context.Context, actor, id, name, key string, input json.RawMessage, confirm bool) (output json.RawMessage, err error) {
	if len(actor) == 0 || len(actor) > 128 || len(key) < 8 || len(key) > 100 {
		return nil, ErrInvalid
	}
	snap, d, e := s.Descriptor(ctx, id, "tool", name)
	if e != nil {
		return nil, e
	}
	if !contracts.ValidTool(d) || !b.ValidToolEffect(d.EffectKind, d.RequiredScopes) || contracts.ValidateToolInput(d.InputSchema, input) != nil {
		return nil, ErrInvalid
	}
	if d.EffectKind != "read" && !confirm {
		return nil, contracts.NewError(contracts.PermissionDenied)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	dedupe := "manual-tool:" + actor + ":" + key
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer rollback(ctx, tx)
	run, e := s.submitTx(ctx, tx, snap, d, input, "invoke:"+id+":"+d.Handler+":"+dedupe, "", nil, time.Now().Add(5*time.Second))
	if e != nil {
		return nil, e
	}
	details, _ := json.Marshal(map[string]string{"instanceId": id, "tool": name, "runId": run, "effectKind": d.EffectKind})
	if _, e = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES($1,'plugins.tool.invoke',$2)`, actor, details); e != nil {
		return nil, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, e
	}
	started := time.Now()
	s.logInfo(ctx, id, "Plugin tool call started", "runId", run, "tool", d.Name, "effectKind", d.EffectKind)
	completed := false
	defer func() {
		result := "unknown"
		if completed {
			result = "succeeded"
		} else if err != nil {
			result = callLogResult(err)
		}
		s.logInfo(context.WithoutCancel(ctx), id, "Plugin tool call finished", "runId", run, "tool", d.Name, "effectKind", d.EffectKind, "result", result, "durationMs", time.Since(started).Milliseconds())
	}()
	out, e := s.invokeSnapshot(ctx, snap, d, input, dedupe)
	if e != nil {
		return nil, e
	}
	if len(out) > contracts.ToolOutputLimit || contracts.ValidateJSON(out) != nil {
		return nil, contracts.NewError(contracts.InvalidExtensionResult)
	}
	// Authorization is not cached for output, either; revoked in-flight calls cannot return data.
	latest, _, e := s.Descriptor(ctx, id, "tool", name)
	if e != nil || latest.ID != snap.ID {
		return nil, ErrConflict
	}
	completed = true
	return out, nil
}
