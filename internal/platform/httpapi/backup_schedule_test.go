package httpapi

import (
	"context"
	"encoding/json"
	"media-tool/internal/modules/maintenance"
	"media-tool/internal/platform/database"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestBackupPolicyResponseHidesOnlyDisabledNext(t *testing.T) {
	next := time.Date(2026, 10, 26, 3, 0, 0, 0, time.UTC)
	for _, enabled := range []bool{false, true} {
		policy := maintenance.PolicyRecord{Value: maintenance.Defaults(), Revision: 19, NextBackup: &next}
		policy.Value.BackupEnabled = enabled
		out := backupPolicyResponse(policy)
		if enabled && out.NextBackup != policy.NextBackup {
			t.Fatal("enabled next changed")
		}
		if !enabled && out.NextBackup != nil {
			t.Fatal("disabled next not null")
		}
		if policy.NextBackup != &next || out.Value != policy.Value || out.Revision != policy.Revision {
			t.Fatal("projection mutated policy")
		}
		raw, err := json.Marshal(out)
		if err != nil {
			t.Fatal(err)
		}
		var decoded map[string]any
		if err = json.Unmarshal(raw, &decoded); err != nil {
			t.Fatal(err)
		}
		if _, exists := decoded["nextBackupAt"]; !exists {
			t.Fatal("nextBackupAt omitted instead of null")
		}
		if !enabled && decoded["nextBackupAt"] != nil {
			t.Fatal(string(raw))
		}
	}
	for _, enabled := range []bool{false, true} {
		policy := maintenance.PolicyRecord{NextBackup: nil}
		policy.Value.BackupEnabled = enabled
		if backupPolicyResponse(policy).NextBackup != nil {
			t.Fatal("nil next fabricated")
		}
	}
}

func TestBackupScheduleReadEndpointsDoNotChangeStoredNext(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated database")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	if err = db.QueryRow(ctx, `SELECT current_database()`).Scan(&name); err != nil || name != "media_tool_test" {
		t.Fatal("test database required", name, err)
	}
	var original []byte
	var oldNext *time.Time
	if err = db.QueryRow(ctx, `SELECT value,next_backup_at FROM ops_policy WHERE singleton`).Scan(&original, &oldNext); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `UPDATE ops_policy SET value=$1,next_backup_at=$2 WHERE singleton`, original, oldNext)
	next := time.Date(2026, 10, 26, 3, 0, 0, 0, time.UTC)
	h := New(cfg(), Dependencies{DB: db, AuthSessions: testSessionReader{}})
	for _, enabled := range []bool{false, true} {
		value, _ := json.Marshal(map[string]bool{"backupEnabled": enabled})
		if _, err = db.Exec(ctx, `UPDATE ops_policy SET value=$1,next_backup_at=$2 WHERE singleton`, value, next); err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{"/api/v1/ops", "/api/v1/system/operations"} {
			w := invoke(h, http.MethodGet, path, "", cfg().SessionKey)
			if w.Code != 200 {
				t.Fatal(path, w.Code, w.Body.String())
			}
			var response struct {
				Data struct {
					Policy    maintenance.PolicyRecord
					Schedules []struct {
						ID   string
						Next *time.Time `json:"nextRunAt"`
					}
				}
			}
			if err = json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			got := response.Data.Policy.NextBackup
			if path == "/api/v1/system/operations" {
				found := false
				for _, s := range response.Data.Schedules {
					if s.ID == "ops-backup" {
						got = s.Next
						found = true
					}
				}
				if !found {
					t.Fatal("backup schedule missing")
				}
			}
			if !enabled && got != nil || enabled && (got == nil || !got.Equal(next)) {
				t.Fatalf("%s enabled=%v next=%v", path, enabled, got)
			}
		}
		var stored time.Time
		if err = db.QueryRow(ctx, `SELECT next_backup_at FROM ops_policy WHERE singleton`).Scan(&stored); err != nil || !stored.Equal(next) {
			t.Fatal("stored next changed", stored, err)
		}
	}
}
