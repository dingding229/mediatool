package maintenance

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/modules/mediausers/nativeauth"
	"media-tool/internal/platform/auth"
)

// The web cases exercise the real single-transaction importer and the same
// database's pre-snapshot/compensation; only candidate preparation is a seam.
// The test session uses native PG client binaries. Actions reuses its existing
// container-compatible PG fixture, unless explicitly asked to exercise native.
func TestRP33WebRestoreRevokesMediaTokensAndKeepsOutputReceipts(t *testing.T) {
	for _, tc := range []struct {
		name, state string
		inject      func(*Engine)
	}{
		{"commit", "completed", nil},
		{"short_write", "failed", func(e *Engine) {
			e.outputIO = &restoreOutputIO{write: func(f *os.File, raw []byte) (int, error) { return f.Write(raw[:len(raw)-1]) }}
		}},
		{"file_fsync", "failed", func(e *Engine) {
			e.outputIO = &restoreOutputIO{sync: func(f *os.File) error { return errors.New("synthetic fsync failure") }}
		}},
		{"directory_fsync", "failed", func(e *Engine) {
			e.outputIO = &restoreOutputIO{sync: func(f *os.File) error {
				info, err := f.Stat()
				if err == nil && info.IsDir() {
					return errors.New("synthetic directory fsync failure")
				}
				return f.Sync()
			}}
		}},
		{"transaction_compensation", "rolled_back", func(e *Engine) {
			run, injected := e.Run, false
			e.Run = func(ctx context.Context, name string, args, env []string, in io.Reader, out io.Writer) error {
				if name == "psql" && !injected {
					injected = true
					in = io.MultiReader(in, strings.NewReader("\nSELECT 1/0;\n"))
				}
				return run(ctx, name, args, env, in, out)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			var token string
			native := os.Getenv("GITHUB_ACTIONS") != "true" || os.Getenv("MEDIA_TOOL_RP33_NATIVE_PG") == "1"
			e, target := webFixtureWithSetup(t, native, func(e Engine) { token = seedRP33NativeToken(t, ctx, e) })
			payload := []byte("synthetic output; not a player URL\n")
			seedRP33OutputBundle(t, ctx, e, target, payload)
			if err := os.WriteFile(filepath.Join(e.OutputRoot, "different.strm"), []byte("existing, never overwrite"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(e.OutputRoot, "match.strm"), payload, 0600); err != nil {
				t.Fatal(err)
			}
			r := queueWeb(t, e, target)
			if tc.inject != nil {
				tc.inject(&e)
			}
			if err := e.RunWebRestore(ctx); err != nil {
				t.Fatal(err)
			}
			got, err := e.WebRestore(ctx, r.ID)
			if err != nil || got.State != tc.state || got.SnapshotID == nil || got.OutputReceipt == nil || got.OutputReceipt.State != "recorded" {
				t.Fatal(got.State, err)
			}
			if tc.state == "completed" && (got.AppliedBackup == nil || *got.AppliedBackup != target) || tc.state == "rolled_back" && (got.AppliedBackup == nil || *got.AppliedBackup != *got.SnapshotID) || tc.state == "failed" && got.AppliedBackup != nil {
				t.Fatal("unexpected database commit marker", got.State)
			}
			records := map[string]RestoreFileResult{}
			for _, f := range got.OutputReceipt.Files {
				records[f.Path] = f
			}
			if records["match.strm"].Status != outputMatch || records["different.strm"].Status != outputMismatch {
				t.Fatal(records)
			}
			filled := records["new.strm"]
			published := tc.state != "failed" || tc.name == "directory_fsync"
			wantStatus := outputFilled
			if tc.state == "failed" {
				wantStatus = outputFailed
			}
			if filled.Status != wantStatus || filled.Published != published || tc.state != "completed" && published && !strings.Contains(got.Message, "已补文件") {
				t.Fatal(filled, got.Message)
			}
			if raw, err := os.ReadFile(filepath.Join(e.OutputRoot, "new.strm")); published && (err != nil || string(raw) != string(payload)) || !published && !errors.Is(err, os.ErrNotExist) {
				t.Fatal("final file missing, partial or incorrectly published", err)
			}
			if raw, err := os.ReadFile(filepath.Join(e.OutputRoot, "different.strm")); err != nil || string(raw) != "existing, never overwrite" {
				t.Fatal("existing data was changed", err)
			}
			if temp, err := filepath.Glob(filepath.Join(e.OutputRoot, ".restore-*")); err != nil || len(temp) > 0 {
				t.Fatal("temporary output leaked", temp, err)
			}
			checkRP33NativeToken(t, ctx, e, token, tc.state != "failed")
		})
	}
}

func TestRP33OfflineCandidateRevokesMediaTokens(t *testing.T) {
	e := fixture(t)
	if os.Getenv("MEDIA_TOOL_TEST_LOCKED") != "1" {
		t.Fatal("test-lock required")
	}
	ctx := context.Background()
	token := seedRP33NativeToken(t, ctx, e)
	// This is the exact offline candidate initialization transaction, run on
	// the isolated fixture instead of provisioning a second DB.
	if err := prepareOfflineRestore(ctx, e.DB, newID()); err != nil {
		t.Fatal(err)
	}
	checkRP33NativeToken(t, ctx, e, token, true)
}

const rp33User = "rp33_fixture_member"

func seedRP33NativeToken(t *testing.T, ctx context.Context, e Engine) string {
	t.Helper()
	pw, err := auth.HashPassword("synthetic-password")
	if err != nil {
		t.Fatal(err)
	}
	var id string
	if err = e.DB.QueryRow(ctx, `INSERT INTO media_users(id,username,display_name,password_hash,enabled,must_change_password) VALUES($1,$2,'Restore fixture',$3,true,false) RETURNING id::text`, newID(), rp33User, pw).Scan(&id); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := e.DB.Exec(context.Background(), `DELETE FROM media_users WHERE id=$1`, id); err != nil {
			t.Error(err)
		}
	})
	device := newID()
	if _, err = e.DB.Exec(ctx, `INSERT INTO media_user_devices(id,user_id,device_key,name) VALUES($1,$2,'rp33-device','Preserved device')`, device, id); err != nil {
		t.Fatal(err)
	}
	token := hash([]byte(newID()))
	digest := sha256.Sum256([]byte(token))
	if _, err = e.DB.Exec(ctx, `INSERT INTO media_user_tokens(token_digest,user_id,device_id,credential_version,expires_at) VALUES($1,$2,$3,1,now()+interval '1 day')`, digest[:], id, device); err != nil {
		t.Fatal(err)
	}
	checkRP33NativeToken(t, ctx, e, token, false)
	return token
}

func checkRP33NativeToken(t *testing.T, ctx context.Context, e Engine, token string, revoked bool) {
	t.Helper()
	var kept, wasRevoked bool
	digest := sha256.Sum256([]byte(token))
	if err := e.DB.QueryRow(ctx, `SELECT u.username=$2 AND d.name='Preserved device',t.revoked_at IS NOT NULL FROM media_user_tokens t JOIN media_users u ON u.id=t.user_id JOIN media_user_devices d ON d.id=t.device_id WHERE t.token_digest=$1`, digest[:], rp33User).Scan(&kept, &wasRevoked); err != nil || !kept || wasRevoked != revoked {
		t.Fatal("account/device not retained or wrong revocation", kept, wasRevoked, err)
	}
	store := nativeauth.Store{DB: e.DB}
	_, _, _, err := store.Authenticate(ctx, token, contracts.DeviceInfo{DeviceID: "rp33-device"})
	if revoked && !errors.Is(err, contracts.ErrUnauthenticated) || !revoked && err != nil {
		t.Fatal("native listener token validator returned unexpected state", err)
	}
}

func seedRP33OutputBundle(t *testing.T, ctx context.Context, e Engine, id string, payload []byte) {
	t.Helper()
	m, dir, err := e.VerifyBundle(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "outputs.tar")
	f, err := os.Create(file)
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(f)
	for _, name := range []string{"new.strm", "match.strm", "different.strm"} {
		if err = tw.WriteHeader(&tar.Header{Name: name, Size: int64(len(payload)), Mode: 0600, Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err = tw.Write(payload); err != nil {
			t.Fatal(err)
		}
		m.Files = append(m.Files, FileProof{Path: name, Bytes: int64(len(payload)), Digest: hash(payload)})
	}
	if err = errors.Join(tw.Close(), f.Close()); err != nil {
		t.Fatal(err)
	}
	m.OutputsDigest, _, err = digestFile(file)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "manifest.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = e.DB.Exec(ctx, `UPDATE ops_backups SET manifest=$2,digest=$3 WHERE id=$1`, id, raw, hash(raw)); err != nil {
		t.Fatal(err)
	}
}

func TestRP33RestorePublicationNeverClobbersRacingTarget(t *testing.T) {
	root := t.TempDir()
	archive := filepath.Join(t.TempDir(), "synthetic.tar")
	const name = "racing.strm"
	raw := []byte("fixture output")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	w := tar.NewWriter(f)
	if err = w.WriteHeader(&tar.Header{Name: name, Size: int64(len(raw)), Mode: 0600, Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err = w.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err = errors.Join(w.Close(), f.Close()); err != nil {
		t.Fatal(err)
	}
	e := Engine{OutputRoot: root, outputIO: &restoreOutputIO{write: func(f *os.File, raw []byte) (int, error) {
		// A concurrent writer established the final name after the initial Lstat.
		if err := os.WriteFile(filepath.Join(root, name), []byte("racing owner"), 0600); err != nil {
			return 0, err
		}
		return f.Write(raw)
	}}}
	results, err := e.restoreOutputs(context.Background(), archive, []FileProof{{Path: name, Bytes: int64(len(raw)), Digest: hash(raw)}})
	if err != nil || len(results) != 1 || results[0].Status != outputMismatch || results[0].Published {
		t.Fatal(results, err)
	}
	content, err := os.ReadFile(filepath.Join(root, name))
	if err != nil || string(content) != "racing owner" {
		t.Fatal("racing target overwritten", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 || entries[0].Name() != name {
		t.Fatal("temporary output leaked", err)
	}
}
