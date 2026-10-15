package acquisition

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgconn"
	"testing"
)

type bindingExecer struct {
	tag  pgconn.CommandTag
	err  error
	args []any
}

func (e *bindingExecer) Exec(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
	e.args = args
	return e.tag, e.err
}
func TestSubscriptionTaskAssociationCannotSilentlyFail(t *testing.T) {
	want := errors.New("database unavailable")
	for _, tc := range []struct {
		tag            string
		err, errorWant error
	}{{"UPDATE 1", nil, nil}, {"UPDATE 0", nil, ErrDownloadIdentityConflict}, {"", want, want}} {
		db := &bindingExecer{tag: pgconn.NewCommandTag(tc.tag), err: tc.err}
		err := assignDownloadSubscription(context.Background(), db, "task", "subscription")
		if !errors.Is(err, tc.errorWant) {
			t.Fatalf("tag %s: got %v want %v", tc.tag, err, tc.errorWant)
		}
		if len(db.args) != 2 || db.args[0] != "task" || db.args[1] != "subscription" {
			t.Fatal("association identity lost", db.args)
		}
	}
}
