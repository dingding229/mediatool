package library

import (
	"context"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"media-tool/internal/modules/metadata"
	pluginv2 "media-tool/internal/modules/plugins/v2"
	dbutil "media-tool/internal/platform/database"
)

// Only search-verified identity pairs cross the policy boundary; artwork, paths,
// file evidence and auto-confirmation are deliberately not represented.
func (e Engine) convertMedia(ctx context.Context, items []metadata.Item, selected *metadata.Item) *metadata.Item {
	if e.Connections == nil || len(items) > 200 {
		return selected
	}
	q := contracts.MediaConversion{Candidates: []contracts.MediaIdentity{}}
	for _, v := range items {
		q.Candidates = append(q.Candidates, contracts.MediaIdentity{TMDBID: v.ID, MediaType: v.MediaType})
	}
	if selected != nil {
		q.Selected = &contracts.MediaIdentity{TMDBID: selected.ID, MediaType: selected.MediaType}
	}
	prepared, choice, err := (pluginv2.Store{DB: e.DB, Vault: e.Connections.Repository.Vault}).ConvertMedia(ctx, q)
	if err != nil || dbutil.CheckPrepared(prepared, e.DB) != nil {
		return selected
	}
	if choice == nil {
		return selected
	}
	for _, v := range items {
		if v.ID == choice.TMDBID && v.MediaType == choice.MediaType {
			return &v
		}
	}
	return selected
}
func (e Engine) convertSuggestions(ctx context.Context, items []metadata.Item) []metadata.Item {
	chosen := e.convertMedia(ctx, items, nil)
	if chosen == nil {
		return items
	}
	out := []metadata.Item{*chosen}
	for _, v := range items {
		if v.ID != chosen.ID || v.MediaType != chosen.MediaType {
			out = append(out, v)
		}
	}
	return out
}
