package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuditIsStaticAndDoesNotExposeURLsOrDefinitions(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "commons"), 0o700)
	os.Mkdir(filepath.Join(root, "sites"), 0o700)
	os.WriteFile(filepath.Join(root, "commons/NexusPHP.json"), []byte(`{}`), 0o600)
	os.WriteFile(filepath.Join(root, "sites/owned.json"), []byte(`{"id":"owned","domain":"https://fixture-private-domain.invalid","schema":"NexusPHP"}`), 0o600)
	os.WriteFile(filepath.Join(root, "sites/bad.json"), []byte(`{"id":"different","domain":"https://other.invalid"}`), 0o600)
	result, err := audit(root, strings.Repeat("a", 40))
	if err != nil {
		t.Fatal(err)
	}
	if result.Definitions != 2 || result.Loaded != 1 || result.Invalid != 1 || result.ParserSupported != 1 || !result.StaticConfigurationOnly || result.AccountVerified {
		t.Fatal("audit scope incorrect")
	}
	encoded, _ := json.Marshal(result)
	if strings.Contains(string(encoded), "fixture-private") || strings.Contains(string(encoded), "https:") {
		t.Fatal("raw config or URL exposed")
	}
	if _, err = audit(root, "main"); err == nil {
		t.Fatal("mutable branch accepted as fixed source")
	}
}

func TestAuditReportsFixedIssueCodesAndDefinitionCountsWithoutRawRules(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "commons"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "sites"), 0o700); err != nil {
		t.Fatal(err)
	}
	fixtures := map[string]string{
		"commons/NexusPHP.json": `{}`,
		"sites/owned.json":      `{"id":"owned","domain":"https://PRIVATE-DOMAIN.invalid","schema":"NexusPHP","requests":{"search":{"chrome":true,"parser":"PRIVATE-PARSER","body":{"secret":"PRIVATE-BODY"},"fields":{"PRIVATE-FIELD":{"selector":"PRIVATE-SELECTOR","filters":[{"name":"timezone","args":["+19"]},{"name":"timezone","args":["+8"]},{"name":"PRIVATE-FILTER"}]}}}}}`,
		"sites/clean.json":      `{"id":"clean","domain":"https://PRIVATE-OTHER.invalid","schema":"NexusPHP","requests":{"search":{"parser":"JsonPath","body":{"filters":[{"name":"PRIVATE-BODY-FILTER"}]}}}}`,
	}
	for path, body := range fixtures {
		if err := os.WriteFile(filepath.Join(root, path), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := audit(root, strings.Repeat("a", 40))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "PRIVATE-") || strings.Contains(string(encoded), "https:") {
		t.Fatal("raw rules leaked")
	}
	var decoded struct {
		IssueDefinitions map[string]int `json:"issueDefinitionCounts"`
		Rows             []struct {
			ID    string   `json:"id"`
			Codes []string `json:"issueCodes"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if result.Loaded != 2 || result.ParserSupported != 1 || result.Partial != 1 || result.AccountVerified {
		t.Fatal("audit scope changed")
	}
	for _, code := range []string{"browser_execution_required", "unsupported_parser", "timezone_semantics_unverified", "unsupported_filter"} {
		if decoded.IssueDefinitions[code] != 1 {
			t.Fatal("missing or occurrence-based issue count", code)
		}
	}
	if len(decoded.IssueDefinitions) != 4 {
		t.Fatal("payload treated as extraction rules")
	}
	for _, row := range decoded.Rows {
		if row.ID == "owned" && len(row.Codes) != 4 {
			t.Fatal("per-definition reasons missing")
		}
		if row.ID == "clean" && len(row.Codes) != 0 {
			t.Fatal("healthy rule received issues")
		}
	}
}
