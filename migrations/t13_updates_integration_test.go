package migrations_test

import "testing"

func TestT13UpdateSettingsUpDownUp(t *testing.T) {
	f := newNativeFixture(t)
	if _, e := f.provider.UpTo(f.ctx, 86); e != nil {
		t.Fatal(e)
	}
	f.exec(t, `INSERT INTO app_settings(key,value) VALUES('online_update.execution','true'),('online_update.recovery.test-latch','true'),('system.update-check','{"enabled":false,"result":{"status":"no_release"}}'),('t13-unrelated-secret','"synthetic-encrypted-value"')`)
	for i := 0; i < 2; i++ {
		if _, e := f.provider.UpTo(f.ctx, 87); e != nil {
			t.Fatal(e)
		}
		if n := f.integer(t, `SELECT count(*) FROM app_settings WHERE key='online_update.execution'`); n != 0 {
			t.Fatal(n)
		}
		if n := f.integer(t, `SELECT count(*) FROM app_settings WHERE key='system.update-check' AND value->>'intervalHours'='12' AND value->>'enabled'='false' AND value#>>'{result,status}'='no_release'`); n != 1 {
			t.Fatal(n)
		}
		if n := f.integer(t, `SELECT count(*) FROM app_settings WHERE (key='online_update.recovery.test-latch' AND value='true') OR (key='t13-unrelated-secret' AND value='"synthetic-encrypted-value"')`); n != 2 {
			t.Fatal("unrelated settings changed", n)
		}
		if i == 0 {
			if _, e := f.provider.Down(f.ctx); e != nil {
				t.Fatal(e)
			}
			if n := f.integer(t, `SELECT count(*) FROM app_settings WHERE key='online_update.execution' AND value='false'`); n != 1 {
				t.Fatal("Down must default closed", n)
			}
		}
	}
}
