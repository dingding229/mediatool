package search

import (
	"context"
	"media-tool/internal/modules/sites"
)

type Service interface {
	Search(context.Context, sites.SearchQuery, []string) ([]sites.Resource, error)
}
