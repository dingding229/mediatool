package mediaserver

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/nativelibrary/catalog"
	"media-tool/internal/platform/pluginactor"
)

// PluginCatalog is deliberately narrower than Provider: no Poster/Playback,
// upstream handler, URL builder, session activation or HTTP client is exposed.
type PluginCatalog struct {
	DB       *pgxpool.Pool
	Registry *Registry
}

func (s PluginCatalog) Libraries(ctx context.Context, provider string, start, limit int) ([]catalog.PluginLibrary, int, error) {
	if _, err := pluginactor.Require(ctx, "mediaserver.read"); err != nil {
		return nil, 0, err
	}
	if provider != "builtin" {
		return nil, 0, ErrUnsupported
	}
	return (catalog.Repository{DB: s.DB}).PluginLibraries(ctx, start, limit)
}
func (s PluginCatalog) Items(ctx context.Context, provider, library, id, text, kind string, page, size int) ([]Item, int, error) {
	a, err := pluginactor.Require(ctx, "mediaserver.read")
	if err != nil {
		return nil, 0, err
	}
	if library != "" && !a.Allows(library) {
		return nil, 0, ErrDenied
	}
	if provider == "builtin" {
		v, n, err := (catalog.Repository{DB: s.DB}).PluginItems(ctx, library, id, text, kind, (page-1)*size, size)
		out := []Item{}
		for _, i := range v {
			out = append(out, Item{ID: i.ID, Title: i.Title, Year: i.Year, MediaType: i.MediaType, Overview: i.Overview, Season: i.Season, Episode: i.Episode})
		}
		return out, n, err
	}
	return nil, 0, ErrUnsupported
}
