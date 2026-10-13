package sites

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func isolatedSiteDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated TEST_DATABASE_URL required")
	}
	configuration, err := pgxpool.ParseConfig(dsn)
	if err != nil || configuration.ConnConfig.Database != "media_tool_test" {
		t.Fatal("only media_tool_test is permitted")
	}
	db, err := pgxpool.NewWithConfig(context.Background(), configuration)
	if err != nil {
		t.Fatal("isolated database unavailable")
	}
	t.Cleanup(db.Close)
	return db
}

func TestPostgresCasePrioritySurvivesJSONBAndLegacyNeedsExplicitReimport(t *testing.T) {
	db := isolatedSiteDatabase(t)
	ctx := context.Background()
	id := fmt.Sprintf("owned_case_%d", time.Now().UnixNano())
	raw := []byte(fmt.Sprintf(`{"templateVersion":1,"id":%q,"domain":"https://owned.invalid","schema":"NexusPHP","requests":{"search":{"parser":"CssSelector","fields":{"downloadvolumefactor":{"case":{".z-free":0,".a-half":0.5,"*":1}}}}}}`, id))
	repo := Repository{DB: db}
	definition, err := repo.Save(ctx, raw, []byte(`{}`), "owned-fixture")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec(ctx, `DELETE FROM site_definitions WHERE id=$1`, id) })
	configuration, err := DecodeStored(definition.RawJSON, definition.CommonJSON)
	if err != nil {
		t.Fatal(err)
	}
	document, _ := parseDocument([]byte(`<span class="a-half z-free">both</span>`), "CssSelector")
	values, err := mapFields(document, object(configuration.Requests["search"]["fields"]), nil, nil)
	if err != nil || text(values["downloadvolumefactor"]) != "0" {
		t.Fatal("jsonb changed priority", err, values)
	}
	if _, err = db.Exec(ctx, `UPDATE site_definitions SET definition=$2::jsonb WHERE id=$1`, id, json.RawMessage(raw)); err != nil {
		t.Fatal(err)
	}
	legacy, err := repo.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if legacy.Compatibility["state"] != "partial" {
		t.Fatal("legacy snapshot mislabeled supported")
	}
	if _, err = DecodeStored(legacy.RawJSON, legacy.CommonJSON); !errors.Is(err, ErrCaseOrderUnavailable) {
		t.Fatal("lost order guessed", err)
	}
	list, err := repo.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	matched := false
	for _, entry := range list {
		if entry.ID == id {
			matched = true
			if entry.Compatibility["state"] != "partial" {
				t.Fatal("list hides reimport requirement")
			}
		}
	}
	if !matched {
		t.Fatal("definition not listed")
	}
	restored, err := repo.Save(ctx, raw, []byte(`{}`), "owned-fixture")
	if err != nil {
		t.Fatal(err)
	}
	if restored.Compatibility["state"] != "parser_supported" {
		t.Fatal("explicit reimport did not restore priority")
	}
}

func TestPostgresImportBatchIsAtomicAndRefIsExplicit(t *testing.T) {
	db := isolatedSiteDatabase(t)
	ctx := context.Background()
	prefix := fmt.Sprintf("owned_batch_%d", time.Now().UnixNano())
	first, second := prefix+"_a", prefix+"_b"
	t.Cleanup(func() { db.Exec(ctx, `DELETE FROM site_definitions WHERE id=ANY($1::text[])`, []string{first, second}) })
	repo := Repository{DB: db}
	commit := strings.Repeat("a", 40)
	firstRaw := []byte(fmt.Sprintf(`{"templateVersion":1,"id":%q,"name":"First","domain":"https://owned.invalid","schema":"NexusPHP"}`, first))
	secondRaw := []byte(fmt.Sprintf(`{"templateVersion":1,"id":%q,"name":"Second","domain":"https://owned.invalid","schema":"NexusPHP"}`, second))
	hash := func(raw []byte) string { s := sha256.Sum256(raw); return hex.EncodeToString(s[:]) }
	catalog := Catalog{CatalogVersion: 1, Repository: NativeRepository, Templates: []CatalogEntry{{ID: first, Name: "First", File: "sites/" + first + ".json", SHA256: hash(firstRaw), TemplateVersion: 1}, {ID: second, Name: "Second", File: "sites/" + second + ".json", SHA256: hash(secondRaw), TemplateVersion: 1}}}
	catalogRaw, _ := json.Marshal(catalog)
	fetch := func(address string) ([]byte, error) {
		if !strings.HasPrefix(address, "https://raw.githubusercontent.com/"+NativeRepository+"/"+commit+"/") {
			return nil, errors.New("unexpected source")
		}
		switch {
		case strings.HasSuffix(address, "/index.json"):
			return catalogRaw, nil
		case strings.HasSuffix(address, "/sites/"+first+".json"):
			return firstRaw, nil
		case strings.HasSuffix(address, "/sites/"+second+".json"):
			return []byte(`{"id":"different_id","domain":"https://owned.invalid"}`), nil
		default:
			return nil, errors.New("unexpected request")
		}
	}
	if _, err := repo.importSnapshot(ctx, []string{first, second}, commit, fetch); !errors.Is(err, ErrConfig) {
		t.Fatal("mismatched source ID accepted", err)
	}
	var count int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM site_definitions WHERE id=ANY($1::text[])`, []string{first, second}).Scan(&count); err != nil || count != 0 {
		t.Fatal("failed batch left partial writes", count, err)
	}
	good := func(address string) ([]byte, error) {
		if strings.HasSuffix(address, "/sites/"+second+".json") {
			return secondRaw, nil
		}
		return fetch(address)
	}
	imported, err := repo.importSnapshot(ctx, []string{first, second}, commit, good)
	if err != nil || len(imported) != 2 {
		t.Fatal(err)
	}
	for _, definition := range imported {
		if definition.SourceCommit != commit || definition.Compatibility["accountVerified"] != false {
			t.Fatal("source ref or real-account claim wrong")
		}
	}
	if _, err := repo.importSnapshot(ctx, []string{first, first}, commit, func(string) ([]byte, error) { t.Error("invalid IDs triggered networking"); return nil, nil }); !errors.Is(err, ErrConfig) {
		t.Fatal("duplicate import IDs accepted")
	}
	if _, err := repo.importSnapshot(ctx, []string{first}, "../../main", func(string) ([]byte, error) { t.Error("invalid ref triggered networking"); return nil, nil }); !errors.Is(err, ErrConfig) {
		t.Fatal("invalid source ref accepted")
	}
}

func TestPostgresOriginalTemplatesRemainVisibleButCannotExecute(t *testing.T) {
	db := isolatedSiteDatabase(t)
	ctx := context.Background()
	id := fmt.Sprintf("owned_old_%d", time.Now().UnixNano())
	t.Cleanup(func() { db.Exec(ctx, `DELETE FROM site_definitions WHERE id=$1`, id) })
	raw := fmt.Sprintf(`{"id":%q,"domain":"https://owned.invalid","schema":"NexusPHP"}`, id)
	if _, e := db.Exec(ctx, `INSERT INTO site_definitions(id,name,source_commit,definition)VALUES($1,'Old',$2,$3)`, id, strings.Repeat("a", 40), json.RawMessage(raw)); e != nil {
		t.Fatal(e)
	}
	d, e := (Repository{DB: db}).Get(ctx, id)
	if e != nil || d.Compatibility["state"] != "unsupported" || d.SourceRepository != "" {
		t.Fatal(d, e)
	}
	if _, e := DecodeStored(d.RawJSON, d.CommonJSON); e == nil {
		t.Fatal("old definition executed")
	}
}
