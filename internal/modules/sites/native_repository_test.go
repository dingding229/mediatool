package sites

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestNativeContractRejectsOldFormatAndInheritance(t *testing.T) {
	good := `{"templateVersion":1,"id":"owned","name":"Owned","domain":"https://owned.invalid","schema":"NexusPHP","requests":{}}`
	if _, e := Decode([]byte(good), nil); e != nil {
		t.Fatal(e)
	}
	bad := []string{strings.Replace(good, `"templateVersion":1,`, "", 1), strings.Replace(good, `"templateVersion":1`, `"templateVersion":2`, 1), strings.Replace(good, `"requests":{}`, `"reuse_schema":"NexusPHP","requests":{}`, 1), strings.Replace(good, `"requests":{}`, `"requests":{"search":{"disabled_extends":{"params":true}}}`, 1)}
	for _, raw := range bad {
		if _, e := Decode([]byte(raw), nil); e == nil {
			t.Fatal("legacy accepted", raw)
		}
	}
	if _, e := Decode([]byte(good), []byte(`{"requests":{}}`)); e == nil {
		t.Fatal("common inheritance")
	}
}
func TestNativeCatalogRejectsUnknownSourcesVersionsPathsAndDuplicates(t *testing.T) {
	raw, e := os.ReadFile("testdata/repository/index.json")
	if e != nil {
		t.Fatal(e)
	}
	c, e := ParseCatalog(raw)
	if e != nil {
		t.Fatal(e)
	}
	for _, kind := range []string{"old_repo", "wrong_version", "path", "url", "duplicate", "digest", "entry_version", "extra"} {
		t.Run(kind, func(t *testing.T) {
			var j map[string]any
			json.Unmarshal(raw, &j)
			entries := j["templates"].([]any)
			entry := entries[0].(map[string]any)
			switch kind {
			case "old_repo":
				j["repository"] = "mantou568/pre-dessert-sites"
			case "wrong_version":
				j["catalogVersion"] = 2
			case "path":
				entry["file"] = "../sites/owned.json"
			case "url":
				entry["file"] = "https://elsewhere.invalid/owned.json"
			case "duplicate":
				j["templates"] = append(entries, entries[0])
			case "digest":
				entry["sha256"] = "bad"
			case "entry_version":
				entry["templateVersion"] = 2
			case "extra":
				j["commonFile"] = "commons/NexusPHP.json"
			}
			b, _ := json.Marshal(j)
			if _, e := ParseCatalog(b); e == nil {
				t.Fatal("catalog accepted", kind)
			}
		})
	}
	if c.Repository != NativeRepository {
		t.Fatal(c)
	}
}
func TestNativeSnapshotPinsOneRepositoryBeforeSelectingFiles(t *testing.T) {
	raw, _ := os.ReadFile("testdata/repository/index.json")
	sha := strings.Repeat("b", 40)
	calls := []string{}
	c, e := catalogSnapshot("", func(address string) ([]byte, error) {
		calls = append(calls, address)
		if address == "https://api.github.com/repos/"+NativeRepository+"/commits/main" {
			return []byte(`{"sha":"` + sha + `"}`), nil
		}
		if address == "https://raw.githubusercontent.com/"+NativeRepository+"/"+sha+"/index.json" {
			return raw, nil
		}
		t.Fatal("other source", address)
		return nil, ErrConfig
	})
	if e != nil || c.SourceCommit != sha || len(calls) != 2 {
		t.Fatal(c, e, calls)
	}
	if _, e = catalogSnapshot("main", func(string) ([]byte, error) { t.Fatal("invalid ref fetched"); return nil, nil }); e == nil {
		t.Fatal("mutable source")
	}
}
func TestNativeImportRejectsLegacyEvenWithMatchingHashBeforeStorage(t *testing.T) {
	bad := []byte(`{"id":"owned","name":"Owned","domain":"https://owned.invalid","schema":"NexusPHP"}`)
	sum := sha256.Sum256(bad)
	catalog := Catalog{CatalogVersion: 1, Repository: NativeRepository, Templates: []CatalogEntry{{ID: "owned", Name: "Owned", File: "sites/owned.json", SHA256: hex.EncodeToString(sum[:]), TemplateVersion: 1}}}
	raw, _ := json.Marshal(catalog)
	_, e := (Repository{}).importSnapshot(context.Background(), []string{"owned"}, strings.Repeat("a", 40), func(address string) ([]byte, error) {
		if strings.HasSuffix(address, "/index.json") {
			return raw, nil
		}
		return bad, nil
	})
	if !errors.Is(e, ErrConfig) {
		t.Fatal("legacy reached storage", e)
	}
}

func TestNativeImportBadHashAndMissingIDNeverWrite(t *testing.T) {
	raw, _ := os.ReadFile("testdata/repository/index.json")
	commit := strings.Repeat("c", 40)
	_, e := (Repository{}).importSnapshot(context.Background(), []string{"hdsky"}, commit, func(address string) ([]byte, error) {
		if strings.HasSuffix(address, "/index.json") {
			return raw, nil
		}
		return []byte(`{"templateVersion":1,"id":"hdsky","name":"altered","domain":"https://owned.invalid"}`), nil
	})
	if !errors.Is(e, ErrConfig) {
		t.Fatal("hash mismatch reached storage", e)
	}
	fetches := 0
	_, e = (Repository{}).importSnapshot(context.Background(), []string{"unknown"}, commit, func(address string) ([]byte, error) {
		fetches++
		if !strings.HasSuffix(address, "/index.json") {
			t.Fatal("unknown file fetched")
		}
		return raw, nil
	})
	if !errors.Is(e, ErrConfig) || fetches != 1 {
		t.Fatal("missing catalog ID", e, fetches)
	}
}
