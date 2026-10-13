package search

import (
	"context"
	"encoding/json"
	"errors"
	"media-tool/internal/modules/releaseattrs"
	"strings"
	"unicode/utf8"
)

type Filter struct {
	Keyword    string `json:"keyword"`
	Sort       string `json:"sort"`
	Resolution string `json:"resolution"`
	Source     string `json:"source"`
	Codec      string `json:"codec"`
	AccountID  string `json:"accountId"`
	MinSize    int64  `json:"minSize"`
	MaxSize    int64  `json:"maxSize"`
	MinSeeders int    `json:"minSeeders"`
	FreeOnly   bool   `json:"freeOnly"`
}

func attrs(title string) map[string]string { return releaseattrs.Parse(title) }

func (f Filter) Validate() error {
	if len(f.Keyword) > 512 || !utf8.ValidString(f.Keyword) || strings.ContainsAny(f.Keyword, "\x00\r\n") {
		return errors.New("invalid keyword")
	}
	if _, ok := sortOrders[f.Sort]; !ok {
		return errors.New("invalid sort")
	}

	if f.MinSize < 0 || f.MaxSize < 0 || f.MinSize > 1<<60 || f.MaxSize > 1<<60 || f.MaxSize > 0 && f.MinSize > f.MaxSize || f.MinSeeders < 0 || f.MinSeeders > 1000000 || len(f.AccountID) > 80 {
		return errors.New("invalid filters")
	}
	for _, s := range []string{f.Resolution, f.Source, f.Codec} {
		if len(s) > 30 || strings.ContainsAny(s, "\x00\r\n") {
			return errors.New("invalid filters")
		}
	}
	return nil
}
func (f Filter) Match(r Resource) bool {
	if f.Keyword != "" {
		subtitle, _ := r.Metadata["subtitle"].(string)
		haystack := strings.ToLower(r.Title + " " + subtitle)
		for _, word := range strings.Fields(strings.ToLower(f.Keyword)) {
			if !strings.Contains(haystack, word) {
				return false
			}
		}
	}
	a := attrs(r.Title)
	for key, want := range map[string]string{"resolution": f.Resolution, "source": f.Source, "codec": f.Codec} {
		if want == "unknown" && a[key] != "" || want != "" && want != "unknown" && a[key] != strings.ToLower(want) {
			return false
		}
	}
	return (f.AccountID == "" || r.AccountID == f.AccountID) && r.Size >= f.MinSize && (f.MaxSize == 0 || r.Size <= f.MaxSize) && r.Seeders >= f.MinSeeders && (!f.FreeOnly || r.DownloadFactor != nil && *r.DownloadFactor == 0)
}

// Only constant whitelist SQL fragments are interpolated; user input never becomes SQL.
var sortOrders = map[string]string{
	"":             "(public_snapshot->>'seeders')::integer DESC",
	"seeders_desc": "(public_snapshot->>'seeders')::integer DESC",
	"seeders_asc":  "(public_snapshot->>'seeders')::integer ASC",
	"size_desc":    "(public_snapshot->>'size')::bigint DESC",
	"size_asc":     "(public_snapshot->>'size')::bigint ASC",
}

// GetFiltered enumerates the entire frozen result snapshot, not only one page.
// Use a bounded snapshot count and keep original download IDs untouched.
func (e Engine) GetFiltered(ctx context.Context, id string, page, limit int, f Filter) (Run, error) {
	if err := f.Validate(); err != nil {
		return Run{}, err
	}
	if page < 1 || page > 10000 || limit < 1 || limit > 200 {
		return Run{}, errors.New("invalid page")
	}
	run, err := e.Get(ctx, id, 1, 1)
	if err != nil {
		return run, err
	}
	rows, err := e.DB.Query(ctx, `SELECT public_snapshot FROM search_run_resources WHERE run_id=$1 ORDER BY `+sortOrders[f.Sort]+`,public_snapshot->>'title',resource_id`, id)
	if err != nil {
		return run, err
	}
	defer rows.Close()
	run.Resources = []Resource{}
	run.Total = 0
	start := (page - 1) * limit
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return run, err
		}
		var raw []byte
		var r Resource
		if err = rows.Scan(&raw); err != nil {
			return run, err
		}
		if json.Unmarshal(raw, &r) != nil {
			return run, ErrSnapshotUnavailable
		}
		if !f.Match(r) {
			continue
		}
		if run.Total >= start && len(run.Resources) < limit {
			if r.Metadata == nil {
				r.Metadata = map[string]any{}
			}
			r.Metadata["attributes"] = attrs(r.Title)
			run.Resources = append(run.Resources, r)
		}
		run.Total++
	}
	return run, rows.Err()
}
