package search

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/platform/config"
)

func TestPostgresProviderPagesAreCollectedAndPartialOutcomeIsNotFailedOrComplete(t *testing.T) {
	for _, mode := range []string{"complete", "repeated", "late_failure", "page_limit"} {
		t.Run(mode, func(t *testing.T) {
			ctx, engine, account, newRun := snapshotSearchFixture(t)
			var mu sync.Mutex
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/profile" {
					fmt.Fprint(w, `{"data":{"username":"owned"}}`)
					return
				}
				mu.Lock()
				calls++
				count := calls
				mu.Unlock()
				var body map[string]any
				json.NewDecoder(r.Body).Decode(&body)
				if body["mode"] != "owned-mode" || body["page"] != fmt.Sprint(count) {
					w.WriteHeader(400)
					return
				}
				if mode == "late_failure" && count == 2 {
					w.WriteHeader(503)
					return
				}
				if mode == "complete" && count == 3 {
					fmt.Fprint(w, `{"data":{"items":[]}}`)
					return
				}
				id := count
				if mode == "repeated" {
					id = 1
				}
				fmt.Fprintf(w, `{"data":{"items":[{"id":"%d","name":"Owned Movie (2026)"}]}}`, id)
			}))
			defer server.Close()
			if _, err := engine.DB.Exec(ctx, `UPDATE integrations SET settings=jsonb_set(settings,'{endpoint}',to_jsonb($2::text)) WHERE id=$1`, account, server.URL); err != nil {
				t.Fatal(err)
			}
			definition := fmt.Sprintf(`{"templateVersion":1,"id":"owned_snapshot","name":"Owned","schema":"mTorrent","domain":%q,"api":%q,"requests":{"profile":{"parser":"JsonPath","path":"/profile","fields":{"username":{"selector":"data.username"}}},"search":{"parser":"JsonPath","method":"POST","path":"/search","body":{"mode":"owned-mode","keyword":"{keyword}","page":"{page}"},"list":{"selector":"data.items"},"fields":{"id":{"selector":"id"},"title":{"selector":"name"}}}}}`, server.URL, server.URL)
			if _, err := engine.DB.Exec(ctx, `INSERT INTO site_definitions(id,name,schema_name,source_commit,definition,common_definition,source_repository)VALUES('owned_snapshot','Owned','mTorrent','fixture',$1,'{}','dingding229/mediatool-site')`, json.RawMessage(definition)); err != nil {
				t.Fatal(err)
			}
			// mTorrent authentication requirements belong to the account, not defaults.
			repository := integrations.Repository{DB: engine.DB, Vault: engine.Vault}
			current, err := repository.Get(ctx, account)
			if err != nil {
				t.Fatal(err)
			}
			current.Record.Settings["userId"] = "1"
			if _, err = repository.Save(ctx, account, integrations.Input{Kind: "ptsite", Name: current.Record.Name, Enabled: true, Revision: current.Record.Revision, Settings: current.Record.Settings, Secrets: map[string]string{"apiKey": "PRIVATE-FIXTURE-KEY"}}); err != nil {
				t.Fatal(err)
			}
			manager := integrations.NewManager(repository, config.Config{})
			defer manager.Close()
			engine.Connections = manager
			runID := newRun()
			if _, err = engine.DB.Exec(ctx, `UPDATE search_runs SET account_ids=$2 WHERE id=$1`, runID, fmt.Sprintf(`[%q]`, account)); err != nil {
				t.Fatal(err)
			}
			if _, err = engine.DB.Exec(ctx, `INSERT INTO search_site_results(run_id,account_id)VALUES($1,$2)`, runID, account); err != nil {
				t.Fatal(err)
			}
			payload, _ := json.Marshal(map[string]string{"runId": runID})
			if err = engine.Process(ctx, jobs.Job{Payload: payload}); err != nil {
				t.Fatal(err)
			}
			run, err := engine.Get(ctx, runID, 1, 50)
			if err != nil || len(run.Resources) == 0 || len(run.Sites) != 1 {
				t.Fatal("no usable result retained", err)
			}
			if mode == "complete" {
				if run.State != "succeeded" || run.Sites[0].State != "succeeded" || len(run.Resources) != 2 {
					t.Fatal("page2 omitted or completion wrong", run.State, len(run.Resources))
				}
			} else {
				if run.State != "partial" || run.Sites[0].State != "partial" || run.Sites[0].Message == "" {
					t.Fatal("partial was promoted or failed", run.State, run.Sites[0].State)
				}
			}
			public, _ := json.Marshal(run)
			if strings.Contains(string(public), "PRIVATE-FIXTURE-KEY") {
				t.Fatal("provider credential leaked in outcome")
			}
			if err = engine.ValidateRunSources(ctx, runID); err != nil {
				t.Fatal("partial source lacks its origin proof", err)
			}
		})
	}
}

func TestPostgresUnsupportedExecutionCannotProduceSuccessfulEmptySearchProof(t *testing.T) {
	for _, mode := range []string{"browser_search", "browser_login", "unknown_parser", "raw_parser", "disabled_search"} {
		t.Run(mode, func(t *testing.T) {
			ctx, engine, account, newRun := snapshotSearchFixture(t)
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path == "/profile" {
					fmt.Fprint(w, `{"login":true}`)
				} else {
					fmt.Fprint(w, `{"items":[]}`)
				}
			}))
			defer server.Close()
			definition := map[string]any{"id": "owned_snapshot", "name": "Owned", "schema": "NexusPHP", "domain": server.URL, "requests": map[string]any{
				"user_basic_info": map[string]any{"path": "/profile", "parser": "JsonPath", "fields": map[string]any{"is_login": map[string]any{"selector": "login"}}},
				"search":          map[string]any{"path": "/search", "parser": "JsonPath", "list": map[string]any{"selector": "items"}, "fields": map[string]any{"id": map[string]any{"selector": "id"}, "title": map[string]any{"selector": "title"}}},
			}}
			requests := definition["requests"].(map[string]any)
			rule := requests["search"].(map[string]any)
			switch mode {
			case "browser_search":
				rule["chrome"] = true
			case "browser_login":
				requests["user_basic_info"].(map[string]any)["chrome"] = true
			case "unknown_parser":
				rule["parser"] = "PRIVATE-UNKNOWN-PARSER"
			case "raw_parser":
				rule["parser"] = "None"
			case "disabled_search":
				rule["disabled"] = true
			}
			raw, err := json.Marshal(definition)
			if err != nil {
				t.Fatal(err)
			}
			repo := integrations.Repository{DB: engine.DB, Vault: engine.Vault}
			current, err := repo.Get(ctx, account)
			if err != nil {
				t.Fatal(err)
			}
			current.Record.Settings["endpoint"] = server.URL
			if _, err = repo.Save(ctx, account, integrations.Input{Kind: "ptsite", Name: current.Record.Name, Enabled: true, Revision: current.Record.Revision, Settings: current.Record.Settings, Secrets: map[string]string{"cookie": "owned=PRIVATE-COOKIE"}}); err != nil {
				t.Fatal(err)
			}
			if _, err = engine.DB.Exec(ctx, `INSERT INTO site_definitions(id,name,schema_name,source_commit,definition,common_definition,source_repository)VALUES('owned_snapshot','Owned','NexusPHP','fixture',$1,'{}','dingding229/mediatool-site')`, json.RawMessage(raw)); err != nil {
				t.Fatal(err)
			}
			manager := integrations.NewManager(repo, config.Config{})
			defer manager.Close()
			engine.Connections = manager
			runID := newRun()
			if _, err = engine.DB.Exec(ctx, `UPDATE search_runs SET account_ids=$2 WHERE id=$1`, runID, fmt.Sprintf(`[%q]`, account)); err != nil {
				t.Fatal(err)
			}
			if _, err = engine.DB.Exec(ctx, `INSERT INTO search_site_results(run_id,account_id)VALUES($1,$2)`, runID, account); err != nil {
				t.Fatal(err)
			}
			payload, _ := json.Marshal(map[string]string{"runId": runID})
			if err = engine.Process(ctx, jobs.Job{Payload: payload}); err != nil {
				t.Fatal(err)
			}
			run, err := engine.Get(ctx, runID, 1, 50)
			if err != nil || run.State != "failed" || len(run.Sites) != 1 || run.Sites[0].State != "failed" || run.Sites[0].Message == "" || len(run.Resources) != 0 || calls.Load() != 0 {
				t.Fatal("unsupported execution recorded successful collection", calls.Load(), err)
			}
			var hasProof bool
			if err = engine.DB.QueryRow(ctx, `SELECT source_revision IS NOT NULL OR definition_fingerprint IS NOT NULL OR observed_at IS NOT NULL FROM search_site_results WHERE run_id=$1 AND account_id=$2`, runID, account).Scan(&hasProof); err != nil || hasProof {
				t.Fatal("failed execution manufactured source proof", err)
			}
			public, _ := json.Marshal(run)
			if strings.Contains(string(public), "PRIVATE-") {
				t.Fatal("configuration/credential appeared in public failure")
			}
		})
	}
}
