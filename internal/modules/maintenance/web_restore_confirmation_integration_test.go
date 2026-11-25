package maintenance

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWebRestoreQueueRejectsExpiredOrUnboundConfirmation(t *testing.T) {
	e := fixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	backupID := newID()
	const digest = "fixture-manifest-digest"
	if _, err := e.DB.Exec(ctx, `INSERT INTO ops_backups(id,state,digest) VALUES($1,'ready',$2)`, backupID, digest); err != nil {
		t.Fatal(err)
	}
	phrase := "恢复 " + backupID
	for _, tc := range []struct {
		name, submittedDigest, submittedPhrase, expiry string
		wantErr                                        error
	}{
		{"expired", digest, phrase, "-1 hour", ErrConflict},
		{"wrong digest", "unbound-digest", phrase, "1 hour", ErrUnsafe},
		{"wrong phrase", digest, "恢复 " + newID(), "1 hour", ErrUnsafe},
		{"valid confirmation", digest, phrase, "1 hour", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			restoreID := newID()
			if _, err := e.DB.Exec(ctx, `INSERT INTO ops_web_restores(id,backup_id,manifest_digest,protected,expires_at)
				VALUES($1,$2,$3,'{}',now()+$4::text::interval)`, restoreID, backupID, digest, tc.expiry); err != nil {
				t.Fatal(err)
			}
			if _, err := e.QueueWebRestore(ctx, restoreID, tc.submittedDigest, tc.submittedPhrase); !errors.Is(err, tc.wantErr) {
				t.Fatalf("queue error = %v, want %v", err, tc.wantErr)
			}
			got, err := e.WebRestore(ctx, restoreID)
			wantState := "preview"
			if tc.wantErr == nil {
				wantState = "pending"
			}
			if err != nil || got.State != wantState || got.SnapshotID != nil {
				t.Fatalf("receipt = %+v, err = %v; want %s with no snapshot", got, err, wantState)
			}
		})
	}
}
