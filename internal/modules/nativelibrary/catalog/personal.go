package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/modules/nativelibrary"
	"time"
)

var _ nativelibrary.PersonalService = BrowseRepository{}

func (r BrowseRepository) PersonalData(parent context.Context, u contracts.User, p contracts.PermissionChecker, id string, patch *nativelibrary.PersonalPatch) (out dto.UserItemDataDto, err error) {
	if len(id) == 0 || len(id) > 19 {
		return out, nativelibrary.ErrNotFound
	}
	if patch != nil && (patch.PositionTicks != nil && *patch.PositionTicks < 0 || patch.PlayCount != nil && (*patch.PlayCount < 0 || *patch.PlayCount > 1000000000) || patch.LastPlayedAt != nil && (patch.LastPlayedAt.Year() < 1 || patch.LastPlayedAt.Year() > 9999)) {
		return out, nativelibrary.ErrInvalidQuery
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	// Query performs identical SQL visibility/parental filtering to all browsing.
	items, e := r.personalItems(ctx, u, p, id)
	if e != nil {
		return out, e
	}
	if len(items) != 1 || items[0].UserData == nil {
		return out, nativelibrary.ErrNotFound
	}
	out = *items[0].UserData
	if patch == nil {
		return out, nil
	}
	v := *patch
	if v.PositionTicks == nil && v.PlayCount == nil && v.Played == nil && v.Favorite == nil && v.LastPlayedAt == nil {
		return out, nativelibrary.ErrInvalidQuery
	}
	ids := []string{id}
	folder := items[0].Type == dto.Series || items[0].Type == dto.Season
	if folder && v.Played != nil {
		visible, e := scope(ctx, u, p)
		if e != nil {
			return out, e
		}
		// T15 episodes inherit their series rating. The authoritative parent
		// was checked above. Reuse the same keys/visibility definition without
		// hydrating names, episode JSON, streams or personal aggregates here.
		rows, e := r.DB.Query(ctx, browseKeysCTE+` SELECT n.id::text FROM keyed k
          JOIN native_item_ids parent ON parent.id=$5::bigint
          LEFT JOIN native_item_ids n ON n.entity_type='episode' AND n.internal_key=k.internal_key
          WHERE k.kind='episode' AND k.internal_key LIKE parent.internal_key||CASE WHEN parent.entity_type='series' THEN ':s%' ELSE 'e%' END
          AND ($3::boolean OR btrim(regexp_replace(upper(btrim($6::text)),'^(CN:|US:)\s*',''))=ANY($2::text[]))
          ORDER BY n.id NULLS LAST LIMIT 5001`, visible.ids, visible.codes, visible.unrated, visible.library, id, items[0].OfficialRating)
		if e != nil {
			return out, contracts.ErrUnavailable
		}
		ids = nil
		missing := false
		for rows.Next() {
			var key *string
			if e = rows.Scan(&key); e != nil {
				break
			}
			if key == nil {
				missing = true
				ids = append(ids, "")
			} else {
				ids = append(ids, *key)
			}
		}
		e = errors.Join(e, rows.Err())
		rows.Close()
		if e != nil {
			return out, contracts.ErrUnavailable
		}
		if len(ids) > 5000 {
			return out, nativelibrary.ErrPersonalBatchTooLarge
		}
		if missing {
			return out, contracts.ErrUnavailable
		}
	} else if items[0].IsFolder && v.Played != nil {
		return out, nativelibrary.ErrInvalidQuery
	}
	tx, e := r.DB.Begin(ctx)
	if e != nil {
		return out, contracts.ErrUnavailable
	}
	defer func() {
		if e := tx.Rollback(ctx); err == nil && e != nil && !errors.Is(e, pgx.ErrTxClosed) {
			err = contracts.ErrUnavailable
		}
	}()
	// Same short user lock as T34. No network, file I/O or full metadata loading.
	var allowed bool
	if e = tx.QueryRow(ctx, `SELECT enabled AND NOT must_change_password AND credential_version=$2 AND (expires_at IS NULL OR expires_at>clock_timestamp()) FROM media_users WHERE id=$1::uuid FOR UPDATE`, u.ID, u.CredentialVersion).Scan(&allowed); e != nil {
		return out, contracts.ErrUnavailable
	}
	if !allowed {
		return out, nativelibrary.ErrNotFound
	}
	if folder {
		if v.Played != nil {
			child := v
			child.Favorite = nil
			child.PositionTicks = nil
			child.PlayCount = nil
			if e = writePersonal(ctx, tx, u.ID, ids, child); e != nil {
				return out, e
			}
		}
		if v.Favorite != nil {
			if e = writePersonal(ctx, tx, u.ID, []string{id}, nativelibrary.PersonalPatch{Favorite: v.Favorite}); e != nil {
				return out, e
			}
		}
	} else {
		if e = writePersonal(ctx, tx, u.ID, ids, v); e != nil {
			return out, e
		}
	}
	// Read only the changed user rows under the same lock. Rehydrating the
	// entire catalog after a 5000-episode write wastes the request budget.
	if folder {
		if v.Favorite != nil {
			out.IsFavorite = *v.Favorite
		}
		if v.Played != nil {
			var played, total, count int
			var stamp *time.Time
			e = tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE d.played),coalesce(sum(d.play_count),0),max(d.last_played_at),coalesce((SELECT favorite FROM media_user_item_data WHERE user_id=$1::uuid AND item_id=$3::bigint),false)
              FROM unnest($2::text[]) key JOIN media_user_item_data d ON d.item_id=key::bigint AND d.user_id=$1::uuid`, u.ID, ids, id).Scan(&total, &played, &count, &stamp, &out.IsFavorite)
			if e != nil {
				return out, contracts.ErrUnavailable
			}
			out.Played = total > 0 && played == total
			unplayed := total - played
			out.UnplayedItemCount = &unplayed
			out.PlayCount = &count
			percentage := float64(0)
			if total > 0 {
				percentage = 100 * float64(played) / float64(total)
			}
			out.PlayedPercentage = &percentage
			out.LastPlayedDate = nil
			if stamp != nil {
				date := dto.NewUTCDateTime(*stamp)
				out.LastPlayedDate = &date
			}
		}
	} else {
		var position int64
		var count int
		var stamp *time.Time
		if e = tx.QueryRow(ctx, `SELECT position_ticks,play_count,played,favorite,last_played_at FROM media_user_item_data WHERE user_id=$1::uuid AND item_id=$2::bigint`, u.ID, id).Scan(&position, &count, &out.Played, &out.IsFavorite, &stamp); e != nil {
			return out, contracts.ErrUnavailable
		}
		out.PlaybackPositionTicks = &position
		out.PlayCount = &count
		out.PlayedPercentage = nil
		out.LastPlayedDate = nil
		if stamp != nil {
			date := dto.NewUTCDateTime(*stamp)
			out.LastPlayedDate = &date
		}
		out = nativelibrary.CompleteUserData(out, id, items[0].RunTimeTicks)
	}
	if e = tx.Commit(ctx); e != nil {
		return out, contracts.ErrUnavailable
	}
	return out, nil
}
func writePersonal(ctx context.Context, tx pgx.Tx, user string, ids []string, p nativelibrary.PersonalPatch) error {
	if len(ids) == 0 {
		return nil
	}
	tag, e := tx.Exec(ctx, `INSERT INTO media_user_item_data AS old(user_id,item_id,position_ticks,play_count,played,favorite,last_played_at)
 SELECT $1::uuid,id::bigint,CASE WHEN $5::boolean THEN 0 ELSE coalesce($3::bigint,0) END,
 coalesce($4::int,CASE WHEN $5::boolean THEN 1 ELSE 0 END),coalesce($5::boolean,false),coalesce($6::boolean,false),
 CASE WHEN $7::timestamptz IS NOT NULL THEN $7 WHEN $5::boolean OR $3::bigint>0 THEN clock_timestamp() END FROM unnest($2::text[]) id
 ON CONFLICT(user_id,item_id) DO UPDATE SET
 position_ticks=CASE WHEN $5::boolean THEN 0 WHEN $3::bigint IS NOT NULL THEN $3 WHEN $5::boolean=false THEN 0 ELSE old.position_ticks END,
 play_count=coalesce($4::int,CASE WHEN $5::boolean AND NOT old.played THEN old.play_count+1 WHEN $5::boolean=false THEN 0 ELSE old.play_count END),
 played=coalesce($5::boolean,old.played),favorite=coalesce($6::boolean,old.favorite),
 last_played_at=CASE WHEN $7::timestamptz IS NOT NULL THEN $7 WHEN $5::boolean=false AND coalesce($3::bigint,0)=0 THEN NULL
 WHEN $5::boolean AND NOT old.played OR $3::bigint IS NOT NULL AND $3 IS DISTINCT FROM old.position_ticks THEN clock_timestamp() ELSE old.last_played_at END,updated_at=clock_timestamp()`, user, ids, p.PositionTicks, p.PlayCount, p.Played, p.Favorite, p.LastPlayedAt)
	if e != nil || tag.RowsAffected() != int64(len(ids)) {
		return contracts.ErrUnavailable
	}
	return nil
}

// Folder UserData does not need posters, streams, episode JSON or the general
// browse assembly. This light batch aggregate also keeps the 5000-row write
// budget independent of how large the series metadata is.
func (r BrowseRepository) personalItems(ctx context.Context, u contracts.User, p contracts.PermissionChecker, id string) ([]dto.BaseItemDto, error) {
	if r.DB == nil {
		return nil, contracts.ErrUnavailable
	}
	var kind string
	e := r.DB.QueryRow(ctx, `SELECT entity_type FROM native_item_ids WHERE id::text=$1`, id).Scan(&kind)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, nativelibrary.ErrNotFound
	}
	if e != nil {
		return nil, contracts.ErrUnavailable
	}
	if kind != "series" && kind != "season" {
		items, _, e := r.Query(ctx, u, p, nativelibrary.ItemQuery{IDs: []string{id}, Limit: 1})
		return items, e
	}
	s, e := scope(ctx, u, p)
	if e != nil {
		return nil, e
	}
	var raw []byte
	var rating string
	var libraries []string
	sql := browseKeysCTE + nextUpLightCTE + ` SELECT k.rating,k.libraries,jsonb_strip_nulls(jsonb_build_object('Played',coalesce(stats.total>0 AND stats.unplayed=0,false),'PlayCount',coalesce(stats.count,0),'IsFavorite',k.user_favorite,'LastPlayedDate',greatest(k.user_last_played,stats.last_played),'UnplayedItemCount',coalesce(stats.unplayed,0),'PlayedPercentage',coalesce(100.0*(stats.total-stats.unplayed)/nullif(stats.total,0),0)))
 FROM item_rows k LEFT JOIN LATERAL(SELECT count(*) total,count(*) FILTER(WHERE NOT e.user_played) unplayed,sum(e.user_count) count,max(e.user_last_played) last_played FROM item_rows e
 WHERE e.kind='episode' AND e.internal_key LIKE k.internal_key||CASE WHEN k.kind='series' THEN ':s%' ELSE 'e%' END
 AND ($3::boolean OR btrim(regexp_replace(upper(btrim(e.rating)),'^(CN:|US:)\s*',''))=ANY($2::text[])))stats ON true
 WHERE k.id=$6::text AND ($3::boolean OR btrim(regexp_replace(upper(btrim(k.rating)),'^(CN:|US:)\s*',''))=ANY($2::text[])) LIMIT 1`
	e = r.DB.QueryRow(ctx, sql, s.ids, s.codes, s.unrated, s.library, s.user, id).Scan(&rating, &libraries, &raw)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, nativelibrary.ErrNotFound
	}
	if e != nil {
		return nil, contracts.ErrUnavailable
	}
	if e = p.ItemAllowed(ctx, u, contracts.ItemAccess{ID: id, LibraryIDs: libraries, OfficialRating: rating}); e != nil {
		return nil, e
	}
	var data dto.UserItemDataDto
	if json.Unmarshal(raw, &data) != nil {
		return nil, contracts.ErrUnavailable
	}
	data = nativelibrary.CompleteUserData(data, id, nil)
	typ := dto.Series
	if kind == "season" {
		typ = dto.Season
	}
	item, e := dto.NewBaseItemDto(id, "", typ)
	if e != nil {
		return nil, e
	}
	item.UserData = &data
	item.OfficialRating = rating
	return []dto.BaseItemDto{item}, nil
}
