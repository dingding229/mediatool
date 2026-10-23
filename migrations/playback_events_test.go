package migrations_test

import "testing"

func TestF7PlaybackEventsMigrationUpDownUp(t *testing.T) {
	f := newNativeFixture(t)
	if _, e := f.provider.UpTo(f.ctx, 78); e != nil {
		t.Fatal(e)
	}
	f.exec(t, `UPDATE application_settings SET value=value||'{"unrelated":"kept"}'::jsonb`)
	for range 2 {
		if _, e := f.provider.UpTo(f.ctx, 79); e != nil {
			t.Fatal(e)
		}
		if f.text(t, `SELECT value->>'playbackRetentionDays' FROM application_settings`) != "180" {
			t.Fatal("retention default")
		}
		if f.text(t, `SELECT value->>'unrelated' FROM application_settings`) != "kept" {
			t.Fatal("unrelated settings modified")
		}
		if n := f.integer(t, `SELECT count(*) FROM media_playback_events`); n != 0 {
			t.Fatal("legacy history was fabricated")
		}
		if _, e := f.provider.DownTo(f.ctx, 78); e != nil {
			t.Fatal(e)
		}
		if f.text(t, `SELECT (value?'playbackRetentionDays')::text FROM application_settings`) != "false" {
			t.Fatal("down failed")
		}
	}
	if _, e := f.provider.UpTo(f.ctx, 79); e != nil {
		t.Fatal(e)
	}
}
