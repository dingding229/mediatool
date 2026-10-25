package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/modules/metadata"
	"media-tool/internal/modules/nativelibrary"
)

type detailKey struct {
	Kind string `json:"kind"`
	TMDB int64  `json:"tmdb"`
	S    int    `json:"s"`
}

func (r BrowseRepository) hydrate(ctx context.Context, records []browseRow, fields []string, includeProbes bool) ([]dto.BaseItemDto, error) {
	out := []dto.BaseItemDto{}
	if len(records) == 0 {
		return out, nil
	}
	keys := []nativelibrary.Key{}
	wanted := []detailKey{}
	for _, x := range records {
		if x.Kind == "library" {
			keys = append(keys, nativelibrary.Key{EntityType: nativelibrary.EntityLibrary, InternalKey: x.Key})
			continue
		}
		root := "series"
		if x.Kind == "movie" {
			root = "movie"
		}
		keys = append(keys, nativelibrary.Key{EntityType: nativelibrary.EntityType(x.Kind), InternalKey: x.Key})
		wanted = append(wanted, detailKey{root, x.TMDB, -1})
		if x.Kind == "season" || x.Kind == "episode" {
			keys = append(keys, nativelibrary.Key{EntityType: nativelibrary.EntitySeries, InternalKey: fmt.Sprint("tmdb:", x.TMDB)})
			wanted = append(wanted, detailKey{"season", x.TMDB, x.S})
		}
		if x.Kind == "episode" {
			keys = append(keys, nativelibrary.Key{EntityType: nativelibrary.EntitySeason, InternalKey: fmt.Sprintf("tmdb:%d:s%d", x.TMDB, x.S)})
		}
	}
	ids, err := r.pageIDs(ctx, keys, len(records))
	if err != nil {
		return nil, err
	}
	keyID := func(kind, key string) string {
		return strconv.FormatInt(ids[nativelibrary.Key{EntityType: nativelibrary.EntityType(kind), InternalKey: key}], 10)
	}
	var imageIDs []string
	for _, x := range records {
		if x.Kind == "episode" {
			imageIDs = append(imageIDs, keyID(x.Kind, x.Key))
		}
	}
	images, err := episodeImagePage(ctx, r.DB, imageIDs)
	if err != nil {
		return nil, contracts.ErrUnavailable
	}
	raw, err := json.Marshal(wanted)
	if err != nil {
		return nil, err
	}
	recordJSON, err := json.Marshal(records)
	if err != nil {
		return nil, err
	}
	details := map[detailKey]nativelibrary.TMDBDetails{}
	// Project bounded fields, and only this page's season episodes. Never transfer
	// a 4 MiB season payload per physical asset or one unpaged full-season array.
	rows, err := r.DB.Query(ctx, `WITH wanted AS(SELECT DISTINCT * FROM jsonb_to_recordset($1) w(kind text,tmdb bigint,s int))
 SELECT w.kind,w.tmdb,w.s,jsonb_build_object('name',d.payload->'name','originalTitle',d.payload->'originalTitle','sortName',d.payload->'sortName',
 'overview',CASE WHEN $3 THEN left(d.payload->>'overview',16384) ELSE '' END,'genres',d.payload->'genres','posterPath',d.payload->'posterPath','backdropPath',d.payload->'backdropPath',
 'premiereDate',d.payload->'premiereDate','communityRating',d.payload->'communityRating','officialRating',d.payload->'officialRating','imdbId',d.payload->'imdbId',
 'backdrops',d.payload->'backdrops','logos',d.payload->'logos','runtimeMinutes',d.payload->'runtimeMinutes','logoPath',d.payload->'logoPath','status',d.payload->'status','endDate',d.payload->'endDate',
 'people',CASE WHEN $4 THEN coalesce((SELECT jsonb_agg(v) FROM(SELECT v FROM jsonb_array_elements(coalesce(d.payload->'people','[]')) v LIMIT 60) a),'[]') ELSE '[]'::jsonb END,
 'episodes',coalesce((SELECT jsonb_agg(ep) FROM jsonb_array_elements(coalesce(d.payload->'episodes','[]')) ep WHERE EXISTS(
 SELECT 1 FROM jsonb_to_recordset($2) p(tmdb bigint,"S" int,"E" int) WHERE p.tmdb=w.tmdb AND p."S"=w.s AND p."E"::text=ep->>'number')),'[]'))
 FROM wanted w JOIN native_tmdb_details d ON d.tmdb_type=w.kind AND d.tmdb_id=w.tmdb AND coalesce(d.season_number,-1)=w.s AND d.fetched_at IS NOT NULL
 AND d.language=coalesce((SELECT nullif(settings->>'language','') FROM integrations WHERE kind='tmdb' AND enabled ORDER BY created_at LIMIT 1),'zh-CN')`, raw, recordJSON, slices.Contains(fields, "Overview"), slices.Contains(fields, "People"))
	if err != nil {
		return nil, contracts.ErrUnavailable
	}
	for rows.Next() {
		var k detailKey
		var payload []byte
		if err := rows.Scan(&k.Kind, &k.TMDB, &k.S, &payload); err != nil {
			rows.Close()
			return nil, contracts.ErrUnavailable
		}
		var d nativelibrary.TMDBDetails
		if err := json.Unmarshal(payload, &d); err != nil {
			rows.Close()
			return nil, contracts.ErrUnavailable
		}
		details[k] = d
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, contracts.ErrUnavailable
	}
	sources := map[string][]nativelibrary.MovieSource{}
	ends := map[string]int{}
	overflows := map[string]bool{}
	rows, err = r.DB.Query(ctx, `WITH wanted AS MATERIALIZED(SELECT * FROM jsonb_to_recordset($1) w("Kind" text,"Key" text,tmdb bigint,"S" int,"E" int,"Libraries" text[])),wanted_roots AS MATERIALIZED(
 SELECT DISTINCT m.id FROM wanted w CROSS JOIN LATERAL(SELECT id FROM media_items WHERE media_type=CASE w."Kind" WHEN 'movie' THEN 'movie' ELSE 'series' END AND tmdb_id=w.tmdb LIMIT 1)m WHERE w."Kind" IN('movie','episode')
 ),source_bindings AS MATERIALIZED(SELECT *FROM media_asset_bindings WHERE media_id IN(SELECT id FROM wanted_roots)),eligible AS MATERIALIZED (
 SELECT e.asset_id,e.cloud_file_id,e.tmdb_id,e.media_type,e.created_at,e.season,
 coalesce((SELECT min(value::int) FROM jsonb_array_elements_text(e.episodes)),e.episode) first_episode,
 coalesce((SELECT max(value::int) FROM jsonb_array_elements_text(e.episodes)),e.episode,0) last_episode
 FROM (`+strings.Replace(ExposableAssetsSQL, "JOIN media_asset_bindings b ON", "JOIN source_bindings b ON", 1)+`)e
 ),ranked AS(
 SELECT w."Key" key,e.asset_id,coalesce(lower(substring(f.full_path FROM '\.([A-Za-z0-9]{1,16})$')),'') container,
 e.last_episode,
 row_number() OVER(PARTITION BY w."Key" ORDER BY e.created_at DESC,e.asset_id) rank,count(*) OVER(PARTITION BY w."Key") versions
 FROM wanted w JOIN eligible e ON e.tmdb_id=w.tmdb AND e.media_type=CASE w."Kind" WHEN 'movie' THEN 'movie' ELSE 'series' END
 JOIN native_asset_libraries al ON al.asset_id=e.asset_id AND al.library_id=ANY(w."Libraries") JOIN cloud_files f ON f.id=e.cloud_file_id
 WHERE w."Kind"='movie' OR w."Kind"='episode' AND e.season=w."S" AND e.first_episode=w."E")
 SELECT key,jsonb_agg(jsonb_build_object('ID',asset_id::text,'Container',container) ORDER BY rank),max(last_episode),max(versions)>100 FROM ranked WHERE rank<=100 GROUP BY key`, recordJSON)
	if err != nil {
		return nil, contracts.ErrUnavailable
	}
	for rows.Next() {
		var key string
		var payload []byte
		var end int
		var overflow bool
		if err := rows.Scan(&key, &payload, &end, &overflow); err != nil {
			rows.Close()
			return nil, contracts.ErrUnavailable
		}
		var list []nativelibrary.MovieSource
		if err := json.Unmarshal(payload, &list); err != nil {
			rows.Close()
			return nil, contracts.ErrUnavailable
		}
		sources[key] = list
		ends[key], overflows[key] = end, overflow
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, contracts.ErrUnavailable
	}
	var pageSources []*nativelibrary.MovieSource
	for _, list := range sources {
		for i := range list {
			pageSources = append(pageSources, &list[i])
		}
	}
	if includeProbes {
		if err := hydrateProbeSources(ctx, r.DB, pageSources); err != nil {
			return nil, contracts.ErrUnavailable
		}
	}
	var server string
	if err := r.DB.QueryRow(ctx, `SELECT trim(server_id::text) FROM native_server_settings`).Scan(&server); err != nil {
		return nil, contracts.ErrUnavailable
	}
	for _, x := range records {
		root := "series"
		if x.Kind == "movie" {
			root = "movie"
		}
		d := details[detailKey{root, x.TMDB, -1}]
		if d.PosterPath == "" {
			d.PosterPath = x.Meta.PosterPath
		}
		if d.BackdropPath == "" {
			d.BackdropPath = x.Meta.BackdropPath
		}
		sd := details[detailKey{"season", x.TMDB, x.S}]
		id := keyID(x.Kind, x.Key)
		seriesID := keyID("series", fmt.Sprint("tmdb:", x.TMDB))
		seasonID := keyID("season", fmt.Sprintf("tmdb:%d:s%d", x.TMDB, x.S))
		var item dto.BaseItemDto
		switch x.Kind {
		case "library":
			item, err = nativelibrary.LibraryDTO(nativelibrary.Library{ID: x.Key, ItemID: id, Name: x.Name, ServerID: server, CollectionType: nativelibrary.CollectionType(x.CollectionType)})
		case "movie":
			item, err = nativelibrary.MovieDTO(nativelibrary.Movie{ID: id, ServerID: server, ParentID: x.Parent, Name: x.Name, Year: x.Year, TMDBID: x.TMDB, Metadata: x.Meta, Details: &d, CreatedAt: x.Created, Sources: sources[x.Key]})
		case "series":
			item, err = nativelibrary.SeriesDTO(nativelibrary.SeriesProjection{ID: id, ServerID: server, ParentID: x.Parent, Name: x.Name, Year: x.Year, TMDBID: x.TMDB, Metadata: x.Meta, Details: &d, SeasonCount: x.Seasons, EpisodeCount: x.Episodes, CreatedAt: x.Created})
		case "season":
			item, err = nativelibrary.SeasonDTO(nativelibrary.SeasonProjection{ID: id, SeriesID: seriesID, ParentID: seriesID, ServerID: server, SeriesName: x.Title, Name: x.Name, Year: x.Year, TMDBID: x.TMDB, Number: x.S, Details: &sd, SeriesDetails: &d, EpisodeCount: x.Episodes, CreatedAt: x.Created})
		case "episode":
			var ep metadata.Episode
			if err = json.Unmarshal(x.EpisodeInfo, &ep); err != nil {
				return nil, contracts.ErrUnavailable
			}
			for _, v := range sd.Episodes {
				if v.Number == x.E {
					if ep.Runtime == 0 {
						ep.Runtime = v.RuntimeMinutes
					}
					if ep.StillPath == "" {
						ep.StillPath = v.StillPath
					}
					if ep.AirDate == "" {
						ep.AirDate = v.AirDate
					}
				}
			}
			var ticks *int64
			if ep.Runtime > 0 {
				v := int64(ep.Runtime) * 600000000
				ticks = &v
			}
			for i := range sources[x.Key] {
				sources[x.Key][i].RunTimeTicks = ticks
			}
			item, err = nativelibrary.EpisodeDTO(nativelibrary.EpisodeProjection{ID: id, SeriesID: seriesID, SeasonID: seasonID, ParentID: seasonID, ServerID: server, SeriesName: x.Title, SeasonName: seasonName(x.S, sd), Name: x.Name, Overview: ep.Overview, Year: x.Year, TMDBID: x.TMDB, Season: x.S, Number: x.E, NumberEnd: ends[x.Key], AirDate: ep.AirDate, RuntimeTicks: ticks, StillPath: firstNonempty(images[id].Still, ep.StillPath), LocalStillTag: images[id].Tag, FallbackPoster: firstNonempty(images[id].Poster, d.PosterPath), SeriesDetails: &d, Sources: sources[x.Key], VersionOverflow: overflows[x.Key], CreatedAt: x.Created})
		}
		if err != nil {
			return nil, err
		}
		if x.Kind == "movie" && overflows[x.Key] && len(item.MediaSources) > 0 {
			item.MediaSources[0].Name += "（仅展示最新100个版本）"
		}
		if x.Kind == "season" || x.Kind == "episode" {
			item.Genres = append(dto.Array[string]{}, x.Meta.Genres...)
		}
		if x.Kind != "library" {
			item.Name = x.Name
			nativelibrary.ApplyEffectiveText(&item, x.Meta)
			item.ProductionYear = nil
			if y, err := strconv.Atoi(x.Year); err == nil && y >= 1 && y <= 9999 {
				item.ProductionYear = &y
			}
		}
		item.OfficialRating = x.Rating
		if !slices.Contains(fields, "Overview") {
			item.Overview = ""
		}
		if !slices.Contains(fields, "People") {
			item.People = nil
		}
		out = append(out, item)
	}
	return out, nil
}

func firstNonempty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
