package migrations_test

import "testing"

func TestLogPolicyMigrationUpDownUp(t *testing.T) {
	f := newNativeFixture(t)
	if _, e := f.provider.UpTo(f.ctx, 77); e != nil {
		t.Fatal(e)
	}
	f.exec(t, `UPDATE application_settings SET value=jsonb_build_object('movieTemplate','preserved','unrelated','kept')`)
	for range 2 {
		if _, e := f.provider.UpTo(f.ctx, 78); e != nil {
			t.Fatal(e)
		}
		if got := f.text(t, `SELECT value->'logging'->>'retentionDays' FROM application_settings`); got != "7" {
			t.Fatal(got)
		}
		if f.text(t, `SELECT value->>'unrelated' FROM application_settings`) != "kept" {
			t.Fatal("unrelated setting modified")
		}
		if _, e := f.provider.DownTo(f.ctx, 77); e != nil {
			t.Fatal(e)
		}
		if f.text(t, `SELECT (value ? 'logging')::text FROM application_settings`) != "false" {
			t.Fatal("down failed")
		}
	}
	if _, e := f.provider.UpTo(f.ctx, 78); e != nil {
		t.Fatal(e)
	}
}
