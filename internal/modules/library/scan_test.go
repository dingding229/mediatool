package library

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

type scanCountRow struct {
	values []any
	err    error
}

func (r scanCountRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	for i := range dest {
		if p, ok := dest[i].(*int); ok {
			*p = r.values[i].(int)
		}
	}
	return nil
}

type scanCountDB struct {
	rows []pgx.Row
}

func (d *scanCountDB) QueryRow(context.Context, string, ...any) pgx.Row {
	r := d.rows[0]
	d.rows = d.rows[1:]
	return r
}

func TestScanCountsPropagatesDirectoryAndFileCountErrors(t *testing.T) {
	want := errors.New("count unavailable")
	for _, rows := range [][]pgx.Row{
		{scanCountRow{err: want}},
		{scanCountRow{values: []any{1}}, scanCountRow{err: want}},
	} {
		db := &scanCountDB{rows: rows}
		if _, _, err := scanCounts(context.Background(), db, "scan"); !errors.Is(err, want) {
			t.Fatalf("scan count error was lost: %v", err)
		}
	}
}
