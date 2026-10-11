package sites

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ DB *pgxpool.Pool }

var ErrImportCommitUnconfirmed = errors.New("site import commit unconfirmed; inspect saved snapshot before retry")

func (r Repository) Get(ctx context.Context, id string) (Definition, error) {
	var d Definition
	var compatibility []byte
	err := r.DB.QueryRow(ctx, `SELECT id,name,schema_name,source_commit,definition,common_definition,compatibility FROM site_definitions WHERE id=$1`, id).Scan(&d.ID, &d.Name, &d.Schema, &d.SourceCommit, &d.RawJSON, &d.CommonJSON, &compatibility)
	if err != nil {
		return d, errors.New("site definition unavailable; import the matching configuration")
	}
	json.Unmarshal(compatibility, &d.Compatibility)
	d.refreshCompatibility()
	return d, nil
}
func (r Repository) List(ctx context.Context) ([]Definition, error) {
	rows, err := r.DB.Query(ctx, `SELECT id,name,schema_name,source_commit,definition,common_definition,compatibility FROM site_definitions ORDER BY name`)
	if err != nil {
		return nil, errors.New("site definition storage unavailable")
	}
	defer rows.Close()
	out := []Definition{}
	for rows.Next() {
		var d Definition
		var raw []byte
		if err = rows.Scan(&d.ID, &d.Name, &d.Schema, &d.SourceCommit, &d.RawJSON, &d.CommonJSON, &raw); err != nil {
			return nil, err
		}
		json.Unmarshal(raw, &d.Compatibility)
		d.refreshCompatibility()
		out = append(out, d)
	}
	return out, rows.Err()
}
func (d *Definition) refreshCompatibility() {
	configuration, err := DecodeStored(d.RawJSON, d.CommonJSON)
	if err != nil {
		note := "stored configuration invalid"
		code := "invalid_stored_definition"
		if errors.Is(err, ErrCaseOrderUnavailable) {
			note = "stored selector case priority lost; re-import the fixed snapshot"
			code = "selector_priority_unavailable"
		}
		d.Compatibility = Map{"state": "partial", "notes": []string{note}, "issueCodes": []string{code}, "accountVerified": false, "source": "pre-dessert-sites"}
		return
	}
	d.Compatibility = Compatibility(configuration)
}

const definitionInsert = `INSERT INTO site_definitions(id,name,schema_name,source_commit,definition,common_definition,compatibility)VALUES($1,$2,$3,$4,$5,$6,$7)ON CONFLICT(id)DO UPDATE SET name=excluded.name,schema_name=excluded.schema_name,source_commit=excluded.source_commit,definition=excluded.definition,common_definition=excluded.common_definition,compatibility=excluded.compatibility,updated_at=now()`

func prepareDefinition(raw, common []byte, commit string) (Definition, error) {
	raw, err := canonicalDefinitionJSON(raw)
	if err != nil {
		return Definition{}, err
	}
	common, err = canonicalDefinitionJSON(common)
	if err != nil {
		return Definition{}, err
	}
	config, err := DecodeStored(raw, common)
	if err != nil {
		return Definition{}, err
	}
	return Definition{ID: config.ID, Name: config.Name, Schema: config.Schema, SourceCommit: commit, RawJSON: raw, CommonJSON: common, Compatibility: Compatibility(config)}, nil
}
func definitionArguments(definition Definition) []any {
	compatibility, _ := json.Marshal(definition.Compatibility)
	return []any{definition.ID, definition.Name, definition.Schema, definition.SourceCommit, json.RawMessage(definition.RawJSON), json.RawMessage(definition.CommonJSON), compatibility}
}
func (r Repository) Save(ctx context.Context, raw, common []byte, commit string) (Definition, error) {
	definition, err := prepareDefinition(raw, common, commit)
	if err != nil {
		return Definition{}, err
	}
	if _, err = r.DB.Exec(ctx, definitionInsert, definitionArguments(definition)...); err != nil {
		return Definition{}, errors.New("site definition save failed")
	}
	return r.Get(ctx, definition.ID)
}

// ImportPinned resolves main once, then fetches only immutable source URLs.
func (r Repository) ImportPinned(ctx context.Context, ids []string) ([]Definition, error) {
	return r.ImportSnapshot(ctx, ids, "")
}

// ImportSnapshot can explicitly restore the same immutable commit. It never
// repairs lost case order by silently fetching a newer configuration.
func (r Repository) ImportSnapshot(ctx context.Context, ids []string, commit string) ([]Definition, error) {
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 3 || req.URL.Scheme != "https" || req.URL.User != nil || (req.URL.Host != "raw.githubusercontent.com" && req.URL.Host != "api.github.com") {
			return ErrConfig
		}
		return nil
	}}
	get := func(address string) ([]byte, error) {
		req, err := http.NewRequestWithContext(ctx, "GET", address, nil)
		if err != nil {
			return nil, ErrConfig
		}
		req.Header.Set("User-Agent", "media-tool")
		response, err := client.Do(req)
		if err != nil {
			return nil, errors.New("upstream site repository unreachable")
		}
		defer response.Body.Close()
		if response.StatusCode != 200 {
			return nil, errors.New("upstream configuration request failed")
		}
		raw, err := io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))
		if err != nil || len(raw) > 2<<20 {
			return nil, ErrConfig
		}
		return raw, nil
	}
	return r.importSnapshot(ctx, ids, commit, get)
}

func ValidateImport(ids []string, commit string) error {
	if len(ids) > 30 {
		return ErrConfig
	}
	pattern := regexp.MustCompile(`^[a-z0-9_]{1,80}$`)
	seen := map[string]bool{}
	for _, id := range ids {
		if !pattern.MatchString(id) || seen[id] {
			return ErrConfig
		}
		seen[id] = true
	}
	if commit != "" && !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(commit) {
		return ErrConfig
	}
	return nil
}

func (r Repository) importSnapshot(ctx context.Context, ids []string, commit string, get func(string) ([]byte, error)) ([]Definition, error) {
	if len(ids) == 0 {
		ids = []string{"hdsky", "ourbits", "mteam"}
	}
	if len(ids) > 30 {
		return nil, errors.New("import at most 30 site configurations per request")
	}
	if err := ValidateImport(ids, commit); err != nil {
		return nil, err
	}
	commitPattern := regexp.MustCompile(`^[a-f0-9]{40}$`)
	if commit == "" {
		raw, err := get("https://api.github.com/repos/mantou568/pre-dessert-sites/commits/main")
		if err != nil {
			return nil, err
		}
		var ref struct {
			SHA string `json:"sha"`
		}
		if json.Unmarshal(raw, &ref) != nil || !commitPattern.MatchString(ref.SHA) {
			return nil, ErrConfig
		}
		commit = ref.SHA
	}
	base := "https://raw.githubusercontent.com/mantou568/pre-dessert-sites/" + commit + "/site_config/"
	common, err := get(base + "commons/NexusPHP.json")
	if err != nil {
		return nil, err
	}
	type item struct {
		id  string
		raw []byte
		err error
	}
	items := make([]item, len(ids))
	var wait sync.WaitGroup
	semaphore := make(chan struct{}, 3)
	for index, id := range ids {
		wait.Add(1)
		go func(index int, id string) {
			defer wait.Done()
			select {
			case semaphore <- struct{}{}:
			case <-ctx.Done():
				items[index].err = ctx.Err()
				return
			}
			defer func() { <-semaphore }()
			raw, err := get(base + "sites/" + url.PathEscape(id) + ".json")
			items[index] = item{id, raw, err}
		}(index, id)
	}
	wait.Wait()
	definitions := make([]Definition, 0, len(items))
	// Validate EVERY definition before opening a write transaction. A failed
	// download, wrong ID, parser shape or case order never leaves a partial batch.
	for _, item := range items {
		if item.err != nil {
			return nil, item.err
		}
		definition, err := prepareDefinition(item.raw, common, commit)
		if err != nil {
			return nil, err
		}
		if definition.ID != item.id {
			return nil, ErrConfig
		}
		definitions = append(definitions, definition)
	}
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return nil, errors.New("site import transaction unavailable")
	}
	defer tx.Rollback(ctx)
	for _, definition := range definitions {
		if _, err = tx.Exec(ctx, definitionInsert, definitionArguments(definition)...); err != nil {
			return nil, errors.New("site import transaction failed")
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, ErrImportCommitUnconfirmed
	}
	return definitions, nil
}
