package nativelibrary

import (
	"context"
	"errors"
	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/mediausers/contracts"
)

// BrowseService extends T14/T15 with cross-library SQL paging. No SQL driver is
// exposed to the protocol. Callers supply only T41 identities and a T42 scope.
type BrowseService interface {
	Views(context.Context, contracts.User, contracts.PermissionChecker) ([]dto.BaseItemDto, error)
	Query(context.Context, contracts.User, contracts.PermissionChecker, ItemQuery) ([]dto.BaseItemDto, int, error)
	Counts(context.Context, contracts.User, contracts.PermissionChecker, ...ItemQuery) (ItemCounts, error)
}
type ItemCounts struct{ MovieCount, SeriesCount, EpisodeCount int }

var ErrInvalidQuery = errors.New("invalid native browse query")
