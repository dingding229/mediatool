package catalog

import (
	"context"
	"fmt"
	"media-tool/internal/modules/classification"
	"net/url"
	"strconv"
	"strings"
)

// Parent identities represent persisted works and season numbers, not a page's
// first physical file. Bind parent filters before grouping/paging.
const managedMembers = ` FROM media_assets a JOIN cloud_files f ON f.id=a.cloud_file_id LEFT JOIN media_asset_bindings b ON b.asset_id=a.id LEFT JOIN media_items m ON m.id=b.media_id `

func (e Engine) managed(ctx context.Context, out Page, q, kind, category string) (Page, error) {
	policy, err := (classification.Repository{DB: e.DB}).Get(ctx)
	if err != nil {
		return out, ErrUnavailable
	}
	out.Categories = categoryOptions(policy.Value)
	if category != "" && category != "unclassified" {
		found := false
		for _, v := range out.Categories {
			found = found || v.ID == category
		}
		if !found {
			return out, ErrInvalid
		}
	}
	parent := out.ParentID
	displayKind := "CASE WHEN m.media_type='series' THEN 'episode' ELSE 'movie' END"
	if parent == "" {
		displayKind = "m.media_type"
	} else if strings.HasPrefix(parent, "series_") {
		displayKind = "'season'"
	}
	where := ` WHERE ($1='' OR coalesce(v.metadata->>'title',f.full_path) ILIKE '%'||$1||'%') AND ($2='' OR m.media_type=$2)`
	args := []any{q, kind}
	fields := `a.id::text id,coalesce(v.metadata->>'title',regexp_replace(f.full_path,'^.*/','','g')) title,coalesce(v.metadata->>'year','') item_year,coalesce(m.media_type,'unknown') media_type,coalesce(v.metadata->>'overview','') overview,coalesce(m.metadata->>'posterPath','') poster,f.full_path file_path,f.state,b.season,b.episode,CASE WHEN m.media_type='series' THEN 'Episode' ELSE 'Movie' END type`
	order := `season NULLS LAST,episode NULLS LAST,id`
	if parent != "" {
		parts := strings.Split(parent, "_")
		if len(parts) < 2 || (parts[0] != "series" && parts[0] != "season") || !uuidID.MatchString(parts[1]) {
			return out, ErrInvalid
		}
		args = append(args, parts[1])
		where += ` AND b.media_id=$3::uuid AND m.media_type='series'`
		if parts[0] == "season" {
			if len(parts) != 3 {
				return out, ErrInvalid
			}
			n, err := strconv.Atoi(parts[2])
			if err != nil || n < 0 || n > 10000 {
				return out, ErrInvalid
			}
			args = append(args, n)
			where += ` AND b.season=$4`
		} else {
			if len(parts) != 2 {
				return out, ErrInvalid
			}
			fields = `'season_'||m.id::text||'_'||b.season id,max(v.metadata->>'title') title,max(coalesce(v.metadata->>'year','')) item_year,'series' media_type,max(coalesce(v.metadata->>'overview','')) overview,max(coalesce(m.metadata->>'posterPath','')) poster,'' file_path,'' state,b.season,NULL::int episode,'Season' type`
			where += ` AND b.season IS NOT NULL GROUP BY m.id,b.season`
		}
	} else {
		fields = `CASE WHEN m.media_type='series' THEN 'series_'||m.id::text ELSE a.id::text END id,max(coalesce(v.metadata->>'title',regexp_replace(f.full_path,'^.*/','','g'))) title,max(coalesce(v.metadata->>'year','')) item_year,max(coalesce(m.media_type,'unknown')) media_type,max(coalesce(v.metadata->>'overview','')) overview,max(coalesce(m.metadata->>'posterPath','')) poster,CASE WHEN m.media_type='series' THEN '' ELSE max(f.full_path) END file_path,CASE WHEN m.media_type='series' THEN '' ELSE max(f.state) END state,NULL::int season,NULL::int episode,CASE WHEN m.media_type='series' THEN 'Series' ELSE 'Movie' END type`
		where += ` GROUP BY CASE WHEN m.media_type='series' THEN 'series_'||m.id::text ELSE a.id::text END,m.media_type,m.id`
		order = `title,id`
	}
	categoryExpr := categorySQLFrom(policy.Value, &args, "c.metadata")
	if category != "" {
		args = append(args, category)
		condition := fmt.Sprintf(" AND (%s)=$%d", categoryExpr, len(args))
		where = strings.Replace(where, " GROUP BY", condition+" GROUP BY", 1)
		if !strings.Contains(where, " GROUP BY") {
			where += condition
		}
	}
	if strings.Contains(where, " GROUP BY") {
		fields += ",max(" + categoryExpr + ") category_id"
	} else {
		fields += "," + categoryExpr + " category_id"
	}
	sql := `WITH entries AS(SELECT ` + fields + managedEffectiveMembers(displayKind) + where + `) `
	if err := e.DB.QueryRow(ctx, sql+`SELECT count(*) FROM entries`, args...).Scan(&out.Total); err != nil {
		return out, ErrUnavailable
	}
	args = append(args, out.PageSize, (out.Page-1)*out.PageSize)
	rows, err := e.DB.Query(ctx, sql+`SELECT id,title,item_year,media_type,overview,poster,file_path,state,season,episode,type,category_id FROM entries ORDER BY `+order+fmt.Sprintf(` LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if err != nil {
		return out, ErrUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		item := Item{Source: "managed"}
		var poster string
		if err = rows.Scan(&item.ID, &item.Title, &item.Year, &item.MediaType, &item.Overview, &poster, &item.FilePath, &item.State, &item.Season, &item.Episode, &item.Type, &item.CategoryID); err != nil {
			return out, ErrUnavailable
		}
		item.Category = "未分类"
		for _, c := range out.Categories {
			if c.ID == item.CategoryID {
				item.Category = c.Name
				break
			}
		}
		if safePoster.MatchString(poster) {
			item.Poster = "/api/v1/media/tmdb/image?path=" + url.QueryEscape(poster)
		}
		out.Items = append(out.Items, item)
	}
	if rows.Err() != nil {
		return out, ErrUnavailable
	}
	return out, nil
}

// ListClassified is the administrator-only category-filtered view. Protocol
// repositories and external Emby browsing do not depend on this projection.
func (e Engine) ListClassified(ctx context.Context, q, kind, parent, category string, page, size int) (Page, error) {
	if len(q) > 200 || page < 1 || page > 10000 || size < 1 || size > 100 || len(category) > 210 || parent != "" && !safeID.MatchString(parent) || kind != "" && kind != "movie" && kind != "series" {
		return Page{}, ErrInvalid
	}
	return e.managed(ctx, Page{Items: []Item{}, Source: "managed", Page: page, PageSize: size, ParentID: parent}, q, kind, category)
}
