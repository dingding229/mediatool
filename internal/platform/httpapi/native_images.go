package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
	"media-tool/internal/modules/embyapi"
	"media-tool/internal/modules/imageedit"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/metadata"
	"media-tool/internal/modules/nativelibrary"
	"media-tool/internal/modules/nativelibrary/catalog"
)

type StillReader interface {
	ReadStill(context.Context, string) ([]byte, string, error)
}
type NativeImageResolver struct {
	DB           *pgxpool.Pool
	Integrations *integrations.Manager
	Redis        *redis.Client
	ImageBaseURL string
	Stills       StillReader
	ImageRoot    string
	group        singleflight.Group
}

func NewNativeImageResolver(db *pgxpool.Pool, manager *integrations.Manager, cache *redis.Client) *NativeImageResolver {
	return &NativeImageResolver{DB: db, Integrations: manager, Redis: cache}
}

type imageMeta struct {
	poster, backdrop, logo, still string
	backdrops, logos              []string
	localBody                     []byte
	localContent, localTag        string
	exposed                       bool
}

func (r *NativeImageResolver) ResolveImage(ctx context.Context, req embyapi.ImageRequest) (embyapi.ImageResult, error) {
	if r.DB == nil {
		return embyapi.ImageResult{}, embyapi.ErrImageUnavailable
	}
	if req.User == nil || req.Permissions == nil {
		return embyapi.ImageResult{}, embyapi.ErrImageNotFound
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	id, idErr := strconv.ParseInt(req.ItemID, 10, 64)
	person, isPerson := nativelibrary.ParsePersonID(req.ItemID)
	if !isPerson && (idErr != nil || id < 100000 || strconv.FormatInt(id, 10) != req.ItemID) {
		return embyapi.ImageResult{}, embyapi.ErrImageNotFound
	}
	if !isPerson {
		visible, err := (catalog.BrowseRepository{DB: r.DB}).ImageVisible(ctx, *req.User, req.Permissions, req.ItemID)
		if err != nil {
			return embyapi.ImageResult{}, embyapi.ErrImageUnavailable
		}
		if !visible {
			return embyapi.ImageResult{}, embyapi.ErrImageNotFound
		}
	}
	kind := strings.ToLower(req.Type)
	if kind != "primary" && kind != "backdrop" && kind != "thumb" && kind != "logo" && kind != "banner" {
		return embyapi.ImageResult{}, embyapi.ErrImageNotFound
	}
	if req.Index > 0 && kind != "backdrop" {
		return embyapi.ImageResult{}, embyapi.ErrImageNotFound
	}
	if !isPerson {
		custom, e := imageedit.NativePage(ctx, r.DB, []string{req.ItemID})
		if e != nil {
			return embyapi.ImageResult{}, embyapi.ErrImageUnavailable
		}
		key := map[string]string{"primary": "Primary", "thumb": "Primary", "backdrop": "Backdrop", "logo": "Logo"}[kind]
		if im, ok := custom[req.ItemID][key]; ok {
			if req.Index > 0 {
				return embyapi.ImageResult{}, embyapi.ErrImageNotFound
			}
			body, ct, tag, e := (imageedit.Repository{DB: r.DB, Root: r.ImageRoot}).File(ctx, im.ID)
			if e != nil {
				return embyapi.ImageResult{}, embyapi.ErrImageUnavailable
			}
			return embyapi.ImageResult{Body: body, ContentType: ct, Tag: tag, MaxAge: 0}, nil
		}
	}
	var meta imageMeta
	var err error
	if isPerson {
		if kind != "primary" || req.Index > 0 {
			return embyapi.ImageResult{}, embyapi.ErrImageNotFound
		}
		meta, err = r.lookupPerson(ctx, person, req)
	} else {
		meta, err = r.lookup(ctx, id, kind, req.Index)
	}
	if err != nil {
		return embyapi.ImageResult{}, err
	}
	if len(meta.localBody) > 0 {
		return embyapi.ImageResult{Body: meta.localBody, ContentType: meta.localContent, Tag: meta.localTag, MaxAge: 31536000}, nil
	}
	path := selectImage(kind, req.Index, meta)
	if path == "" || !validTMDBImagePath(path) {
		return embyapi.ImageResult{}, embyapi.ErrImageNotFound
	}
	size := imageSize(req.RequestedWidth)
	raw, content, err := r.load(ctx, size, path)
	if err != nil {
		return embyapi.ImageResult{}, err
	}
	return embyapi.ImageResult{Body: raw, ContentType: content, Tag: nativelibrary.ImageTag(path), MaxAge: 31536000}, nil
}
func (r *NativeImageResolver) lookup(ctx context.Context, id int64, kind string, index int) (imageMeta, error) {
	var entity, internal string
	if err := r.DB.QueryRow(ctx, `SELECT entity_type,internal_key FROM native_item_ids WHERE id=$1 AND retired_at IS NULL`, id).Scan(&entity, &internal); err != nil {
		return imageMeta{}, embyapi.ErrImageNotFound
	}
	switch entity {
	case string(nativelibrary.EntityMovie), string(nativelibrary.EntitySeries):
		return r.lookupMovieSeries(ctx, id, entity)
	case string(nativelibrary.EntitySeason):
		return r.lookupSeason(ctx, id, internal)
	case string(nativelibrary.EntityEpisode):
		return r.lookupEpisode(ctx, id, internal, kind == "primary")
	default:
		return imageMeta{}, embyapi.ErrImageNotFound
	}
}
func (r *NativeImageResolver) lookupMovieSeries(ctx context.Context, id int64, entity string) (imageMeta, error) {
	mediaType := "movie"
	if entity == string(nativelibrary.EntitySeries) {
		mediaType = "series"
	}
	var metadataRaw, detailsRaw []byte
	err := r.DB.QueryRow(ctx, `WITH eligible AS (`+catalog.ExposableAssetsSQL+`) SELECT m.metadata,coalesce(d.payload,'{}'::jsonb) FROM native_item_ids n JOIN media_items m ON m.media_type=$2 AND n.entity_type=$1 AND n.internal_key='tmdb:'||m.tmdb_id LEFT JOIN native_tmdb_details d ON d.tmdb_type=$2 AND d.tmdb_id=m.tmdb_id AND d.fetched_at IS NOT NULL AND d.language=coalesce((SELECT nullif(settings->>'language','') FROM integrations WHERE kind='tmdb' AND enabled ORDER BY created_at LIMIT 1),'zh-CN') WHERE n.id=$3 AND EXISTS(SELECT 1 FROM eligible e JOIN native_asset_libraries al ON al.asset_id=e.asset_id JOIN native_libraries l ON l.id=al.library_id WHERE e.media_id=m.id AND NOT l.hidden) ORDER BY d.fetched_at DESC NULLS LAST LIMIT 1`, entity, mediaType, id).Scan(&metadataRaw, &detailsRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return imageMeta{}, embyapi.ErrImageNotFound
	}
	if err != nil {
		return imageMeta{}, embyapi.ErrImageUnavailable
	}
	var metadata nativelibrary.StoredMovieMetadata
	var details nativelibrary.TMDBDetails
	if json.Unmarshal(metadataRaw, &metadata) != nil || json.Unmarshal(detailsRaw, &details) != nil {
		return imageMeta{}, embyapi.ErrImageUnavailable
	}
	return imageMeta{poster: first(details.PosterPath, metadata.PosterPath), backdrop: first(details.BackdropPath, metadata.BackdropPath), logo: details.LogoPath, backdrops: details.Backdrops, logos: details.Logos}, nil
}
func (r *NativeImageResolver) lookupSeason(ctx context.Context, id int64, internal string) (imageMeta, error) {
	series, season, ok := parseSeasonKey(internal)
	if !ok {
		return imageMeta{}, embyapi.ErrImageNotFound
	}
	var seasonRaw, seriesRaw []byte
	err := r.DB.QueryRow(ctx, `WITH eligible AS (`+catalog.ExposableAssetsSQL+`) SELECT coalesce(sd.payload,'{}'::jsonb),coalesce(ss.payload,'{}'::jsonb) FROM native_item_ids n LEFT JOIN native_tmdb_details sd ON sd.tmdb_type='season' AND sd.tmdb_id=$1 AND sd.season_number=$2 AND sd.fetched_at IS NOT NULL AND sd.language=coalesce((SELECT nullif(settings->>'language','') FROM integrations WHERE kind='tmdb' AND enabled ORDER BY created_at LIMIT 1),'zh-CN') LEFT JOIN native_tmdb_details ss ON ss.tmdb_type='series' AND ss.tmdb_id=$1 AND ss.fetched_at IS NOT NULL AND ss.language=coalesce((SELECT nullif(settings->>'language','') FROM integrations WHERE kind='tmdb' AND enabled ORDER BY created_at LIMIT 1),'zh-CN') WHERE n.id=$3 AND n.entity_type='season' AND EXISTS(SELECT 1 FROM eligible e JOIN native_asset_libraries al ON al.asset_id=e.asset_id JOIN native_libraries l ON l.id=al.library_id WHERE e.media_type='series' AND e.tmdb_id=$1 AND e.season=$2 AND NOT l.hidden) ORDER BY sd.fetched_at DESC NULLS LAST LIMIT 1`, series, season, id).Scan(&seasonRaw, &seriesRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return imageMeta{}, embyapi.ErrImageNotFound
	}
	if err != nil {
		return imageMeta{}, embyapi.ErrImageUnavailable
	}
	var sd, ss nativelibrary.TMDBDetails
	if json.Unmarshal(seasonRaw, &sd) != nil || json.Unmarshal(seriesRaw, &ss) != nil {
		return imageMeta{}, embyapi.ErrImageUnavailable
	}
	return imageMeta{poster: first(sd.PosterPath, ss.PosterPath), backdrop: ss.BackdropPath, logo: ss.LogoPath, backdrops: ss.Backdrops, logos: ss.Logos}, nil
}
func (r *NativeImageResolver) lookupEpisode(ctx context.Context, id int64, internal string, primary bool) (imageMeta, error) {
	series, season, episode, ok := parseEpisodeKey(internal)
	if !ok {
		return imageMeta{}, embyapi.ErrImageNotFound
	}
	rows, err := r.DB.Query(ctx, `WITH eligible AS (`+catalog.ExposableAssetsSQL+`) SELECT e.asset_identity,jsonb_build_object('posterPath',sd.payload->>'posterPath','episodes',coalesce((SELECT jsonb_agg(ep) FROM jsonb_array_elements(CASE WHEN jsonb_typeof(sd.payload->'episodes')='array' THEN sd.payload->'episodes' ELSE '[]'::jsonb END) ep WHERE ep->>'number'=$3::int::text),'[]'::jsonb)),jsonb_build_object('posterPath',coalesce(nullif(ss.payload->>'posterPath',''),m.metadata->>'posterPath'),'backdropPath',coalesce(nullif(ss.payload->>'backdropPath',''),m.metadata->>'backdropPath'),'logoPath',ss.payload->>'logoPath','backdrops',ss.payload->'backdrops','logos',ss.payload->'logos'),coalesce(ed.payload->>'stillPath','') FROM eligible e JOIN media_asset_bindings b ON b.asset_id=e.asset_id JOIN media_items m ON m.id=e.media_id LEFT JOIN native_tmdb_details sd ON sd.tmdb_type='season' AND sd.tmdb_id=$1 AND sd.season_number=$2 AND sd.fetched_at IS NOT NULL AND sd.language=coalesce((SELECT nullif(settings->>'language','') FROM integrations WHERE kind='tmdb' AND enabled ORDER BY created_at LIMIT 1),'zh-CN') LEFT JOIN native_tmdb_details ss ON ss.tmdb_type='series' AND ss.tmdb_id=$1 AND ss.fetched_at IS NOT NULL AND ss.language=coalesce((SELECT nullif(settings->>'language','') FROM integrations WHERE kind='tmdb' AND enabled ORDER BY created_at LIMIT 1),'zh-CN') LEFT JOIN native_tmdb_details ed ON ed.tmdb_type='episode' AND ed.tmdb_id=$1 AND ed.season_number=$2 AND ed.episode_number=$3 AND ed.fetched_at IS NOT NULL AND ed.language=coalesce((SELECT nullif(settings->>'language','') FROM integrations WHERE kind='tmdb' AND enabled ORDER BY created_at LIMIT 1),'zh-CN') WHERE e.media_type='series' AND e.tmdb_id=$1 AND b.season=$2 AND coalesce((SELECT min(v::int) FROM jsonb_array_elements_text(CASE WHEN jsonb_typeof(b.episodes)='array' THEN b.episodes ELSE '[]'::jsonb END) v WHERE v~'^[0-9]{1,9}$'),b.episode)=$3 AND EXISTS(SELECT 1 FROM native_asset_libraries al JOIN native_libraries l ON l.id=al.library_id WHERE al.asset_id=e.asset_id AND NOT l.hidden) AND EXISTS(SELECT 1 FROM native_item_ids n WHERE n.id=$4 AND n.entity_type='episode') ORDER BY e.created_at DESC,e.asset_id LIMIT 100`, series, season, episode, id)
	if err != nil {
		return imageMeta{}, embyapi.ErrImageUnavailable
	}
	defer rows.Close()
	var fallback imageMeta
	for rows.Next() {
		var identityRaw, seasonRaw, seriesRaw []byte
		var episodeStill string
		if err := rows.Scan(&identityRaw, &seasonRaw, &seriesRaw, &episodeStill); err != nil {
			return imageMeta{}, err
		}
		var identity metadata.Item
		var sd, ss nativelibrary.TMDBDetails
		if json.Unmarshal(identityRaw, &identity) != nil || json.Unmarshal(seasonRaw, &sd) != nil || json.Unmarshal(seriesRaw, &ss) != nil {
			return imageMeta{}, embyapi.ErrImageUnavailable
		}
		fallback = imageMeta{exposed: true, poster: first(sd.PosterPath, ss.PosterPath), backdrop: ss.BackdropPath, logo: ss.LogoPath, backdrops: ss.Backdrops, logos: ss.Logos}
		for _, ep := range sd.Episodes {
			if ep.Number == episode {
				episodeStill = first(episodeStill, ep.StillPath)
			}
		}
		if episodeStill != "" {
			fallback.still = episodeStill
			rows.Close()
			return r.retire(ctx, id, fallback, primary)
		}
		for _, ep := range identity.EpisodeDetails {
			if ep.Season == season && ep.Number == episode && ep.StillPath != "" {
				fallback.still = ep.StillPath
				rows.Close()
				return r.retire(ctx, id, fallback, primary)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return imageMeta{}, err
	}
	rows.Close()
	if r.Stills != nil && primary && fallback.exposed {
		body, tag, readErr := r.Stills.ReadStill(ctx, strconv.FormatInt(id, 10))
		if readErr == nil && len(body) > 0 && tag != "" {
			fallback.localBody = body
			fallback.localContent = "image/jpeg"
			fallback.localTag = tag
			return fallback, nil
		}
	}
	if fallback.poster == "" && fallback.backdrop == "" && fallback.logo == "" {
		return imageMeta{}, embyapi.ErrImageNotFound
	}
	return fallback, nil
}
func (r *NativeImageResolver) load(ctx context.Context, size, path string) ([]byte, string, error) {
	key := size + path
	v, err, _ := r.group.Do(key, func() (any, error) {
		cacheKey := "media-tool:tmdb-image-cache:" + fmt.Sprintf("%x", sha256.Sum256([]byte(size+"/"+path)))
		if r.Redis != nil {
			if raw, e := r.Redis.Get(ctx, cacheKey).Bytes(); e == nil && len(raw) > 0 && len(raw) <= 4<<20 {
				if ct := imageContentType(raw); ct != "" {
					return imageValue{raw, ct}, nil
				}
			}
		}
		client := &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		if r.Integrations != nil {
			selected, e := r.Integrations.HTTPClient(ctx, "tmdb")
			if e != nil {
				return nil, e
			}
			copy := *selected
			client = &copy
			client.Timeout = 8 * time.Second
			client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		}
		base := strings.TrimRight(r.ImageBaseURL, "/")
		if base == "" {
			base = "https://image.tmdb.org/t/p"
		}
		req, e := http.NewRequestWithContext(ctx, http.MethodGet, base+"/"+size+path, nil)
		if e != nil {
			return nil, e
		}
		resp, e := client.Do(req)
		if e != nil {
			return nil, e
		}
		raw, e := io.ReadAll(io.LimitReader(resp.Body, 4<<20+1))
		closeErr := resp.Body.Close()
		if e != nil || closeErr != nil || resp.StatusCode != http.StatusOK || len(raw) == 0 || len(raw) > 4<<20 {
			return nil, embyapi.ErrImageUnavailable
		}
		ct := imageContentType(raw)
		if ct == "" {
			return nil, embyapi.ErrImageUnavailable
		}
		if r.Redis != nil {
			if e := r.Redis.Set(ctx, cacheKey, raw, 24*time.Hour).Err(); e != nil { /* cache is best effort; response remains valid */
			}
		}
		return imageValue{raw, ct}, nil
	})
	if err != nil {
		return nil, "", err
	}
	value := v.(imageValue)
	return value.body, value.content, nil
}

type imageValue struct {
	body    []byte
	content string
}

func imageContentType(raw []byte) string {
	ct := http.DetectContentType(raw)
	if ct == "image/jpeg" || ct == "image/png" || ct == "image/webp" {
		return ct
	}
	return ""
}
func selectImage(kind string, index int, m imageMeta) string {
	if index < 0 {
		index = 0
	}
	switch kind {
	case "primary":
		for _, p := range []string{m.still, m.poster} {
			if p = nativelibrary.ImagePath(p); p != "" {
				return p
			}
		}
	case "backdrop":
		paths := nativelibrary.ImagePaths(m.backdrop, m.backdrops)
		if index < len(paths) {
			return paths[index]
		}
	case "logo":
		paths := nativelibrary.ImagePaths(m.logo, m.logos)
		if index == 0 && len(paths) > 0 {
			return paths[0]
		}
	// Thumb is available only when there is an actual episode still. Do not
	// advertise an invented Banner/Art/Thumb by treating any backdrop as one.
	case "thumb":
		if index == 0 {
			return nativelibrary.ImagePath(m.still)
		}
	}
	return ""
}
func imageSize(width int) string {
	for _, n := range []int{92, 154, 185, 342, 500, 780, 1280} {
		if width > 0 && n >= width {
			return "w" + strconv.Itoa(n)
		}
	}
	if width > 1280 {
		return "original"
	}
	return "w500"
}
func validTMDBImagePath(p string) bool { return p != "" && nativelibrary.ImagePath(p) == p }
func parseSeasonKey(raw string) (int64, int, bool) {
	parts := strings.Split(raw, ":")
	if len(parts) != 3 || parts[0] != "tmdb" || !strings.HasPrefix(parts[2], "s") {
		return 0, 0, false
	}
	series, e1 := strconv.ParseInt(parts[1], 10, 64)
	season, e2 := strconv.Atoi(strings.TrimPrefix(parts[2], "s"))
	return series, season, e1 == nil && e2 == nil && series > 0 && season >= 0
}
func parseEpisodeKey(raw string) (int64, int, int, bool) {
	parts := strings.Split(raw, ":")
	if len(parts) != 3 || parts[0] != "tmdb" {
		return 0, 0, 0, false
	}
	series, e1 := strconv.ParseInt(parts[1], 10, 64)
	s, e, ok := strings.Cut(strings.TrimPrefix(parts[2], "s"), "e")
	season, e2 := strconv.Atoi(s)
	episode, e3 := strconv.Atoi(e)
	return series, season, episode, e1 == nil && e2 == nil && e3 == nil && ok && series > 0 && season >= 0 && episode > 0
}
func first(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func (r *NativeImageResolver) retire(ctx context.Context, id int64, m imageMeta, primary bool) (imageMeta, error) {
	if primary {
		if s, ok := r.Stills.(interface {
			RetireStill(context.Context, string) error
		}); ok {
			if err := s.RetireStill(ctx, strconv.FormatInt(id, 10)); err != nil {
				return imageMeta{}, embyapi.ErrImageUnavailable
			}
		}
	}
	return m, nil
}
