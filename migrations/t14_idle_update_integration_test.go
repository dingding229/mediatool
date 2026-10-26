package migrations_test

import "testing"

func TestT14IdleUpdateUpDownUp(t *testing.T) {
	f := newNativeFixture(t)
	if _, e := f.provider.UpTo(f.ctx, 87); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		if _, e := f.provider.UpTo(f.ctx, 88); e != nil {
			t.Fatal(e)
		}
		if n := f.integer(t, `SELECT count(*)FROM idle_update_policy WHERE NOT enabled AND quiet_minutes=60`); n != 1 {
			t.Fatal(n)
		}
		if i == 0 {
			f.exec(t, `UPDATE idle_update_policy SET enabled=true;INSERT INTO idle_update_attempts(id,version,local_day,timezone,grant_id,state)VALUES('fixture','0.41.0',current_date,'Asia/Shanghai','grant','failed')`)
			if _, e := f.provider.Down(f.ctx); e != nil {
				t.Fatal(e)
			}
		}
	}
}
