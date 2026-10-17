package linksource

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestReservedFetchReusesActualAdmissionAndReleasesOnce(t *testing.T) {
	f := setup(t)
	ctx, close, err := Reserve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer close()
	blocked, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, _, err := Reserve(blocked); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("reservation bypassed global cap", err)
	}
	ticket, err := f.s.Fetch(ctx, asset)
	if err != nil {
		t.Fatal(err)
	}
	_, ua, err := ticket.Claim()
	if err != nil || ua != probeUA {
		t.Fatal(err)
	}
	close()
	close()
	if _, err := f.s.Fetch(ctx, asset); err == nil {
		t.Fatal("reservation consumed twice")
	}
	if err := ConfigureConcurrency(1); err != nil {
		t.Fatal("reservation leaked", err)
	}
}
func TestEarlyReservationCloseDoesNotReleaseActiveFetch(t *testing.T) {
	setup(t)
	ctx, close, err := Reserve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	end, err := takeReservation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	close()
	if ConfigureConcurrency(2) == nil {
		t.Fatal("active reservation closed early")
	}
	end()
	if err := ConfigureConcurrency(1); err != nil {
		t.Fatal(err)
	}
}
