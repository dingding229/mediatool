package storage

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

type diagnosticFixture struct{}

func (diagnosticFixture) Error() string {
	return "PRIVATE-TOKEN https://provider.test/?PRIVATE-SIGNATURE"
}
func (diagnosticFixture) SafeDiagnostic() (int64, int) { return 40140117, 429 }
func TestResolveObservationIsFixedEnumsAndNumbersOnly(t *testing.T) {
	var events []ResolveObservation
	ctx := WithResolveObserver(context.Background(), func(e ResolveObservation) { events = append(events, e) })
	ObserveResolve(ctx, StageOfficialDownload, time.Now(), diagnosticFixture{})
	ObserveResolve(ctx, ResolveStage("PRIVATE-PATH"), time.Now(), nil)
	if len(events) != 1 || events[0].Outcome != "rate_limited" || events[0].ProviderCode != 40140117 || events[0].HTTPStatus != 429 {
		t.Fatal(events)
	}
	raw, _ := json.Marshal(events)
	if strings.Contains(string(raw), "PRIVATE") || strings.Contains(string(raw), "https:") {
		t.Fatal("secret in observation")
	}
	for _, test := range []struct {
		err  error
		want string
	}{{context.DeadlineExceeded, "timeout"}, {errors.Join(ErrUnavailable, ErrRequestTimeout), "timeout"}, {context.Canceled, "cancelled"}, {ErrInvalidResponse, "invalid_response"}, {ErrForbidden, "forbidden"}, {errors.New("PRIVATE-SIGNED-URL"), "failed"}} {
		got, _, _ := ResolveFailure(test.err)
		if got != test.want {
			t.Fatal(got, test.want)
		}
	}
}
