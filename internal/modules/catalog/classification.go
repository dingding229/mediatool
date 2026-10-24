package catalog

import (
	"fmt"
	"strconv"
	"strings"

	"media-tool/internal/modules/classification"
	"media-tool/internal/modules/metadata"
)

// Category IDs follow configured identities, never translated genre names.
type Category struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Group string `json:"group"`
}

func categoryKey(group, rule string) string {
	if group == "" {
		return "unclassified"
	}
	if rule == "" {
		return "g:" + group
	}
	return "r:" + group + ":" + rule
}
func categoryOptions(p classification.Policy) []Category {
	out := []Category{}
	for _, g := range p.Groups {
		if !g.Enabled {
			continue
		}
		hasRules := false
		for _, r := range g.Rules {
			if r.Enabled {
				hasRules = true
				out = append(out, Category{categoryKey(g.ID, r.ID), g.Name + " / " + r.Name, g.Name})
			}
		}
		if !hasRules {
			out = append(out, Category{categoryKey(g.ID, ""), g.Name, g.Name})
		}
	}
	return out
}
func categoryFor(p classification.Policy, item metadata.Item) (string, string) {
	m := p.Match(item)
	if m.GroupID == "" {
		return "unclassified", "未分类"
	}
	return categoryKey(m.GroupID, m.RuleID), m.Name
}

// Compile the same ordered first-match policy into a parameterized CASE so
// classification filtering and totals happen BEFORE grouping and SQL paging.
// Differential tests compare every result with classification.Policy.Match.
func categorySQL(p classification.Policy, args *[]any) string {
	bind := func(v any) string { *args = append(*args, v); return fmt.Sprintf("$%d", len(*args)) }
	array := func(key string, values []string) string {
		if len(values) == 0 {
			return "true"
		}
		return "EXISTS(SELECT 1 FROM jsonb_array_elements_text(CASE WHEN jsonb_typeof(m.metadata->'" + key + "')='array' THEN m.metadata->'" + key + "' ELSE '[]'::jsonb END) v WHERE v=ANY(" + bind(values) + "::text[]))"
	}
	genres := func(ids []int) string {
		values := make([]string, len(ids))
		for i, id := range ids {
			values[i] = strconv.Itoa(id)
		}
		return array("genreIds", values)
	}
	cases := []string{}
	for _, g := range p.Groups {
		if !g.Enabled {
			continue
		}
		group := "m.media_type=" + bind(g.MediaType) + " AND " + genres(g.GenreIDs)
		hasRules := false
		for _, r := range g.Rules {
			if !r.Enabled {
				continue
			}
			hasRules = true
			condition := group + " AND " + genres(r.GenreIDs) + " AND " + array("countries", r.Countries)
			if len(r.Languages) > 0 {
				condition += " AND coalesce(m.metadata->>'originalLanguage','')=ANY(" + bind(r.Languages) + "::text[])"
			}
			cases = append(cases, "WHEN "+condition+" THEN "+bind(categoryKey(g.ID, r.ID))+"::text")
		}
		if !hasRules {
			cases = append(cases, "WHEN "+group+" THEN "+bind(categoryKey(g.ID, ""))+"::text")
		}
	}
	if len(cases) == 0 {
		return "'unclassified'::text"
	}
	return "CASE " + strings.Join(cases, " ") + " ELSE 'unclassified'::text END"
}
