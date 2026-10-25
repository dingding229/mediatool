package maintenance

import (
	"context"
	"testing"
)

func TestT9FinalBackupDueSlotThenNull(t *testing.T) {
	e := fixture(t)
	ctx := context.Background()
	_, err := e.DB.Exec(ctx, `UPDATE ops_policy SET value='{"backupEnabled":true,"backupCron":"0 0 0 1 1 * 2020"}',next_backup_at=now()-interval '1 minute',next_sample_at=now()+interval '1 day'`)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	r, err := e.Policy(ctx)
	if err != nil || r.NextBackup != nil || !r.Value.BackupEnabled {
		t.Fatal(r, err)
	}
	var before, after int
	e.DB.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE kind='ops.backup'`).Scan(&before)
	if before != 1 {
		t.Fatal(before)
	}
	for i := 0; i < 3; i++ {
		if err = e.Schedule(ctx); err != nil {
			t.Fatal(err)
		}
	}
	e.DB.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE kind='ops.backup'`).Scan(&after)
	if after != before {
		t.Fatal(before, after)
	}
}
