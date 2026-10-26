package migrations_test

import (
	"testing"
)

func TestT15JWTUpDownUpRetiresOnlyExternalCredentials(t *testing.T) {
	f := newNativeFixture(t)
	if _, e := f.provider.UpTo(f.ctx, 88); e != nil {
		t.Fatal(e)
	}
	f.exec(t, `INSERT INTO external_api_keys(id,name,token_digest,token_ciphertext,prefix,scopes)VALUES('11111111-2222-4333-8444-555555555555','legacy','synthetic-digest','synthetic-cipher','mt_api_fixture','["status.read"]');INSERT INTO external_api_calls(key_id,method,path,status)VALUES('11111111-2222-4333-8444-555555555555','GET','/api/external/v1/status',200);INSERT INTO app_settings(key,value)VALUES('t15-unrelated-credential','"synthetic-secret"')`)
	for i := 0; i < 2; i++ {
		if _, e := f.provider.UpTo(f.ctx, 89); e != nil {
			t.Fatal(e)
		}
		if n := f.integer(t, `SELECT count(*)FROM external_api_keys`); n != 0 {
			t.Fatal(n)
		}
		want := int64(1)
		if i == 1 {
			want = 0
		}
		if n := f.integer(t, `SELECT legacy_count FROM external_api_migration_receipt`); n != want {
			t.Fatal(n)
		}
		if n := f.integer(t, `SELECT count(*)FROM app_settings WHERE key='t15-unrelated-credential'AND value='"synthetic-secret"'`); n != 1 {
			t.Fatal("unrelated credential changed")
		}
		if n := f.integer(t, `SELECT count(*)FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='external_api_keys'AND column_name IN('token_ciphertext','token_digest','expires_at','prefix')`); n != 0 {
			t.Fatal(n)
		}
		if i == 0 {
			if n := f.integer(t, `SELECT count(*)FROM external_api_calls WHERE credential_name='legacy'`); n != 1 {
				t.Fatal("history not preserved")
			}
			if _, e := f.provider.Down(f.ctx); e != nil {
				t.Fatal(e)
			}
		}
	}
}
