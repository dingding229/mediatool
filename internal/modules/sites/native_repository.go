package sites

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"sync"
	"time"
)

const NativeRepository = "dingding229/mediatool-site"
const NativeRepositoryURL = "https://github.com/" + NativeRepository
const NativeTemplateVersion = 1

type CatalogEntry struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	File            string `json:"file"`
	SHA256          string `json:"sha256"`
	TemplateVersion int    `json:"templateVersion"`
}
type Catalog struct {
	CatalogVersion int            `json:"catalogVersion"`
	Repository     string         `json:"repository"`
	Templates      []CatalogEntry `json:"templates"`
	SourceCommit   string         `json:"sourceCommit,omitempty"`
}

func ValidateImport(ids []string, commit string) error {
	if len(ids) > 30 {
		return ErrConfig
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if !regexp.MustCompile(`^[a-z0-9_]{1,80}$`).MatchString(id) || seen[id] {
			return ErrConfig
		}
		seen[id] = true
	}
	if commit != "" && !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(commit) {
		return ErrConfig
	}
	return nil
}
func ParseCatalog(raw []byte) (Catalog, error) {
	var c Catalog
	if len(raw) == 0 || len(raw) > 256<<10 {
		return c, ErrConfig
	}
	// Ordered decoding rejects duplicate keys and trailing JSON before typed decode.
	if _, e := decodeDefinition(raw, false); e != nil {
		return c, ErrConfig
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&c) != nil || c.CatalogVersion != 1 || c.Repository != NativeRepository || c.SourceCommit != "" || len(c.Templates) < 1 || len(c.Templates) > 100 {
		return c, ErrConfig
	}
	ids := map[string]bool{}
	for _, entry := range c.Templates {
		if ValidateImport([]string{entry.ID}, "") != nil || ids[entry.ID] || entry.File != "sites/"+entry.ID+".json" || entry.TemplateVersion != NativeTemplateVersion || entry.Name == "" || len(entry.Name) > 120 || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(entry.SHA256) {
			return c, ErrConfig
		}
		ids[entry.ID] = true
	}
	return c, nil
}
func nativeFetch(ctx context.Context, address string) ([]byte, error) {
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, e := http.NewRequestWithContext(ctx, "GET", address, nil)
	if e != nil {
		return nil, ErrConfig
	}
	req.Header.Set("User-Agent", "MediaTool-NativeSite/1")
	response, e := client.Do(req)
	if e != nil {
		return nil, errors.New("native site repository unreachable")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, errors.New("native repository request failed")
	}
	raw, e := io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))
	if e != nil || len(raw) > 2<<20 {
		return nil, ErrConfig
	}
	return raw, nil
}
func catalogSnapshot(commit string, get func(string) ([]byte, error)) (Catalog, error) {
	if ValidateImport(nil, commit) != nil {
		return Catalog{}, ErrConfig
	}
	if commit == "" {
		raw, e := get("https://api.github.com/repos/" + NativeRepository + "/commits/main")
		if e != nil {
			return Catalog{}, e
		}
		var ref struct {
			SHA string `json:"sha"`
		}
		if json.Unmarshal(raw, &ref) != nil || ValidateImport(nil, ref.SHA) != nil || ref.SHA == "" {
			return Catalog{}, ErrConfig
		}
		commit = ref.SHA
	}
	raw, e := get("https://raw.githubusercontent.com/" + NativeRepository + "/" + commit + "/index.json")
	if e != nil {
		return Catalog{}, e
	}
	catalog, e := ParseCatalog(raw)
	if e != nil {
		return Catalog{}, e
	}
	catalog.SourceCommit = commit
	return catalog, nil
}
func RepositoryCatalog(ctx context.Context, commit string) (Catalog, error) {
	return catalogSnapshot(commit, func(address string) ([]byte, error) { return nativeFetch(ctx, address) })
}
func (r Repository) ImportPinned(ctx context.Context, ids []string) ([]Definition, error) {
	return r.ImportSnapshot(ctx, ids, "")
}
func (r Repository) ImportSnapshot(ctx context.Context, ids []string, commit string) ([]Definition, error) {
	return r.importSnapshot(ctx, ids, commit, func(address string) ([]byte, error) { return nativeFetch(ctx, address) })
}
func (r Repository) importSnapshot(ctx context.Context, ids []string, commit string, get func(string) ([]byte, error)) ([]Definition, error) {
	if e := ValidateImport(ids, commit); e != nil {
		return nil, e
	}
	catalog, e := catalogSnapshot(commit, get)
	if e != nil {
		return nil, e
	}
	if len(ids) == 0 {
		for _, entry := range catalog.Templates {
			ids = append(ids, entry.ID)
		}
	}
	if e := ValidateImport(ids, catalog.SourceCommit); e != nil {
		return nil, e
	}
	byID := map[string]CatalogEntry{}
	for _, entry := range catalog.Templates {
		byID[entry.ID] = entry
	}
	for _, id := range ids {
		if _, ok := byID[id]; !ok {
			return nil, ErrConfig
		}
	}
	type result struct {
		definition Definition
		err        error
	}
	items := make([]result, len(ids))
	var wait sync.WaitGroup
	limit := make(chan struct{}, 3)
	for i, id := range ids {
		wait.Add(1)
		go func(i int, id string) {
			defer wait.Done()
			select {
			case limit <- struct{}{}:
			case <-ctx.Done():
				items[i].err = ctx.Err()
				return
			}
			defer func() { <-limit }()
			entry := byID[id]
			raw, err := get("https://raw.githubusercontent.com/" + NativeRepository + "/" + catalog.SourceCommit + "/" + entry.File)
			if err != nil {
				items[i].err = err
				return
			}
			hash := sha256.Sum256(raw)
			if hex.EncodeToString(hash[:]) != entry.SHA256 {
				items[i].err = ErrConfig
				return
			}
			definition, err := prepareDefinition(raw, []byte(`{}`), catalog.SourceCommit)
			if err == nil && (definition.ID != id || definition.Name != entry.Name) {
				err = ErrConfig
			}
			items[i] = result{definition, err}
		}(i, id)
	}
	wait.Wait()
	definitions := make([]Definition, 0, len(items))
	for _, item := range items {
		if item.err != nil {
			return nil, item.err
		}
		definitions = append(definitions, item.definition)
	}
	if r.DB == nil {
		return nil, errors.New("native site template storage unavailable")
	}
	tx, e := r.DB.Begin(ctx)
	if e != nil {
		return nil, errors.New("site import transaction unavailable")
	}
	defer tx.Rollback(ctx)
	for _, definition := range definitions {
		if _, e = tx.Exec(ctx, definitionInsert, definitionArguments(definition)...); e != nil {
			return nil, errors.New("site import transaction failed")
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, ErrImportCommitUnconfirmed
	}
	return definitions, nil
}
