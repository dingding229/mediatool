package migrations_test

import "testing"

func TestB2AppendOnlySessionsAndControlledJobsPreserveLiveValues(t *testing.T) {
	f := newNativeFixture(t)
	if _, err := f.provider.UpTo(f.ctx, 71); err != nil {
		t.Fatal(err)
	}
	f.exec(t, `INSERT INTO administrator_accounts(id,username,password_hash)VALUES('11111111-0000-4000-8000-000000000072','b2-migration','owned-hash')`)
	f.exec(t, `INSERT INTO administrator_sessions(token_digest,account_id,credential_version,expires_at)VALUES('owned-digest','11111111-0000-4000-8000-000000000072',1,now()+interval '1 hour')`)
	if _, err := f.provider.UpTo(f.ctx, 73); err != nil {
		t.Fatal(err)
	}
	f.version(t, 73)
	if f.integer(t, `SELECT count(*) FROM administrator_sessions WHERE token_digest='owned-digest' AND id IS NOT NULL AND last_active_at=created_at`) != 1 {
		t.Fatal("existing session lost")
	}
	if f.integer(t, `SELECT count(*) FROM administrator_accounts WHERE password_hash='owned-hash'`) != 1 {
		t.Fatal("live password changed")
	}
	f.exec(t, `INSERT INTO jobs(id,kind,state)VALUES('22222222-0000-4000-8000-000000000073','download.add','unknown')`)
	if _, err := f.provider.Down(f.ctx); err != nil {
		t.Fatal(err)
	}
	if f.integer(t, `SELECT count(*) FROM jobs WHERE state='paused'`) != 1 {
		t.Fatal("downgrade discarded uncertain job")
	}
	if _, err := f.provider.UpTo(f.ctx, 73); err != nil {
		t.Fatal(err)
	}
	f.version(t, 73)
}
