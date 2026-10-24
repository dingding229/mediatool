package migrations_test

import "testing"

func TestOfflineIPMigrationUpDownUpOnlyRemovesOwnSettings(t *testing.T) {
	f := newNativeFixture(t)
	if _, e := f.provider.UpTo(f.ctx, 79); e != nil {
		t.Fatal(e)
	}
	f.exec(t, `INSERT INTO integrations(id,kind,name,enabled,credentials_ciphertext)VALUES('aaaaaaaa-0000-4000-8000-000000000080','tmdb','H2a unrelated credential',false,decode('aabbcc','hex'))`)
	f.exec(t, `INSERT INTO ip_geo_settings(encrypted_credentials)VALUES(decode('010203','hex'))`)
	for pass := 0; pass < 2; pass++ {
		if _, e := f.provider.UpTo(f.ctx, 80); e != nil {
			t.Fatal(e)
		}
		if f.text(t, `SELECT (to_regclass('ip_geo_settings') IS NULL)::text`) != "true" {
			t.Fatal("obsolete key storage remains")
		}
		if f.text(t, `SELECT encode(credentials_ciphertext,'hex') FROM integrations WHERE id='aaaaaaaa-0000-4000-8000-000000000080'`) != "aabbcc" {
			t.Fatal("unrelated credentials changed")
		}
		if pass == 0 {
			if _, e := f.provider.DownTo(f.ctx, 79); e != nil {
				t.Fatal(e)
			}
			if f.integer(t, `SELECT count(*) FROM ip_geo_settings`) != 0 {
				t.Fatal("removed keys were resurrected")
			}
		}
	}
}
func TestOfflineIPMigrationRefusesUnexpectedProvider(t *testing.T) {
	f := newNativeFixture(t)
	if _, e := f.provider.UpTo(f.ctx, 79); e != nil {
		t.Fatal(e)
	}
	if _, e := f.db.ExecContext(f.ctx, `ALTER TABLE ip_geo_settings DROP CONSTRAINT ip_geo_settings_id_check;INSERT INTO ip_geo_settings(id,encrypted_credentials)VALUES('unrelated-provider',decode('aabb','hex'))`); e != nil {
		t.Fatal(e)
	}
	if _, e := f.provider.UpTo(f.ctx, 80); e == nil {
		t.Fatal("unexpected provider must stop cleanup")
	}
	if f.text(t, `SELECT encode(encrypted_credentials,'hex') FROM ip_geo_settings WHERE id='unrelated-provider'`) != "aabb" {
		t.Fatal("unrelated provider credentials changed")
	}
}

func TestOfflineIPMigrationToleratesAlreadyRemovedOwnTable(t *testing.T) {
	f := newNativeFixture(t)
	if _, e := f.provider.UpTo(f.ctx, 79); e != nil {
		t.Fatal(e)
	}
	if _, e := f.db.ExecContext(f.ctx, `DROP TABLE ip_geo_settings`); e != nil {
		t.Fatal(e)
	}
	if _, e := f.provider.UpTo(f.ctx, 80); e != nil {
		t.Fatal(e)
	}
}
