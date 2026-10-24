package subtitles

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"media-tool/internal/platform/mediaext"
)

func (r Repository) List(ctx context.Context, asset string) (Page, error) {
	out := Page{AssetID: asset, Versions: []Version{}, Items: []Entry{}}
	if e := r.asset(ctx, asset); e != nil {
		return out, e
	}
	rows, e := r.DB.Query(ctx, `SELECT a.id::text,regexp_replace(f.full_path,'^.*/','') FROM media_asset_bindings b JOIN media_assets a ON a.id=b.asset_id JOIN cloud_files f ON f.id=a.cloud_file_id JOIN media_asset_bindings chosen ON chosen.asset_id=$1 WHERE b.media_id=chosen.media_id AND b.season IS NOT DISTINCT FROM chosen.season AND b.episode IS NOT DISTINCT FROM chosen.episode ORDER BY a.id LIMIT 101`, asset)
	if e != nil {
		return out, ErrUnavailable
	}
	for rows.Next() {
		var v Version
		if e = rows.Scan(&v.ID, &v.Name); e != nil {
			rows.Close()
			return out, ErrUnavailable
		}
		out.Versions = append(out.Versions, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, ErrUnavailable
	}
	if len(out.Versions) > 100 {
		return out, ErrInvalid
	}
	groups, e := r.Groups(ctx, []string{asset}, mediaext.SubtitleExtensions())
	if e != nil {
		return out, e
	}
	if groups[asset] != nil {
		out.Items = groups[asset]
	}
	return out, nil
}
func audit(ctx context.Context, tx pgx.Tx, action, asset, id string) error {
	raw, _ := json.Marshal(map[string]string{"assetId": asset, "subtitleId": id})
	_, e := tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator',$1,$2)`, "subtitle."+action, raw)
	return e
}
