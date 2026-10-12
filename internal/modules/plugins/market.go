package plugins

import (
	"context"
	"io"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/playback"
	"net/http"
	"net/url"
	"time"
)

type Repository struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}
type MarketEntry struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Author      string `json:"author"`
	Description string `json:"description"`
	PackageURL  string `json:"packageUrl"`
	Digest      string `json:"digest"`
}

func publicFetch(ctx context.Context, target string, limit int64) ([]byte, error) {
	policy := playback.Policy{}
	if policy.Check(ctx, target) != nil {
		return nil, ErrInvalid
	}
	u, e := url.Parse(target)
	if e != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, ErrInvalid
	}
	req, e := http.NewRequestWithContext(ctx, "GET", target, nil)
	if e != nil {
		return nil, e
	}
	req.Header.Set("User-Agent", "MediaTool-PluginMarket/1")
	transport := &http.Transport{Proxy: nil, DialContext: policy.DialContext, TLSHandshakeTimeout: 3 * time.Second}
	defer transport.CloseIdleConnections()
	client := &http.Client{Timeout: 10 * time.Second, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, e := client.Do(req)
	if e != nil {
		return nil, e
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, ErrInvalid
	}
	raw, e := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if e != nil || int64(len(raw)) > limit {
		return nil, ErrInvalid
	}
	return raw, nil
}
func (r Registry) Repositories(ctx context.Context) ([]Repository, error) {
	rows, e := r.DB.Query(ctx, `SELECT id::text,name,url FROM extension_repositories ORDER BY created_at`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Repository{{ID: "official", Name: "Media Tool 示例仓库", URL: "https://raw.githubusercontent.com/dingding229/mediatool/main/examples/plugins/index.json"}}
	for rows.Next() {
		var x Repository
		if rows.Scan(&x.ID, &x.Name, &x.URL) != nil {
			return nil, ErrInvalid
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (r Registry) AddRepository(ctx context.Context, name, target string) error {
	if len(name) < 1 || len(name) > 100 {
		return ErrInvalid
	}
	if _, e := FetchCatalog(ctx, target); e != nil {
		return e
	}
	tag, e := r.DB.Exec(ctx, `INSERT INTO extension_repositories(id,name,url)SELECT $1,$2,$3 WHERE (SELECT count(*)FROM extension_repositories)<10`, jobs.UUID(), name, target)
	if e == nil && tag.RowsAffected() != 1 {
		return ErrInvalid
	}
	return e
}
func FetchCatalog(ctx context.Context, target string) ([]MarketEntry, error) {
	raw, e := publicFetch(ctx, target, 256<<10)
	if e != nil {
		return nil, e
	}
	var data struct {
		Contract int           `json:"contractVersion"`
		Plugins  []MarketEntry `json:"plugins"`
	}
	if strictJSON(raw, &data) != nil || data.Contract != 1 || len(data.Plugins) > 100 {
		return nil, ErrInvalid
	}
	seen := map[string]bool{}
	for i := range data.Plugins {
		x := &data.Plugins[i]
		if !identifier.MatchString(x.ID) || seen[x.ID] || len(x.Name) < 1 || len(x.Name) > 120 || len(x.Version) < 1 || len(x.Version) > 40 || len(x.Digest) != 64 {
			return nil, ErrInvalid
		}
		seen[x.ID] = true
		base, _ := url.Parse(target)
		ref, e := url.Parse(x.PackageURL)
		if e != nil {
			return nil, ErrInvalid
		}
		u := base.ResolveReference(ref)
		if u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return nil, ErrInvalid
		}
		x.PackageURL = u.String()
	}
	return data.Plugins, nil
}
func (r Registry) MarketPackage(ctx context.Context, repoID, pluginID string) (Package, error) {
	repos, e := r.Repositories(ctx)
	if e != nil {
		return Package{}, e
	}
	for _, repo := range repos {
		if repo.ID != repoID {
			continue
		}
		catalog, e := FetchCatalog(ctx, repo.URL)
		if e != nil {
			return Package{}, e
		}
		for _, entry := range catalog {
			if entry.ID != pluginID {
				continue
			}
			raw, e := publicFetch(ctx, entry.PackageURL, MaxPackageBytes)
			if e != nil {
				return Package{}, e
			}
			p, e := ParsePackage(raw)
			if e != nil || p.Digest != entry.Digest || p.Manifest.ID != entry.ID || p.Manifest.Version != entry.Version {
				return Package{}, ErrInvalid
			}
			return p, nil
		}
	}
	return Package{}, ErrInvalid
}
