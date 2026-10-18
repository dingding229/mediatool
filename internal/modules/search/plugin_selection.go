package search

import (
	"context"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	pluginv2 "media-tool/internal/modules/plugins/v2"
)

// PrepareSelection retains the exact host-owned resource objects. Only their
// opaque IDs cross the hook; hints, authenticated URLs and metadata never do.
// The caller carries the returned fence into any resulting business write.
func (e Engine) PrepareSelection(ctx context.Context, run string, items []Resource) (context.Context, []Resource, error) {
	q := contracts.ResourceSelectionRequest{RunID: run, Candidates: []contracts.ResourceCandidate{}}
	byID := map[string]Resource{}
	for _, r := range items {
		q.Candidates = append(q.Candidates, contracts.ResourceCandidate{ID: r.ID, AccountID: r.AccountID, Size: r.Size, Seeders: r.Seeders})
		byID[r.ID] = r
	}
	ctx, ids, err := (pluginv2.Store{DB: e.DB, Vault: e.Vault}).SelectResources(ctx, q)
	if err != nil {
		return ctx, nil, err
	}
	out := []Resource{}
	for _, id := range ids {
		out = append(out, byID[id])
	}
	return ctx, out, nil
}
