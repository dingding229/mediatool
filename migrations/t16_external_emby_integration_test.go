package migrations_test

import (
	"fmt"
	"media-tool/internal/modules/acquisition"
	"testing"
)

func TestT16UpDownUpRetiresExternalOnlyAndFinishesEvidenceBackedDownloads(t *testing.T) {
	f := newNativeFixture(t)
	if _, e := f.provider.UpTo(f.ctx, 89); e != nil {
		t.Fatal(e)
	}
	f.exec(t, `INSERT INTO storage_connections(id,name,provider)VALUES($1,'synthetic','clouddrive2')`, storageID)
	f.exec(t, `INSERT INTO integrations(id,kind,name,settings,credentials_ciphertext)VALUES($1,'clouddrive2','keep','{}','synthetic'),($2,'emby','retire','{}','external')`, storageID, userOne)
	f.exec(t, `INSERT INTO media_items(id,media_type,tmdb_id,title,metadata)VALUES($1,'movie',16,'Synthetic','{}')`, sessionID)
	// Three cases: complete evidence, imported flag without owned artifact, pending file.
	for i := 1; i <= 3; i++ {
		id := func(n int) string { return fmt.Sprintf("%08d-0016-4016-8016-%012d", n, i) }
		task, file, cloud, asset := id(1), id(2), id(3), id(4)
		f.exec(t, `INSERT INTO download_tasks(id,download_state)VALUES($1,'refresh_requested')`, task)
		f.exec(t, `INSERT INTO download_files(id,task_id,file_index,relative_path,size_bytes,media_state)VALUES($1,$2,0,'synthetic.mkv',1,$3)`, file, task, map[bool]string{true: "pending", false: "imported"}[i == 3])
		f.exec(t, `INSERT INTO cloud_files(id,storage_id,full_path,size_bytes)VALUES($1,$2,$3,1)`, cloud, storageID, fmt.Sprintf("/synthetic/%d.mkv", i))
		f.exec(t, `INSERT INTO media_assets(id,cloud_file_id,artifacts_state)VALUES($1,$2,'ready')`, asset, cloud)
		f.exec(t, `INSERT INTO media_asset_bindings(asset_id,media_id,download_file_id)VALUES($1,$2,$3)`, asset, sessionID, file)
		f.exec(t, `INSERT INTO transfer_tasks(id,download_file_id,integration_id,destination,device_id,state)VALUES($1,$2,$3,$4,'synthetic','imported')`, id(5), file, storageID, fmt.Sprintf("/synthetic/%d.mkv", i))
		f.exec(t, `INSERT INTO strm_entries(id,asset_id,output_path,key_reference)VALUES($1,$2,$3,'synthetic')`, id(6), asset, fmt.Sprintf("%d.strm", i))
		if i == 1 {
			f.exec(t, `INSERT INTO managed_media_artifacts(relative_path,asset_id,artifact_kind,content_digest,owner_scope)VALUES('1.strm',$1::uuid,'strm',decode('00','hex'),'asset:'||($1::uuid)::text)`, asset)
		}
	}
	f.exec(t, `INSERT INTO app_settings(key,value)VALUES('t16-unrelated','"must-stay"')`)
	f.exec(t, `UPDATE application_settings SET value=value||'{"embyUserId":"external-user","access":{"embyUrl":"https://external.invalid","memberUrl":"https://native.invalid"}}'`)
	f.exec(t, `INSERT INTO jobs(id,kind,payload,idempotency_key)VALUES($1,'emby.confirm','{}','t16:external')`, userTwo)
	for round := 0; round < 2; round++ {
		if _, e := f.provider.UpTo(f.ctx, 90); e != nil {
			t.Fatal(e)
		}
		if n := f.integer(t, `SELECT count(*) FROM download_tasks WHERE download_state='imported'`); n != 1 {
			t.Fatal("false success or missing completion", n)
		}
		if n := f.integer(t, `SELECT count(*) FROM download_tasks WHERE download_state='refresh_requested'`); n != 2 {
			t.Fatal("incomplete evidence was discarded", n)
		}
		if n := f.integer(t, `SELECT count(*) FROM download_tasks d WHERE download_state='imported' AND `+acquisition.ImportEvidenceSQL); n != 1 {
			t.Fatal("runtime/migration evidence drift", n)
		}
		if n := f.integer(t, `SELECT count(*) FROM integrations WHERE kind='emby'`); n != 0 {
			t.Fatal(n)
		}
		if n := f.integer(t, `SELECT count(*) FROM integrations WHERE kind='clouddrive2' AND credentials_ciphertext='synthetic'`); n != 1 {
			t.Fatal("other credential modified")
		}
		if f.text(t, `SELECT value#>>'{access,memberUrl}' FROM application_settings`) != "https://native.invalid" {
			t.Fatal("native URL modified")
		}
		if f.text(t, `SELECT state FROM jobs WHERE id=$1`, userTwo) != "cancelled" {
			t.Fatal("external job replayable")
		}
		if f.text(t, `SELECT value#>>'{}' FROM app_settings WHERE key='t16-unrelated'`) != "must-stay" {
			t.Fatal("other setting modified")
		}
		f.rejects(t, "23514", `INSERT INTO integrations(id,kind,name,credentials_ciphertext)VALUES($1,'emby','removed','synthetic')`, userOne)
		if round == 0 {
			if _, e := f.provider.Down(f.ctx); e != nil {
				t.Fatal(e)
			}
		}
	}
}
