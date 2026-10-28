package catalog

import (
	"context"
	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/modules/nativelibrary"
)

// ImageVisible applies the same live library scope as browsing,
// before consulting even a cached image or answering a conditional request.
func (r BrowseRepository) ImageVisible(ctx context.Context, u contracts.User, p contracts.PermissionChecker, id string) (bool, error) {
	q := nativelibrary.ItemQuery{IDs: []string{id}}
	_, where, args, err := r.plan(ctx, u, p, q)
	if err != nil {
		return false, err
	}
	sql, args := pruneHeadersSQL(browseCTE, q, args)
	var visible bool
	err = r.DB.QueryRow(ctx, sql+`SELECT EXISTS(SELECT 1 FROM item_rows WHERE `+where+`)`, args...).Scan(&visible)
	return visible, err
}

// ImageScope reuses browse's live grant scope. Filtering
// happens before any bounded person-candidate page, not after truncating titles.
func ImageScope(ctx context.Context, u contracts.User, p contracts.PermissionChecker) (libraries []string, err error) {
	s, err := scope(ctx, u, p)
	return s.ids, err
}
