package integrations_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/search"
	"media-tool/internal/modules/sites"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
)

func TestPostgresExplicitPTHeadersStayEncryptedAndRevisionBound(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated TEST_DATABASE_URL required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	if err = db.QueryRow(ctx, `SELECT current_database()`).Scan(&name); err != nil || name != "media_tool_test" {
		t.Fatal("isolated media_tool_test required", err)
	}
	cleanup := `TRUNCATE integrations,resources,site_definitions,search_runs,download_tasks,subscriptions CASCADE`
	if _, err = db.Exec(ctx, cleanup); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(context.Background(), cleanup)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer literal{passkey}" || r.Header.Get("X-Csrf-Token") != "PRIVATE-CSRF" || r.Header.Get("X-Owned-Unused") != "" {
			w.WriteHeader(403)
			return
		}
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer server.Close()
	vault, _ := secrets.New(strings.Repeat("a", 64))
	repo := integrations.Repository{DB: db, Vault: vault}
	manager := integrations.NewManager(repo, config.Config{})
	defer manager.Close()
	definition := fmt.Sprintf(`{"templateVersion":1,"id":"owned_encrypted_headers","name":"Owned","schema":"NexusPHP","domain":%q,"requests":{"user_basic_info":{"path":"/profile","parser":"JsonPath","required_headers":["Authorization","X-Csrf-Token"],"fields":{"is_login":{"selector":"ok"}}}}}`, server.URL)
	if _, err = db.Exec(ctx, `INSERT INTO site_definitions(id,name,schema_name,source_commit,definition,common_definition,source_repository)VALUES('owned_encrypted_headers','Owned','NexusPHP','fixture',$1,'{}','dingding229/mediatool-site')`, json.RawMessage(definition)); err != nil {
		t.Fatal(err)
	}
	input := integrations.Input{Kind: "ptsite", Name: "Owned", Enabled: false, Settings: map[string]any{"endpoint": server.URL, "definitionId": "owned_encrypted_headers"}, Secrets: map[string]string{"requestHeaders": `{"authorization":"Bearer literal{passkey}","X-Csrf-Token":"PRIVATE-CSRF","X-Owned-Unused":"PRIVATE-UNUSED"}`}}
	record, err := repo.Save(ctx, "", input)
	if err != nil {
		t.Fatal(err)
	}
	assertPrivate := func() {
		t.Helper()
		var raw []byte
		if err = db.QueryRow(ctx, `SELECT credentials_ciphertext FROM integrations WHERE id=$1`, record.ID).Scan(&raw); err != nil || strings.Contains(string(raw), "PRIVATE-") || strings.Contains(string(raw), "literal{passkey}") {
			t.Fatal("plaintext request-header credentials stored", err)
		}
		list, err := repo.List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		public, _ := json.Marshal(list)
		if strings.Contains(string(public), "PRIVATE-") || strings.Contains(string(public), "literal{passkey}") {
			t.Fatal("public integration DTO exposed request-header values")
		}
		var audit string
		if err = db.QueryRow(ctx, `SELECT coalesce(json_agg(row_to_json(a)),'[]'::json)::text FROM audit_logs a`).Scan(&audit); err != nil || strings.Contains(audit, "PRIVATE-") {
			t.Fatal("request-header values appeared in audit", err)
		}
	}
	assertPrivate()
	if _, err = manager.Site(ctx, record.ID); err == nil {
		t.Fatal("disabled PT account became usable for business requests")
	}
	check, err := manager.Test(ctx, record.ID)
	if err != nil || !check.Passed {
		t.Fatal("disabled PT account cannot explicitly verify credentials before enabling", err)
	}
	stillDisabled, err := repo.Get(ctx, record.ID)
	if err != nil || stillDisabled.Record.Enabled || stillDisabled.Record.Revision != record.Revision {
		t.Fatal("readonly PT check enabled or changed the account revision", err)
	}
	if _, err = manager.Site(ctx, record.ID); err == nil {
		t.Fatal("readonly validation implicitly enabled business usage")
	}
	input.Revision = record.Revision
	input.Enabled = true
	input.Secrets = nil
	record, err = repo.Save(ctx, record.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	client, err := manager.Site(ctx, record.ID)
	if err != nil || client.Account.RequestHeaders["Authorization"] != "Bearer literal{passkey}" {
		t.Fatal("manager did not decrypt exact credentials", err)
	}
	if err = client.Test(ctx); err != nil {
		t.Fatal("account-required headers not usable through manager", err)
	}
	searcher := search.Engine{DB: db, Vault: vault, Connections: manager}
	run, err := searcher.StartInline(ctx, "Owned", []string{record.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err = searcher.SaveResourcesWithClient(ctx, run, record.ID, nil, client); err != nil {
		t.Fatal(err)
	}
	if err = searcher.ValidateRunSources(ctx, run); err != nil {
		t.Fatal(err)
	}
	// An ordinary edit leaves the encrypted value intact, but still changes
	// account revision. The exact older HTTP client's evidence is not reusable.
	input.Revision = record.Revision
	input.Name = "Owned renamed"
	input.Secrets = nil
	record, err = repo.Save(ctx, record.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	if err = searcher.ValidateRunSources(ctx, run); !errors.Is(err, search.ErrSnapshotChanged) {
		t.Fatal("header account edit did not invalidate prior origin evidence", err)
	}
	retained, err := manager.Site(ctx, record.ID)
	if err != nil || retained.Account.RequestHeaders["X-Csrf-Token"] != "PRIVATE-CSRF" {
		t.Fatal("blank UI edit failed to preserve encrypted values", err)
	}
	assertPrivate()
	newRun, err := searcher.StartInline(ctx, "Owned", []string{record.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err = searcher.SaveResourcesWithClient(ctx, newRun, record.ID, nil, retained); err != nil {
		t.Fatal(err)
	}
	input.Revision = record.Revision
	input.Secrets = map[string]string{"requestHeaders": `{"Authorization":"Bearer literal{passkey}","X-Csrf-Token":"PRIVATE-CSRF-ROTATED"}`}
	record, err = repo.Save(ctx, record.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := manager.Site(ctx, record.ID)
	if err != nil || rotated.Account.RequestHeaders["X-Csrf-Token"] != "PRIVATE-CSRF-ROTATED" {
		t.Fatal("encrypted header change not read from latest revision", err)
	}
	if err = searcher.ValidateRunSources(ctx, newRun); !errors.Is(err, search.ErrSnapshotChanged) {
		t.Fatal("changed header did not invalidate exact prior source proof", err)
	}
	if err = searcher.SaveResourcesWithClient(ctx, newRun, record.ID, nil, rotated); !errors.Is(err, search.ErrSnapshotChanged) {
		t.Fatal("new header revision overwrote old collection evidence", err)
	}
	// Changing the access origin cannot retain this credential blob implicitly.
	input.Revision = record.Revision
	input.Secrets = nil
	input.Settings["endpoint"] = "https://different-owned.invalid"
	if _, err = repo.Save(ctx, record.ID, input); err == nil {
		t.Fatal("changed origin retained request-header credentials without replacement")
	}
	input.Settings["endpoint"] = server.URL
	assertPrivate()
	// A dry-run rekey of the containing ciphertext must leave exact bytes and
	// values intact; no new table or migration is introduced for these headers.
	var before []byte
	if err = db.QueryRow(ctx, `SELECT credentials_ciphertext FROM integrations WHERE id=$1`, record.ID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	next, _ := secrets.New(strings.Repeat("b", 64))
	counts, err := secrets.Rekey(ctx, db, vault, next, true)
	if err != nil || counts["integrations"] != 1 {
		t.Fatal("request-header credential rekey dry-run failed", err)
	}
	var after []byte
	if err = db.QueryRow(ctx, `SELECT credentials_ciphertext FROM integrations WHERE id=$1`, record.ID).Scan(&after); err != nil || string(before) != string(after) {
		t.Fatal("dry-run committed changed ciphertext", err)
	}
	// {} is an explicit clear. Empty UI inputs are omitted and preserve values.
	input.Revision = record.Revision
	input.Secrets = map[string]string{"requestHeaders": " { } "}
	record, err = repo.Save(ctx, record.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	cleared, err := manager.Site(ctx, record.ID)
	if err != nil || len(cleared.Account.RequestHeaders) != 0 {
		t.Fatal("explicit clear retained request headers", err)
	}
	for _, field := range record.SecretFields {
		if field == "requestHeaders" {
			t.Fatal("cleared headers still marked present")
		}
	}
	if err = cleared.Test(ctx); !errors.Is(err, sites.ErrAuthentication) {
		t.Fatal("missing required headers did not fail before HTTP", err)
	}
}
