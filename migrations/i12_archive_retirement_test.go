package migrations_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestI12ManagementUpDownUpPreservesJournalAndDisk(t *testing.T) {
	f := newNativeFixture(t)
	if _, e := f.provider.UpTo(f.ctx, 92); e != nil {
		t.Fatal(e)
	}
	root := t.TempDir()
	active := filepath.Join(root, "Movie.strm")
	retired := filepath.Join(root, ".media-tool-retired", "old.strm.retired")
	if e := os.MkdirAll(filepath.Dir(retired), 0700); e != nil {
		t.Fatal(e)
	}
	body := []byte("synthetic-stable-resolver-url\n")
	for _, p := range []string{active, retired} {
		if e := os.WriteFile(p, body, 0600); e != nil {
			t.Fatal(e)
		}
	}
	sum := sha256.Sum256(body)
	f.exec(t, `INSERT INTO storage_connections(id,name,provider)VALUES('12121212-0000-4000-8000-000000000001','I12','clouddrive2')`)
	f.exec(t, `INSERT INTO cloud_files(id,storage_id,full_path,size_bytes)VALUES('12121212-0000-4000-8000-000000000002','12121212-0000-4000-8000-000000000001','/I12.mkv',1)`)
	f.exec(t, `INSERT INTO media_assets(id,cloud_file_id)VALUES('12121212-0000-4000-8000-000000000003','12121212-0000-4000-8000-000000000002')`)
	f.exec(t, `INSERT INTO media_artifact_journal(id,relative_path,root_path,asset_id,owner_scope,artifact_kind,content_digest,expected_digest,operation,archive_relative,state)VALUES('12121212-0000-4000-8000-000000000004','old.strm',$1,'12121212-0000-4000-8000-000000000003','asset:12121212-0000-4000-8000-000000000003','strm',$2,$2,'archive','.media-tool-retired/old.strm.retired','applied')`, root, sum[:])
	f.exec(t, `UPDATE media_archive_policy SET retention_days=45,revision=7`)
	snapshot := f.text(t, `SELECT jsonb_agg(to_jsonb(j) ORDER BY id)::text FROM media_artifact_journal j`)
	for _, version := range []int64{93, 92, 93} {
		var e error
		if version == 92 {
			_, e = f.provider.DownTo(f.ctx, version)
		} else {
			_, e = f.provider.UpTo(f.ctx, version)
		}
		if e != nil {
			t.Fatal(e)
		}
		f.version(t, version)
		if f.text(t, `SELECT jsonb_agg(to_jsonb(j) ORDER BY id)::text FROM media_artifact_journal j`) != snapshot {
			t.Fatal("journal evidence changed")
		}
		for _, p := range []string{active, retired} {
			actual, e := os.ReadFile(p)
			if e != nil || sha256.Sum256(actual) != sum {
				t.Fatal("disk bytes changed", p, e)
			}
		}
		want := int64(0)
		if version == 92 {
			want = 3
			if f.integer(t, `SELECT retention_days FROM media_archive_policy`) != 30 || f.integer(t, `SELECT revision FROM media_archive_policy`) != 1 {
				t.Fatal("Down must restore default, not fabricate original policy")
			}
		}
		if f.integer(t, `SELECT count(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name IN('media_archive_policy','media_archive_cleanup_requests','media_archive_cleanup_operations')`) != want {
			t.Fatal("management table shape")
		}
		t.Logf("schema=%d journal and active/retired SHA256=%s unchanged; managementTables=%d", version, hex.EncodeToString(sum[:]), want)
	}
}

func TestI12MigrationStopsForUnfinishedApprovalsAndJobs(t *testing.T) {
	f := newNativeFixture(t)
	if _, e := f.provider.UpTo(f.ctx, 92); e != nil {
		t.Fatal(e)
	}
	f.exec(t, `INSERT INTO storage_connections(id,name,provider)VALUES('12121212-0000-4000-8000-000000000001','I12','clouddrive2')`)
	f.exec(t, `INSERT INTO cloud_files(id,storage_id,full_path,size_bytes)VALUES('12121212-0000-4000-8000-000000000002','12121212-0000-4000-8000-000000000001','/I12.mkv',1)`)
	f.exec(t, `INSERT INTO media_assets(id,cloud_file_id)VALUES('12121212-0000-4000-8000-000000000003','12121212-0000-4000-8000-000000000002')`)
	f.exec(t, `INSERT INTO media_artifact_journal(id,relative_path,root_path,asset_id,owner_scope,artifact_kind,content_digest,state)VALUES('12121212-0000-4000-8000-000000000004','old.strm','/isolated','12121212-0000-4000-8000-000000000003','asset:owned','strm',decode(repeat('a',64),'hex'),'applied')`)
	f.exec(t, `INSERT INTO media_archive_cleanup_requests(id,plan_digest,archive_ids,policy_revision,retention_days)VALUES('12121212-0000-4000-8000-000000000005',repeat('b',64),ARRAY['12121212-0000-4000-8000-000000000004']::uuid[],1,30)`)
	f.exec(t, `INSERT INTO media_archive_cleanup_operations(id,request_id,journal_id,expected_digest,quarantine_relative)VALUES('12121212-0000-4000-8000-000000000006','12121212-0000-4000-8000-000000000005','12121212-0000-4000-8000-000000000004',decode(repeat('a',64),'hex'),'.cleanup-owned.retired')`)
	for _, state := range []string{"approved", "quarantined", "conflict"} {
		f.exec(t, `UPDATE media_archive_cleanup_operations SET state=$1`, state)
		if _, e := f.provider.UpTo(f.ctx, 93); e == nil || !strings.Contains(e.Error(), "unfinished archive") {
			t.Fatal("approval guard", state, e)
		}
		f.version(t, 92)
		if f.text(t, `SELECT state FROM media_archive_cleanup_operations`) != state {
			t.Fatal("failed migration changed approval")
		}
	}
	f.exec(t, `UPDATE media_archive_cleanup_operations SET state='cancelled'`)
	for _, state := range []string{"pending", "running", "retry", "paused", "unknown", "checking"} {
		f.exec(t, `INSERT INTO jobs(id,kind,state,lease_until,lease_token,locked_by)VALUES('12121212-0000-4000-8000-000000000007','library.archive.cleanup',$1,CASE WHEN $1='running' THEN now()+interval '1 minute' END,CASE WHEN $1='running' THEN '12121212-0000-4000-8000-000000000008'::uuid END,CASE WHEN $1='running' THEN 'i12-fixture' END)`, state)
		if _, e := f.provider.UpTo(f.ctx, 93); e == nil || !strings.Contains(e.Error(), "non-terminal archive") {
			t.Fatal("job guard", state, e)
		}
		f.version(t, 92)
		f.exec(t, `DELETE FROM jobs WHERE id='12121212-0000-4000-8000-000000000007'`)
	}
	if _, e := f.provider.UpTo(f.ctx, 93); e != nil {
		t.Fatal(e)
	}
	f.version(t, 93)
}
