// Package catalog implements administrator-side native library synchronization.
// Protocol consumers depend on nativelibrary contracts, never this SQL package.
package catalog

import (
	"errors"
	"media-tool/internal/modules/classification"
	"media-tool/internal/modules/nativelibrary"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
)

var ErrInvalid = errors.New("invalid native library policy or granularity")
var ErrConflict = errors.New("native library policy revision conflict")

type Library struct {
	ID             string                       `json:"id"`
	ItemID         string                       `json:"itemId"`
	Name           string                       `json:"name"`
	CollectionType nativelibrary.CollectionType `json:"collectionType"`
	Directory      string                       `json:"directory"`
	Hidden         bool                         `json:"hidden"`
	SortOrder      int                          `json:"sortOrder"`
	GroupID        string                       `json:"groupId"`
	RuleID         string                       `json:"ruleId"`
	GroupName      string                       `json:"groupName"`
	RuleName       string                       `json:"ruleName"`
	AssetCount     int64                        `json:"assetCount"`
}
type Page struct {
	Items      []Library `json:"items"`
	Total      int64     `json:"total"`
	StartIndex int       `json:"startIndex"`
	Limit      int       `json:"limit"`
}

// Plan is deterministic, bounded by classification.Validate, and has no I/O.
func Plan(policy classification.Policy, granularity nativelibrary.LibraryGranularity) ([]Library, error) {
	if err := policy.Validate(); err != nil {
		return nil, ErrInvalid
	}
	if granularity != nativelibrary.GranularityRule && granularity != nativelibrary.GranularityGroup {
		return nil, ErrInvalid
	}
	out := []Library{}
	for _, g := range policy.Groups {
		if !utf8.ValidString(g.Name) || strings.ContainsFunc(g.Name, unicode.IsControl) {
			return nil, ErrInvalid
		}
		if !g.Enabled {
			continue
		}
		collection := nativelibrary.CollectionMovies
		if g.MediaType == "series" {
			collection = nativelibrary.CollectionTVShows
		}
		group := Library{ID: "g:" + g.ID, Name: g.Name, CollectionType: collection, Directory: g.Directory, GroupID: g.ID, GroupName: g.Name}
		count := 0
		if granularity == nativelibrary.GranularityRule {
			for _, r := range g.Rules {
				if !r.Enabled {
					continue
				}
				if !utf8.ValidString(r.Name) || strings.ContainsFunc(r.Name, unicode.IsControl) {
					return nil, ErrInvalid
				}
				item := group
				item.ID = "r:" + g.ID + ":" + r.ID
				item.RuleID = r.ID
				item.RuleName = r.Name
				item.Name = r.Name
				item.Directory = path.Join(g.Directory, r.Directory)
				out = append(out, item)
				count++
			}
		}
		if count == 0 {
			out = append(out, group)
		}
	}
	counts := map[string]int{}
	for _, item := range out {
		counts[item.Name]++
	}
	for i := range out {
		if counts[out[i].Name] > 1 {
			out[i].Name += " · " + out[i].GroupName
		}
	}
	counts = map[string]int{}
	for _, item := range out {
		counts[item.Name]++
	}
	for i := range out {
		if counts[out[i].Name] > 1 {
			suffix := " · " + out[i].ID
			runes := []rune(out[i].Name)
			if len(runes) > 256-utf8.RuneCountInString(suffix) {
				runes = runes[:256-utf8.RuneCountInString(suffix)]
			}
			out[i].Name = string(runes) + suffix
		}
		out[i].SortOrder = i
	}
	return out, nil
}
