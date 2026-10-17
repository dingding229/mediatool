package nativelibrary

import (
	"context"
	"errors"
	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/mediausers/contracts"
)

// Discovery modes are protocol operations, not persisted database states.
type DiscoveryMode string

const (
	Resume   DiscoveryMode = "resume"
	NextUp   DiscoveryMode = "nextup"
	Seasons  DiscoveryMode = "seasons"
	Episodes DiscoveryMode = "episodes"
	Latest   DiscoveryMode = "latest"
	Hints    DiscoveryMode = "hints"
	Genres   DiscoveryMode = "genres"
	Filters  DiscoveryMode = "filters"
	Similar  DiscoveryMode = "similar"
)

type DiscoveryQuery struct {
	MediaTypes                    []string
	Mode                          DiscoveryMode
	Items                         ItemQuery
	ItemID, SeasonID, StartItemID string
	Season                        *int
	Special                       *bool
	GroupItems                    *bool
}
type DiscoveryResult struct {
	Items   []dto.BaseItemDto
	Total   int
	Hints   dto.SearchHintResult
	Genres  []dto.NameIdPair
	Filters dto.ItemFilters
}
type DiscoveryService interface {
	Discover(context.Context, contracts.User, contracts.PermissionChecker, DiscoveryQuery) (DiscoveryResult, error)
}

var ErrDiscoveryTooBroad = errors.New("native discovery candidate limit exceeded")
