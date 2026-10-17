package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/metadata"
	"media-tool/internal/modules/nativelibrary"
)

type HierarchyRepository struct{ DB *pgxpool.Pool }

var _ nativelibrary.HierarchyService = HierarchyRepository{}

var hierarchySorts = map[string]string{"SortName": "lower(coalesce(nullif(p.metadata->>'sortName',''),p.title)) COLLATE \"C\"", "DateCreated": "p.date_created", "ProductionYear": "CASE WHEN p.release_year~'^[0-9]{4}$' THEN p.release_year::int ELSE NULL END", "CommunityRating": "CASE WHEN jsonb_typeof(coalesce(d.payload->'communityRating',p.metadata->'communityRating'))='number' THEN (coalesce(d.payload->>'communityRating',p.metadata->>'communityRating'))::numeric ELSE NULL END", "DatePlayed": "p.date_created"}

func validHierarchyQuery(q nativelibrary.HierarchyQuery, sorts map[string]string) error {
	if !q.Visible.Valid() || q.StartIndex < 0 || q.StartIndex > 1000000 || q.Limit < 1 || q.Limit > 200 {
		return ErrInvalid
	}
	if q.SortBy == "" {
		q.SortBy = "SortName"
	}
	if _, ok := sorts[q.SortBy]; !ok {
		return ErrInvalid
	}
	return nil
}
func (r HierarchyRepository) ListSeries(ctx context.Context, q nativelibrary.HierarchyQuery) ([]dto.BaseItemDto, int, error) {
	if q.SortBy == "" {
		q.SortBy = "SortName"
	}
	if err := validHierarchyQuery(q, hierarchySorts); err != nil {
		return nil, 0, err
	}
	if r.DB == nil {
		return nil, 0, nativelibrary.ErrSettingsUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	order := hierarchySorts[q.SortBy]
	direction := " ASC"
	if q.Descending {
		direction = " DESC"
	}
	cte := `WITH eligible AS (` + ExposableAssetsSQL + `), members AS(SELECT e.* FROM eligible e JOIN media_asset_bindings b ON b.asset_id=e.asset_id JOIN native_asset_libraries al ON al.asset_id=e.asset_id JOIN native_libraries l ON l.id=al.library_id WHERE e.media_type='series' AND l.id=$1 AND NOT l.hidden), series_rows AS(SELECT media_id,tmdb_id,title,release_year,metadata,min(created_at) date_created,count(DISTINCT season) season_count,count(DISTINCT (season::text||':'||coalesce((SELECT min(value::int) FROM jsonb_array_elements_text(episodes) WHERE value~'^[0-9]+$'),episode)::text)) episode_count FROM members GROUP BY media_id,tmdb_id,title,release_year,metadata)`
	var total int
	if err := r.DB.QueryRow(ctx, cte+`SELECT count(*) FROM series_rows`, q.Visible.RequestedID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.DB.Query(ctx, cte+`SELECT p.tmdb_id,p.title,p.release_year,p.metadata,p.date_created,l.item_id::text,trim(n.server_id::text),p.season_count,p.episode_count,coalesce(d.payload,'{}'::jsonb) FROM series_rows p JOIN native_libraries l ON l.id=$1 CROSS JOIN native_server_settings n LEFT JOIN native_tmdb_details d ON d.tmdb_type='series' AND d.tmdb_id=p.tmdb_id AND d.fetched_at IS NOT NULL AND d.language=coalesce((SELECT nullif(settings->>'language','') FROM integrations WHERE kind='tmdb' AND enabled ORDER BY created_at LIMIT 1),'zh-CN') ORDER BY `+order+direction+`,p.tmdb_id ASC LIMIT $2 OFFSET $3`, q.Visible.RequestedID, q.Limit, q.StartIndex)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	type row struct {
		tmdbID            int64
		title, year       string
		metadata, details []byte
		created           time.Time
		parent, server    string
		seasons, episodes int
	}
	records := []row{}
	keys := []nativelibrary.Key{}
	for rows.Next() {
		var x row
		if err := rows.Scan(&x.tmdbID, &x.title, &x.year, &x.metadata, &x.created, &x.parent, &x.server, &x.seasons, &x.episodes, &x.details); err != nil {
			return nil, 0, err
		}
		k, e := nativelibrary.SeriesKey(x.tmdbID)
		if e != nil {
			return nil, 0, e
		}
		keys = append(keys, k)
		records = append(records, x)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	ids, err := nativelibrary.EnsureIDs(ctx, IDQueries{DB: r.DB}, keys)
	if err != nil {
		return nil, 0, err
	}
	out := make([]dto.BaseItemDto, 0, len(records))
	for i, x := range records {
		var meta nativelibrary.StoredMovieMetadata
		var details nativelibrary.TMDBDetails
		if err = json.Unmarshal(x.metadata, &meta); err != nil {
			return nil, 0, err
		}
		if err = json.Unmarshal(x.details, &details); err != nil {
			return nil, 0, err
		}
		item, e := nativelibrary.SeriesDTO(nativelibrary.SeriesProjection{ID: strconv.FormatInt(ids[keys[i]], 10), ServerID: x.server, ParentID: x.parent, Name: x.title, Year: x.year, TMDBID: x.tmdbID, Metadata: meta, Details: &details, SeasonCount: x.seasons, EpisodeCount: x.episodes, CreatedAt: x.created})
		if e != nil {
			return nil, 0, e
		}
		out = append(out, item)
	}
	return out, total, nil
}
func (r HierarchyRepository) GetSeries(ctx context.Context, visible nativelibrary.VisibleLibraries, seriesID string) (dto.BaseItemDto, error) {
	if !visible.Valid() {
		return dto.BaseItemDto{}, ErrInvalid
	}
	items, _, err := r.ListSeries(ctx, nativelibrary.HierarchyQuery{Visible: visible, Limit: 200})
	if err != nil {
		return dto.BaseItemDto{}, err
	}
	for _, item := range items {
		if item.Id == seriesID {
			return item, nil
		}
	}
	return dto.BaseItemDto{}, nativelibrary.ErrNotFound
}
func (r HierarchyRepository) ListSeasons(ctx context.Context, visible nativelibrary.VisibleLibraries, seriesID string) ([]dto.BaseItemDto, error) {
	if r.DB == nil {
		return nil, nativelibrary.ErrSettingsUnavailable
	}
	if !visible.Valid() || !numeric(seriesID) {
		return nil, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var tmdbID int64
	if err := r.DB.QueryRow(ctx, `SELECT substring(n.internal_key from 6)::bigint FROM native_item_ids n WHERE n.id=$1::bigint AND n.entity_type='series'`, seriesID).Scan(&tmdbID); err != nil {
		return nil, err
	}
	rows, err := r.DB.Query(ctx, `WITH eligible AS (`+ExposableAssetsSQL+`) SELECT b.season,max(m.title),max(m.release_year),min(e.created_at),l.item_id::text,count(DISTINCT coalesce((SELECT min(value::int) FROM jsonb_array_elements_text(b.episodes) WHERE value~'^[0-9]+$'),b.episode)),coalesce(d.payload,'{}'::jsonb),coalesce(sdseries.payload,'{}'::jsonb) FROM eligible e JOIN media_asset_bindings b ON b.asset_id=e.asset_id JOIN media_items m ON m.id=e.media_id JOIN native_asset_libraries al ON al.asset_id=e.asset_id JOIN native_libraries l ON l.id=al.library_id LEFT JOIN native_tmdb_details d ON d.tmdb_type='season' AND d.tmdb_id=$2 AND d.season_number=b.season AND d.fetched_at IS NOT NULL AND d.language=coalesce((SELECT nullif(settings->>'language','') FROM integrations WHERE kind='tmdb' AND enabled ORDER BY created_at LIMIT 1),'zh-CN') LEFT JOIN native_tmdb_details sdseries ON sdseries.tmdb_type='series' AND sdseries.tmdb_id=$2 AND sdseries.fetched_at IS NOT NULL AND sdseries.language=coalesce((SELECT nullif(settings->>'language','') FROM integrations WHERE kind='tmdb' AND enabled ORDER BY created_at LIMIT 1),'zh-CN') WHERE e.media_type='series' AND l.id=$1 AND NOT l.hidden AND b.season IS NOT NULL GROUP BY b.season,l.item_id,d.payload,sdseries.payload ORDER BY b.season`, visible.RequestedID, tmdbID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []dto.BaseItemDto{}
	for rows.Next() {
		var number int
		var seriesName, year, parent string
		var details, seriesDetails []byte
		var created time.Time
		var count int
		if err := rows.Scan(&number, &seriesName, &year, &created, &parent, &count, &details, &seriesDetails); err != nil {
			return nil, err
		}
		var d, sd nativelibrary.TMDBDetails
		if err := json.Unmarshal(details, &d); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(seriesDetails, &sd); err != nil {
			return nil, err
		}
		name := d.Name
		if name == "" {
			name = seasonName(number, d)
		}
		k, e := nativelibrary.SeasonKey(tmdbID, number)
		if e != nil {
			return nil, e
		}
		idmap, e := nativelibrary.EnsureIDs(ctx, IDQueries{DB: r.DB}, []nativelibrary.Key{k})
		if e != nil {
			return nil, e
		}
		item, e := nativelibrary.SeasonDTO(nativelibrary.SeasonProjection{ID: strconv.FormatInt(idmap[k], 10), SeriesID: seriesID, ServerID: "", ParentID: parent, SeriesName: seriesName, Name: name, Year: year, TMDBID: tmdbID, Details: &d, SeriesDetails: &sd, EpisodeCount: count, CreatedAt: created})
		if e != nil {
			return nil, e
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
func (r HierarchyRepository) ListEpisodes(ctx context.Context, q nativelibrary.EpisodeQuery) ([]dto.BaseItemDto, int, error) {
	if r.DB == nil {
		return nil, 0, nativelibrary.ErrSettingsUnavailable
	}
	if !q.Visible.Valid() || q.Limit < 1 || q.Limit > 200 || q.StartIndex < 0 || q.StartIndex > 1000000 {
		return nil, 0, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	var tmdbID int64
	if err := r.DB.QueryRow(ctx, `SELECT substring(n.internal_key from 6)::bigint FROM native_item_ids n WHERE n.id=$1::bigint AND n.entity_type='series'`, q.SeriesID).Scan(&tmdbID); err != nil {
		return nil, 0, err
	}
	seasonFilter := -1
	if q.TargetSeason != nil {
		seasonFilter = *q.TargetSeason
	}
	if q.SeasonID != "" && q.TargetSeason == nil {
		if err := r.DB.QueryRow(ctx, `SELECT substring(n.internal_key FROM ':s([0-9]+)$')::int FROM native_item_ids n WHERE n.id=$1::bigint AND n.entity_type='season'`, q.SeasonID).Scan(&seasonFilter); err != nil {
			return nil, 0, err
		}
	}
	// The first query pages the logical episode keys, not physical assets. This
	// keeps long series browseable even when every episode has many editions.
	keyCTE := `WITH eligible AS (` + ExposableAssetsSQL + `), asset_keys AS(
 SELECT DISTINCT b.season,coalesce((SELECT min(value::int) FROM jsonb_array_elements_text(b.episodes) WHERE value~'^[0-9]+$'),b.episode) first_episode
 FROM eligible e JOIN media_asset_bindings b ON b.asset_id=e.asset_id JOIN native_asset_libraries al ON al.asset_id=e.asset_id JOIN native_libraries l ON l.id=al.library_id
 WHERE e.media_type='series' AND e.tmdb_id=$6 AND l.id=$1 AND NOT l.hidden AND b.season IS NOT NULL AND ($2<0 OR b.season=$2) AND ($5<0 OR coalesce((SELECT min(value::int) FROM jsonb_array_elements_text(b.episodes) WHERE value~'^[0-9]+$'),b.episode)=$5)), episode_keys AS(
 SELECT season,first_episode FROM asset_keys WHERE first_episode IS NOT NULL GROUP BY season,first_episode)
 SELECT season,first_episode,count(*) OVER() AS total FROM episode_keys ORDER BY season,first_episode LIMIT $3 OFFSET $4`
	rows, err := r.DB.Query(ctx, keyCTE, q.Visible.RequestedID, seasonFilter, q.Limit, q.StartIndex, targetEpisode(q), tmdbID)
	if err != nil {
		return nil, 0, err
	}
	type pageKey struct{ season, episode int }
	pageKeys := []pageKey{}
	total := 0
	for rows.Next() {
		var key pageKey
		if err := rows.Scan(&key.season, &key.episode, &total); err != nil {
			rows.Close()
			return nil, 0, err
		}
		pageKeys = append(pageKeys, key)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, 0, err
	}
	rows.Close()
	if len(pageKeys) == 0 {
		return []dto.BaseItemDto{}, total, nil
	}
	// Recompute the same page keys in SQL and fetch only the newest 100 physical
	// assets per logical episode. TMDB payloads are intentionally not joined here.
	assetSQL := `WITH eligible AS (` + ExposableAssetsSQL + `), page_keys AS(
 SELECT DISTINCT b.season,coalesce((SELECT min(value::int) FROM jsonb_array_elements_text(b.episodes) WHERE value~'^[0-9]+$'),b.episode) first_episode
 FROM eligible e JOIN media_asset_bindings b ON b.asset_id=e.asset_id JOIN native_asset_libraries al ON al.asset_id=e.asset_id JOIN native_libraries l ON l.id=al.library_id
 WHERE e.media_type='series' AND e.tmdb_id=$6 AND l.id=$1 AND NOT l.hidden AND b.season IS NOT NULL AND ($2<0 OR b.season=$2) AND ($5<0 OR coalesce((SELECT min(value::int) FROM jsonb_array_elements_text(b.episodes) WHERE value~'^[0-9]+$'),b.episode)=$5)
 GROUP BY b.season,coalesce((SELECT min(value::int) FROM jsonb_array_elements_text(b.episodes) WHERE value~'^[0-9]+$'),b.episode)
 ORDER BY b.season,first_episode LIMIT $3 OFFSET $4), ranked AS(
 SELECT e.asset_id,e.asset_identity,e.created_at,b.season,b.episode,b.episodes,f.full_path,m.title,m.release_year,l.item_id::text AS parent,
  coalesce((SELECT min(value::int) FROM jsonb_array_elements_text(b.episodes) WHERE value~'^[0-9]+$'),b.episode) first_episode,
  row_number() OVER(PARTITION BY b.season,coalesce((SELECT min(value::int) FROM jsonb_array_elements_text(b.episodes) WHERE value~'^[0-9]+$'),b.episode) ORDER BY e.created_at DESC,e.asset_id DESC) version_rank,
  count(*) OVER(PARTITION BY b.season,coalesce((SELECT min(value::int) FROM jsonb_array_elements_text(b.episodes) WHERE value~'^[0-9]+$'),b.episode)) version_count
 FROM eligible e JOIN media_asset_bindings b ON b.asset_id=e.asset_id JOIN cloud_files f ON f.id=e.cloud_file_id JOIN media_items m ON m.id=e.media_id JOIN native_asset_libraries al ON al.asset_id=e.asset_id JOIN native_libraries l ON l.id=al.library_id
 JOIN page_keys pk ON pk.season=b.season AND pk.first_episode=coalesce((SELECT min(value::int) FROM jsonb_array_elements_text(b.episodes) WHERE value~'^[0-9]+$'),b.episode)
 WHERE e.media_type='series' AND e.tmdb_id=$6 AND l.id=$1 AND NOT l.hidden)
 SELECT asset_id,asset_identity,created_at,season,episode,episodes,full_path,title,release_year,parent,version_count FROM ranked WHERE version_rank<=100 ORDER BY season,first_episode,version_rank`
	rows, err = r.DB.Query(ctx, assetSQL, q.Visible.RequestedID, seasonFilter, q.Limit, q.StartIndex, targetEpisode(q), tmdbID)
	if err != nil {
		return nil, 0, err
	}
	type rawAsset struct {
		asset, parent                 string
		identity, episodes            []byte
		created                       time.Time
		season, episode, versionCount int
		path, title, year             string
	}
	assets := []rawAsset{}
	for rows.Next() {
		var x rawAsset
		if err := rows.Scan(&x.asset, &x.identity, &x.created, &x.season, &x.episode, &x.episodes, &x.path, &x.title, &x.year, &x.parent, &x.versionCount); err != nil {
			rows.Close()
			return nil, 0, err
		}
		assets = append(assets, x)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, 0, err
	}
	rows.Close()
	// Fetch each TMDB snapshot once per request, not once per physical asset.
	var seriesRaw []byte
	if err := r.DB.QueryRow(ctx, `SELECT coalesce(d.payload,'{}'::jsonb)||jsonb_build_object('posterPath',coalesce(nullif(d.payload->>'posterPath',''),m.metadata->>'posterPath'),'backdropPath',coalesce(nullif(d.payload->>'backdropPath',''),m.metadata->>'backdropPath')) FROM media_items m LEFT JOIN native_tmdb_details d ON d.tmdb_type='series' AND d.tmdb_id=m.tmdb_id AND d.fetched_at IS NOT NULL AND d.language=coalesce((SELECT nullif(settings->>'language','') FROM integrations WHERE kind='tmdb' AND enabled ORDER BY created_at LIMIT 1),'zh-CN') WHERE m.media_type='series' AND m.tmdb_id=$1 ORDER BY d.fetched_at DESC NULLS LAST LIMIT 1`, tmdbID).Scan(&seriesRaw); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, err
	}
	var seriesDetails nativelibrary.TMDBDetails
	if len(seriesRaw) > 0 {
		if err := json.Unmarshal(seriesRaw, &seriesDetails); err != nil {
			return nil, 0, err
		}
	}
	seasonNumbers := make([]int, 0, len(pageKeys))
	for _, key := range pageKeys {
		seasonNumbers = append(seasonNumbers, key.season)
	}
	seasonDetails := map[int]nativelibrary.TMDBDetails{}
	seasonRows, err := r.DB.Query(ctx, `SELECT DISTINCT ON (season_number) season_number,payload FROM native_tmdb_details WHERE tmdb_type='season' AND tmdb_id=$1 AND season_number=ANY($2) AND fetched_at IS NOT NULL ORDER BY season_number,fetched_at DESC`, tmdbID, seasonNumbers)
	if err != nil {
		return nil, 0, err
	}
	for seasonRows.Next() {
		var number int
		var raw []byte
		if err := seasonRows.Scan(&number, &raw); err != nil {
			seasonRows.Close()
			return nil, 0, err
		}
		var details nativelibrary.TMDBDetails
		if err := json.Unmarshal(raw, &details); err != nil {
			seasonRows.Close()
			return nil, 0, err
		}
		seasonDetails[number] = details
	}
	if err := seasonRows.Err(); err != nil {
		seasonRows.Close()
		return nil, 0, err
	}
	seasonRows.Close()
	groups := map[string]nativelibrary.EpisodeProjection{}
	for _, asset := range assets {
		var identity metadata.Item
		if err := json.Unmarshal(asset.identity, &identity); err != nil {
			continue
		}
		episodes := identity.EpisodeDetails
		if len(episodes) == 0 {
			episodes = []metadata.Episode{{Season: asset.season, Number: asset.episode, Title: asset.title}}
		}
		sort.Slice(episodes, func(i, j int) bool { return episodes[i].Number < episodes[j].Number })
		selected := []metadata.Episode{}
		for _, episode := range episodes {
			if episode.Season == asset.season && episode.Number == asset.episode || episode.Season == asset.season && episode.Number > 0 {
				selected = append(selected, episode)
			}
		}
		if len(selected) == 0 {
			continue
		}
		first := selected[0]
		key := strconv.Itoa(first.Season) + ":" + strconv.Itoa(first.Number)
		details := seasonDetails[first.Season]
		for _, ep := range details.Episodes {
			if ep.Number == first.Number && first.StillPath == "" {
				first.StillPath = ep.StillPath
			}
		}
		runtime := first.Runtime
		if runtime == 0 {
			for _, ep := range details.Episodes {
				if ep.Number == first.Number {
					runtime = ep.RuntimeMinutes
					break
				}
			}
		}
		var ticks *int64
		if runtime > 0 {
			v := int64(runtime) * 60 * 10000000
			ticks = &v
		}
		end := first.Number
		for _, ep := range selected {
			if ep.Number > end {
				end = ep.Number
			}
		}
		source := nativelibrary.MovieSource{ID: asset.asset, Container: extension(asset.path), RunTimeTicks: ticks}
		if old, ok := groups[key]; ok {
			old.Sources = append(old.Sources, source)
			if end > old.NumberEnd {
				old.NumberEnd = end
			}
			if asset.versionCount > 100 {
				old.VersionOverflow = true
			}
			groups[key] = old
			continue
		}
		seasonID := ""
		sk, err := nativelibrary.SeasonKey(tmdbID, first.Season)
		if err != nil {
			return nil, 0, err
		}
		seasonIDs, err := nativelibrary.EnsureIDs(ctx, IDQueries{DB: r.DB}, []nativelibrary.Key{sk})
		if err != nil {
			return nil, 0, err
		}
		seasonID = strconv.FormatInt(seasonIDs[sk], 10)
		name := first.Title
		if name == "" {
			name = "第 " + strconv.Itoa(first.Number) + " 集"
		}
		groups[key] = nativelibrary.EpisodeProjection{SeriesID: q.SeriesID, SeasonID: seasonID, ParentID: seasonID, SeriesName: asset.title, SeasonName: seasonName(first.Season, details), Name: name, Overview: first.Overview, Year: asset.year, TMDBID: tmdbID, Number: first.Number, NumberEnd: end, Season: first.Season, AirDate: first.AirDate, RuntimeTicks: ticks, StillPath: first.StillPath, FallbackPoster: firstNonempty(details.PosterPath, seriesDetails.PosterPath), SeriesDetails: &seriesDetails, Sources: []nativelibrary.MovieSource{source}, CreatedAt: asset.created, VersionOverflow: asset.versionCount > 100}
	}
	var pageSources []*nativelibrary.MovieSource
	for _, item := range groups {
		for i := range item.Sources {
			pageSources = append(pageSources, &item.Sources[i])
		}
	}
	if err := hydrateProbeSources(ctx, r.DB, pageSources); err != nil {
		return nil, 0, err
	}
	list := make([]nativelibrary.EpisodeProjection, 0, len(groups))
	for _, item := range groups {
		list = append(list, item)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Season != list[j].Season {
			return list[i].Season < list[j].Season
		}
		return list[i].Number < list[j].Number
	})
	var imageIDs []string
	for i := range list {
		key, e := nativelibrary.EpisodeKey(tmdbID, list[i].Season, list[i].Number)
		if e != nil {
			return nil, 0, e
		}
		ids, e := nativelibrary.EnsureIDs(ctx, IDQueries{DB: r.DB}, []nativelibrary.Key{key})
		if e != nil {
			return nil, 0, e
		}
		list[i].ID = strconv.FormatInt(ids[key], 10)
		imageIDs = append(imageIDs, list[i].ID)
	}
	images, e := episodeImagePage(ctx, r.DB, imageIDs)
	if e != nil {
		return nil, 0, e
	}
	out := make([]dto.BaseItemDto, 0, len(list))
	for _, item := range list {
		image := images[item.ID]
		item.LocalStillTag = image.Tag
		item.StillPath = firstNonempty(image.Still, item.StillPath)
		item.FallbackPoster = firstNonempty(image.Poster, item.FallbackPoster)
		mapped, err := nativelibrary.EpisodeDTO(item)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, mapped)
	}
	return out, total, nil
}

func (r HierarchyRepository) GetEpisode(ctx context.Context, q nativelibrary.EpisodeQuery, episodeID string) (dto.BaseItemDto, error) {
	if episodeID == "" || !q.Visible.Valid() {
		return dto.BaseItemDto{}, nativelibrary.ErrNotFound
	}
	var entity, internal string
	if err := r.DB.QueryRow(ctx, `SELECT entity_type,internal_key FROM native_item_ids WHERE id=$1::bigint AND retired_at IS NULL`, episodeID).Scan(&entity, &internal); err != nil {
		return dto.BaseItemDto{}, nativelibrary.ErrNotFound
	}
	parts := strings.Split(internal, ":")
	if entity != string(nativelibrary.EntityEpisode) || len(parts) != 3 || parts[0] != "tmdb" || !strings.HasPrefix(parts[2], "s") || !strings.Contains(parts[2], "e") {
		return dto.BaseItemDto{}, nativelibrary.ErrNotFound
	}
	seriesTMDB, err1 := strconv.ParseInt(parts[1], 10, 64)
	seasonPart, episodePart, _ := strings.Cut(strings.TrimPrefix(parts[2], "s"), "e")
	season, err2 := strconv.Atoi(seasonPart)
	episode, err3 := strconv.Atoi(episodePart)
	if err1 != nil || err2 != nil || err3 != nil {
		return dto.BaseItemDto{}, nativelibrary.ErrNotFound
	}
	var seriesID string
	if err := r.DB.QueryRow(ctx, `SELECT id::text FROM native_item_ids WHERE entity_type='series' AND internal_key=$1 AND retired_at IS NULL`, "tmdb:"+strconv.FormatInt(seriesTMDB, 10)).Scan(&seriesID); err != nil {
		return dto.BaseItemDto{}, nativelibrary.ErrNotFound
	}
	q.SeriesID = seriesID
	q.TargetSeason = &season
	q.TargetEpisode = &episode
	q.StartIndex = 0
	q.Limit = 1
	items, _, err := r.ListEpisodes(ctx, q)
	if err != nil {
		return dto.BaseItemDto{}, err
	}
	for _, item := range items {
		if item.Id == episodeID {
			return item, nil
		}
	}
	return dto.BaseItemDto{}, nativelibrary.ErrNotFound
}
func targetEpisode(q nativelibrary.EpisodeQuery) int {
	if q.TargetEpisode == nil {
		return -1
	}
	return *q.TargetEpisode
}

func numeric(s string) bool { _, err := strconv.ParseInt(s, 10, 64); return err == nil }
func extension(path string) string {
	base := path
	if i := strings.LastIndex(base, "."); i >= 0 && i < len(base)-1 {
		return strings.ToLower(base[i+1:])
	}
	return "mkv"
}
func seasonName(number int, d nativelibrary.TMDBDetails) string {
	if d.Name != "" {
		return d.Name
	}
	if number == 0 {
		return "特别篇"
	}
	return "第 " + strconv.Itoa(number) + " 季"
}
