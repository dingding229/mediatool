package migrations_test

import (
	"encoding/json"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/nativelibrary/catalog"
	"strings"
	"testing"
)

func TestNativeAssetAssignmentAndTriggerQueue(t *testing.T) {
	f := newNativeFixture(t)
	// This test executes the current job repository as well as assignment.
	if _, err := f.provider.UpTo(f.ctx, 73); err != nil {
		t.Fatal(err)
	}
	pool := catalogTestPool(t, f)
	defer pool.Close()
	asset := "99999999-0000-4000-8000-000000000057"
	file := "99999999-0000-4000-8000-000000000058"
	storage := "99999999-0000-4000-8000-000000000059"
	media := "99999999-0000-4000-8000-000000000060"
	f.exec(t, `INSERT INTO storage_connections(id,name,provider)VALUES($1,'T12 storage','clouddrive2')`, storage)
	f.exec(t, `INSERT INTO cloud_files(id,storage_id,full_path,size_bytes,state)VALUES($1,$2,'/115/T12.mkv',1,'present')`, file, storage)
	f.exec(t, `INSERT INTO media_items(id,media_type,tmdb_id,title,release_year,metadata)VALUES($1,'movie',120057,'T12 synthetic','2026',$2)`, media, json.RawMessage(`{"mediaType":"movie","title":"T12 synthetic","year":"2026"}`))
	f.exec(t, `INSERT INTO media_assets(id,cloud_file_id,media_identity,revision)VALUES($1,$2,'{}',1)`, asset, file)
	f.exec(t, `INSERT INTO media_asset_bindings(asset_id,media_id,state)VALUES($1,$2,'ready')`, asset, media)
	f.exec(t, `INSERT INTO native_item_ids(entity_type,internal_key)VALUES('library','r:movie:doc')`)
	f.exec(t, `INSERT INTO native_libraries(id,item_id,group_id,rule_id,name,directory,collection_type,hidden,sort_order,policy_revision)SELECT 'r:movie:doc',id,'movie','doc','纪录片','电影/华语电影','movies',false,0,1 FROM native_item_ids WHERE entity_type='library' AND internal_key='r:movie:doc'`)
	f.exec(t, `INSERT INTO strm_entries(id,asset_id,output_path,key_reference)VALUES($1,$2,'电影/华语电影/T12.strm','t12')`, jobs.UUID(), asset)
	repo := catalog.AssignmentRepository{DB: pool}
	if err := repo.Assign(f.ctx, asset, 1); err != nil {
		t.Fatal(err)
	}
	if got := f.text(t, `SELECT library_id FROM native_asset_libraries WHERE asset_id=$1`, asset); got != "r:movie:doc" {
		t.Fatal("longest/path assignment", got)
	}
	if got := f.text(t, `SELECT source FROM native_asset_libraries WHERE asset_id=$1`, asset); got != "output_directory" {
		t.Fatal(got)
	}
	f.exec(t, `UPDATE strm_entries SET output_path='电影/华语电影2/T12.strm' WHERE asset_id=$1`, asset)
	f.exec(t, `INSERT INTO native_item_ids(entity_type,internal_key)VALUES('library','r:movie:doc2')`)
	f.exec(t, `INSERT INTO native_libraries(id,item_id,group_id,rule_id,name,directory,collection_type,hidden,sort_order,policy_revision)SELECT 'r:movie:doc2',id,'movie','doc2','近似','电影/华语电影2','movies',false,1,1 FROM native_item_ids WHERE entity_type='library' AND internal_key='r:movie:doc2'`)
	f.exec(t, `UPDATE strm_entries SET output_path='电影/华语电影/T12.strm' WHERE asset_id=$1`, asset)
	if err := repo.Assign(f.ctx, asset, 1); err != nil {
		t.Fatal(err)
	}
	if got := f.text(t, `SELECT library_id FROM native_asset_libraries WHERE asset_id=$1`, asset); got != "r:movie:doc" {
		t.Fatal("segment boundary", got)
	}
	f.exec(t, `UPDATE strm_entries SET output_path='其他/T12.strm' WHERE asset_id=$1`, asset)
	f.exec(t, `UPDATE media_assets SET revision=2 WHERE id=$1`, asset)
	f.exec(t, `UPDATE directory_classification SET value=$1,revision=1 WHERE singleton`, json.RawMessage(`{"groups":[{"id":"movie","name":"电影","directory":"电影","mediaType":"movie","enabled":true,"genreIds":[],"rules":[{"id":"doc","name":"纪录片","directory":"华语电影","enabled":true,"genreIds":[],"countries":[],"languages":[]}]}]}`))
	if err := repo.Assign(f.ctx, asset, 2); err != nil {
		t.Fatal(err)
	}
	if got := f.text(t, `SELECT library_id FROM native_asset_libraries WHERE asset_id=$1`, asset); got != "f:movie" {
		t.Fatal("fallback", got)
	}
	if f.integer(t, `SELECT count(*) FROM native_libraries WHERE id='f:movie' AND NOT hidden`) != 1 {
		t.Fatal("fallback not visible")
	}
	var diagnostic []byte
	if err := f.db.QueryRowContext(f.ctx, `SELECT diagnostic FROM native_asset_libraries WHERE asset_id=$1`, asset).Scan(&diagnostic); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(diagnostic), "classificationMismatch") {
		t.Fatal("classification diagnostic missing", string(diagnostic))
	}
	key := `native.assign:` + asset
	f.exec(t, `UPDATE jobs SET state='succeeded',lease_until=NULL,lease_token=NULL,locked_by=NULL,payload=jsonb_set(payload,'{rerun}','false'::jsonb) WHERE idempotency_key=$1`, key)
	f.exec(t, `UPDATE cloud_files SET state='missing' WHERE id=$1`, file)
	if f.text(t, `SELECT state FROM jobs WHERE idempotency_key=$1`, key) != "pending" {
		t.Fatal("succeeded assignment was not requeued")
	}
	f.exec(t, `UPDATE media_assets SET revision=3 WHERE id=$1`, asset)
	if err := repo.Assign(f.ctx, asset, 3); err != nil {
		t.Fatal(err)
	}
	if f.integer(t, `SELECT count(*) FROM native_asset_libraries WHERE asset_id=$1`, asset) != 0 || f.integer(t, `SELECT count(*) FROM native_libraries WHERE id='f:movie' AND NOT hidden`) != 0 {
		t.Fatal("invalid asset exposed")
	}
	if err := catalog.EnqueueAsset(f.ctx, pool, asset, 3); err != nil {
		t.Fatal(err)
	}
	if err := catalog.EnqueueAsset(f.ctx, pool, asset, 3); err != nil {
		t.Fatal(err)
	}
	if f.integer(t, `SELECT count(*) FROM jobs WHERE kind='native.assign' AND idempotency_key=$1`, key) != 1 {
		t.Fatal("assignment enqueue not idempotent")
	}
	f.exec(t, `UPDATE cloud_files SET state='present' WHERE id=$1`, file)
	f.exec(t, `UPDATE media_assets SET revision=4 WHERE id=$1`, asset)
	f.exec(t, `UPDATE strm_entries SET output_path='其他/T12-running.strm' WHERE asset_id=$1`, asset)
	if err := repo.Assign(f.ctx, asset, 4); err != nil {
		t.Fatal(err)
	}
	f.exec(t, `UPDATE jobs SET state='running',lease_until=now()+interval '1 minute',lease_token=gen_random_uuid(),locked_by='t12',payload=jsonb_set(payload,'{rerun}','false'::jsonb) WHERE idempotency_key=$1`, key)
	f.exec(t, `UPDATE strm_entries SET output_path='其他/T12-running-2.strm' WHERE asset_id=$1`, asset)
	var lease string
	var attempts, maxAttempts int
	if err := f.db.QueryRowContext(f.ctx, `SELECT lease_token::text,attempts,max_attempts FROM jobs WHERE idempotency_key=$1`, key).Scan(&lease, &attempts, &maxAttempts); err != nil {
		t.Fatal(err)
	}
	if f.text(t, `SELECT payload->>'rerun' FROM jobs WHERE idempotency_key=$1`, key) != "true" {
		t.Fatal("running assignment did not mark rerun")
	}
	if err := (jobs.Repository{DB: pool}).Finish(f.ctx, jobs.Job{ID: f.text(t, `SELECT id::text FROM jobs WHERE idempotency_key=$1`, key), LeaseToken: lease, Attempts: attempts, MaxAttempts: maxAttempts, Kind: "native.assign"}, nil); err != nil {
		t.Fatal(err)
	}
	if f.text(t, `SELECT state FROM jobs WHERE idempotency_key=$1`, key) != "pending" {
		t.Fatal("rerun was not requeued after running completion")
	}
	next, count, err := catalog.EnqueueBatch(f.ctx, pool, "", 500)
	if err != nil {
		t.Fatal(err)
	}
	if count < 1 || next == "" {
		t.Fatal("full batch did not enqueue", next, count)
	}
	var payload []byte
	if err := f.db.QueryRowContext(f.ctx, `SELECT payload FROM jobs WHERE kind='native.assign' ORDER BY created_at DESC LIMIT 1`).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if json.Unmarshal(payload, &decoded) != nil {
		t.Fatal("invalid job payload")
	}
}
