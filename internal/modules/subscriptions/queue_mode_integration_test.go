package subscriptions

import (
	"errors"
	"testing"
)

func TestQueueRejectsReusingDifferentRunMode(t *testing.T) {
	f := retryFixtureFor(t)
	if _, err := f.engine.Queue(f.ctx, f.record.ID, true); !errors.Is(err, ErrRunModeConflict) {
		t.Fatal("active execution was reused for preview", err)
	}
	if id, err := f.engine.Queue(f.ctx, f.record.ID, false); err != nil || id != f.runID {
		t.Fatal(id, err)
	}
	if _, err := f.engine.DB.Exec(f.ctx, `UPDATE subscription_runs SET preview=true WHERE id=$1`, f.runID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.engine.Queue(f.ctx, f.record.ID, false); !errors.Is(err, ErrRunModeConflict) {
		t.Fatal("active preview was reused for execution", err)
	}
}
