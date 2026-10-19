package v2

import (
	"bytes"
	"context"
	"errors"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"log/slog"
	"strings"
	"testing"
)

func TestLogResultsAreFixedCodesAndMetadataOnly(t *testing.T) {
	for _, v := range []struct {
		err  error
		want string
	}{{context.DeadlineExceeded, "unknown"}, {contracts.NewError(contracts.PermissionDenied), "denied"}, {errors.New("secret-sentinel"), "failed"}} {
		if got := callLogResult(v.err); got != v.want {
			t.Fatal(got)
		}
	}
	var output bytes.Buffer
	s := Store{Logger: slog.New(slog.NewJSONHandler(&output, nil))}
	s.logInfo(context.Background(), "instance-one", "Plugin tool call finished", "runId", "run-one", "result", callLogResult(errors.New("secret-sentinel")))
	if strings.Contains(output.String(), "secret-sentinel") || !strings.Contains(output.String(), `"instance":"instance-one"`) || !strings.Contains(output.String(), `"module":"plugins"`) {
		t.Fatal(output.String())
	}
}
func TestOperatorActionCASPreservedThroughReapAndExplicitEnable(t *testing.T) {
	s, l := activeFixture(t)
	ctx := context.Background()
	stale := l
	stale.GrantRevision++
	if e := s.pauseForAction(ctx, stale); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
	if e := s.pauseForAction(ctx, l); e != nil {
		t.Fatal(e)
	}
	if e := s.Finish(ctx, l, WorkerFailed); e != nil {
		t.Fatal(e)
	}
	var state, desired string
	var revision int64
	if e := s.DB.QueryRow(ctx, `SELECT runtime_state,desired_state,revision FROM plugin_v2_instances WHERE id=$1`, l.ID).Scan(&state, &desired, &revision); e != nil || state != "needs_action" || desired != "disabled" {
		t.Fatal(state, desired, e)
	}
	if _, e := s.Apply(ctx, "admin", Request{Kind: Enable, InstanceID: l.ID, Fingerprint: l.Fingerprint, ExpectedRevision: revision, OperationKey: "explicit-resume-after-action", Confirm: true}); e != nil {
		t.Fatal(e)
	}
}
