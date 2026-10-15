package playeraccess

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"strings"
	"testing"
	"time"
)

type snapshotRow struct {
	raw string
	err error
}

func (r snapshotRow) Scan(out ...any) error {
	if r.err != nil {
		return r.err
	}
	*out[0].(*bool) = true
	*out[1].(*int64) = 8
	*out[2].(*time.Time) = time.Now()
	*out[3].(*[]byte) = []byte(r.raw)
	return nil
}

type snapshotReader struct {
	calls int
	row   snapshotRow
	t     *testing.T
}

func (r *snapshotReader) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	r.calls++
	if strings.Contains(sql, "advisory") {
		r.t.Fatal("snapshot acquired global lock")
	}
	return r.row
}
func TestPlaybackPolicyUsesOneRevisionedSnapshot(t *testing.T) {
	q := &snapshotReader{t: t, row: snapshotRow{raw: `[{"id":"owned-rule","label":"TV","matchType":"prefix","userAgent":"SenPlayer","enabled":true}]`}}
	p, err := playbackPolicySnapshot(context.Background(), q)
	if err != nil || q.calls != 1 || p.Revision != 8 || len(p.Rules) != 1 || !p.Decide("SenPlayer/6").Allowed {
		t.Fatal(p, err, q.calls)
	}
	for _, row := range []snapshotRow{{err: errors.New("unavailable")}, {raw: "not-json"}} {
		q.row = row
		if _, err = playbackPolicySnapshot(context.Background(), q); !errors.Is(err, ErrUnavailable) {
			t.Fatal("unsafe snapshot accepted", err)
		}
	}
}
