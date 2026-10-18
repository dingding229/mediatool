package catalog

import (
	"context"
	"github.com/jackc/pgx/v5"
	dbutil "media-tool/internal/platform/database"
	"media-tool/internal/platform/pluginactor"
)

// Plugin catalog is an administrative projection of confirmed/exposable assets,
// NOT BrowseRepository's member realm. No synthetic media_users/VisibleLibraries
// are constructed. PersonalData/NextUp require a real, authenticated member.
type PluginLibrary struct {
	ID, Name, MediaType string
	Count               int64
}
type PluginItem struct {
	ID, Title, Year, MediaType, Overview string
	Season, Episode                      *int
}

func (r Repository) PluginLibraries(ctx context.Context, start, limit int) ([]PluginLibrary, int, error) {
	a, err := pluginactor.Require(ctx, "mediaserver.read")
	if err != nil {
		return nil, 0, err
	}
	if start < 0 || start > 200000 || limit < 1 || limit > 200 {
		return nil, 0, ErrInvalid
	}
	var total int
	where := ` FROM native_libraries l WHERE NOT l.hidden AND(coalesce(cardinality($1::text[]),0)=0 OR l.id=ANY($1))`
	if err = r.DB.QueryRow(ctx, `SELECT count(*)`+where, a.Libraries()).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.DB.Query(ctx, `SELECT l.id,l.name,CASE WHEN l.collection_type='movies' THEN 'movie' ELSE 'series' END,(SELECT count(*)FROM native_asset_libraries al JOIN (`+ExposableAssetsSQL+`) e ON e.asset_id=al.asset_id WHERE al.library_id=l.id)`+where+` ORDER BY l.sort_order,l.id LIMIT $2 OFFSET $3`, a.Libraries(), limit, start)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []PluginLibrary{}
	for rows.Next() {
		var v PluginLibrary
		if err = rows.Scan(&v.ID, &v.Name, &v.MediaType, &v.Count); err != nil {
			return nil, 0, err
		}
		out = append(out, v)
	}
	return out, total, rows.Err()
}

const pluginItemsSQL = ` FROM (` + ExposableAssetsSQL + `) e JOIN native_asset_libraries al ON al.asset_id=e.asset_id JOIN native_libraries l ON l.id=al.library_id WHERE NOT l.hidden AND(coalesce(cardinality($1::text[]),0)=0 OR l.id=ANY($1))AND($2=''OR l.id=$2)AND($3=''OR e.asset_id::text=$3)AND($4=''OR position(lower($4)IN lower(e.title))>0)AND($5=''OR e.media_type=$5)`

func (r Repository) PluginItems(ctx context.Context, library, id, text, kind string, start, limit int) ([]PluginItem, int, error) {
	a, err := pluginactor.Require(ctx, "mediaserver.read")
	if err != nil {
		return nil, 0, err
	}
	if library != "" && !a.Allows(library) {
		return nil, 0, pluginactor.ErrDenied
	}
	if start < 0 || start > 200000 || limit < 1 || limit > 200 || len([]rune(text)) > 200 {
		return nil, 0, ErrInvalid
	}
	args := []any{a.Libraries(), library, id, text, kind}
	var total int
	if err = r.DB.QueryRow(ctx, `SELECT count(*)`+pluginItemsSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.DB.Query(ctx, `SELECT e.asset_id::text,e.title,e.release_year,e.media_type,coalesce(e.metadata->>'overview',''),e.season,e.episode`+pluginItemsSQL+` ORDER BY e.title,e.asset_id LIMIT $6 OFFSET $7`, append(args, limit, start)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []PluginItem{}
	for rows.Next() {
		var v PluginItem
		if err = rows.Scan(&v.ID, &v.Title, &v.Year, &v.MediaType, &v.Overview, &v.Season, &v.Episode); err != nil {
			return nil, 0, err
		}
		out = append(out, v)
	}
	return out, total, rows.Err()
}
func (r Repository) PluginRefreshAsset(ctx context.Context, id string) error {
	a, err := pluginactor.Require(ctx, "mediaserver.refresh")
	if err != nil {
		return err
	}
	var ok bool
	err = r.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM (`+ExposableAssetsSQL+`) e JOIN native_asset_libraries al ON al.asset_id=e.asset_id JOIN native_libraries l ON l.id=al.library_id WHERE e.asset_id=$1 AND NOT l.hidden AND(coalesce(cardinality($2::text[]),0)=0 OR l.id=ANY($2)))`, id, a.Libraries()).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return pgx.ErrNoRows
	}
	return nil
}

// PluginRefreshContext rechecks asset membership inside the business commit,
// after the ordinary live-grant fence, not only in a preflight read.
func (r Repository) PluginRefreshContext(ctx context.Context, id string) (context.Context, error) {
	if err := r.PluginRefreshAsset(ctx, id); err != nil {
		return nil, err
	}
	a, err := pluginactor.Require(ctx, "mediaserver.refresh")
	if err != nil {
		return nil, err
	}
	return dbutil.WithCommitGuard(ctx, func(c context.Context, tx pgx.Tx) error {
		if err := dbutil.CheckCommit(ctx, tx); err != nil {
			return err
		}
		var allowed string
		err := tx.QueryRow(c, `SELECT al.library_id FROM (`+ExposableAssetsSQL+`) e JOIN native_asset_libraries al ON al.asset_id=e.asset_id JOIN native_libraries l ON l.id=al.library_id WHERE e.asset_id=$1 AND NOT l.hidden AND(coalesce(cardinality($2::text[]),0)=0 OR l.id=ANY($2)) FOR SHARE OF al,l`, id, a.Libraries()).Scan(&allowed)
		if err != nil {
			return err
		}
		if allowed == "" {
			return pluginactor.ErrDenied
		}
		return nil
	}), nil
}
