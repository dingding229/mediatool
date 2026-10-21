package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/modules/mediausers/nativepermissions"
	"media-tool/internal/modules/nativelibrary"
)

type BrowseRepository struct {
	DB      *pgxpool.Pool
	Logger  *slog.Logger
	warning *sync.Once
}

var defaultBrowseWarning sync.Once

func NewBrowseRepository(db *pgxpool.Pool, logger *slog.Logger) BrowseRepository {
	return BrowseRepository{DB: db, Logger: logger, warning: &sync.Once{}}
}

var _ nativelibrary.BrowseService = BrowseRepository{}

type browseRow struct {
	UserData                          dto.UserItemDataDto `json:"user_data"`
	Kind, Key                         string
	TMDB                              int64 `json:"tmdb"`
	S, E                              int
	Created                           time.Time
	Libraries                         []string
	Parent, Title, Name, Year, Rating string
	CollectionType                    string `json:"collection_type"`
	Meta                              nativelibrary.StoredMovieMetadata
	EpisodeInfo                       json.RawMessage `json:"episode_info"`
	Seasons, Episodes                 int
}
type browseScope struct {
	user       string
	ids, codes []string
	unrated    bool
	library    string
	parentKind string
}

func scope(ctx context.Context, u contracts.User, p contracts.PermissionChecker) (browseScope, error) {
	s := browseScope{user: u.ID}
	if p == nil {
		return s, contracts.ErrUnavailable
	}
	var err error
	s.ids, err = p.VisibleLibraries(ctx, u)
	if err != nil {
		return s, err
	}
	for _, code := range nativepermissions.RatingCodes() {
		err = p.ParentalAllowed(ctx, u, contracts.ItemAccess{OfficialRating: code})
		if err == nil {
			s.codes = append(s.codes, code)
		} else if !errors.Is(err, contracts.ErrParentalDenied) {
			return s, err
		}
	}
	err = p.ParentalAllowed(ctx, u, contracts.ItemAccess{})
	s.unrated = err == nil
	if err != nil && !errors.Is(err, contracts.ErrParentalDenied) {
		return s, err
	}
	return s, nil
}
func (r BrowseRepository) Views(ctx context.Context, u contracts.User, p contracts.PermissionChecker) ([]dto.BaseItemDto, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if p == nil {
		return nil, contracts.ErrUnavailable
	}
	ids, err := p.VisibleLibraries(ctx, u)
	if err != nil {
		return nil, err
	}
	if r.DB == nil {
		return nil, contracts.ErrUnavailable
	}
	rows, err := r.DB.Query(ctx, `WITH eligible AS (`+ExposableAssetsSQL+`) SELECT l.id,l.item_id::text,l.name,l.collection_type,trim(n.server_id::text),coalesce(d.favorite,false),d.last_played_at,coalesce(d.position_ticks,0),coalesce(d.play_count,0),coalesce(d.played,false)
 FROM native_libraries l CROSS JOIN native_server_settings n LEFT JOIN media_user_item_data d ON d.item_id=l.item_id AND d.user_id=$2::uuid WHERE l.id=ANY($1::text[]) AND NOT l.hidden
 AND (l.id NOT IN('f:movie','f:series') OR EXISTS(SELECT 1 FROM eligible e JOIN native_asset_libraries al ON al.asset_id=e.asset_id WHERE al.library_id=l.id)) ORDER BY l.sort_order,l.id LIMIT 200`, ids, u.ID)
	if err != nil {
		return nil, contracts.ErrUnavailable
	}
	defer rows.Close()
	out := []dto.BaseItemDto{}
	for rows.Next() {
		var l nativelibrary.Library
		var favorite, played bool
		var position int64
		var count int
		var last *time.Time
		if err := rows.Scan(&l.ID, &l.ItemID, &l.Name, &l.CollectionType, &l.ServerID, &favorite, &last, &position, &count, &played); err != nil {
			return nil, contracts.ErrUnavailable
		}
		if err := p.CanBrowse(ctx, u, l.ID); err != nil {
			return nil, err
		}
		item, err := nativelibrary.LibraryDTO(l)
		if err != nil {
			return nil, err
		}
		data := nativelibrary.CompleteUserData(dto.UserItemDataDto{IsFavorite: favorite, Played: played, PlaybackPositionTicks: &position, PlayCount: &count}, item.Id, nil)
		if last != nil {
			stamp := dto.NewUTCDateTime(*last)
			data.LastPlayedDate = &stamp
		}
		item.UserData = &data
		out = append(out, item)
	}
	if rows.Err() != nil {
		return nil, contracts.ErrUnavailable
	}
	return out, nil
}

// conditions contains only fixed expressions. Every request value is a bind.
func conditions(q nativelibrary.ItemQuery, s browseScope) (string, []any) {
	args := []any{s.ids, s.codes, s.unrated, s.library, s.user}
	where := []string{`(kind='library' OR $3::boolean OR btrim(regexp_replace(upper(btrim(rating)),'^(CN:|US:)\s*',''))=ANY($2::text[]))`}
	add := func(expr string, v any) { args = append(args, v); where = append(where, fmt.Sprintf(expr, len(args))) }
	types := []string{}
	for _, v := range q.IncludeItemTypes {
		types = append(types, itemKind(v))
	}
	if len(types) > 0 {
		add("kind=ANY($%d::text[])", types)
	}
	types = nil
	for _, v := range q.ExcludeItemTypes {
		types = append(types, itemKind(v))
	}
	if len(types) > 0 {
		add("NOT(kind=ANY($%d::text[]))", types)
	}
	if len(q.IDs) > 0 {
		add("id=ANY($%d::text[])", q.IDs)
	}
	if len(q.Years) > 0 {
		add("item_year=ANY($%d::text[])", q.Years)
	}
	if len(q.Genres) > 0 {
		add("EXISTS(SELECT 1 FROM jsonb_array_elements_text(CASE WHEN jsonb_typeof(meta->'genres')='array' THEN meta->'genres' ELSE '[]'::jsonb END) g WHERE lower(g)=ANY($%d::text[]))", lower(q.Genres))
	}
	if q.SearchTerm != "" {
		add("strpos(lower(name),lower($%d::text))>0", q.SearchTerm)
	}
	if q.IsFolder != nil {
		add("(kind NOT IN('movie','episode'))=$%d::boolean", *q.IsFolder)
	}
	for _, f := range []struct {
		v    *bool
		expr string
	}{{q.IsPlayed, "user_played=$%d::boolean"}, {q.IsFavorite, "user_favorite=$%d::boolean"}, {q.IsResumable, "(kind IN('movie','episode') AND user_position>0 AND NOT user_played)=$%d::boolean"}} {
		if f.v != nil {
			add(f.expr, *f.v)
		}
	}

	// Additional IDs intersect the ParentId library/season scope; they must not
	// replace it and accidentally borrow versions from another library.
	if q.SeriesID != "" {
		add("internal_key LIKE (SELECT internal_key FROM native_item_ids WHERE id::text=$%d::text AND entity_type='series')||':s%%'", q.SeriesID)
	}
	if q.SeasonID != "" {
		add("kind='episode' AND internal_key LIKE (SELECT internal_key FROM native_item_ids WHERE id::text=$%d::text AND entity_type='season')||'e%%'", q.SeasonID)
	}
	return strings.Join(where, " AND "), args
}
func lower(in []string) []string {
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = strings.ToLower(v)
	}
	return out
}
func (r BrowseRepository) Query(ctx context.Context, u contracts.User, p contracts.PermissionChecker, q nativelibrary.ItemQuery) ([]dto.BaseItemDto, int, error) {
	return r.query(ctx, u, p, q, browseOptions{})
}

type browseOptions struct {
	extra           func(string, []any) (string, []any)
	order           []string
	startID, search string
	bounded         bool
	skipProbes      bool
	nextUp          bool
	nextSeries      string
	nextSpecial     bool
}

const discoveryCandidateLimit = 50000

func (r BrowseRepository) query(ctx context.Context, u contracts.User, p contracts.PermissionChecker, q nativelibrary.ItemQuery, options browseOptions) ([]dto.BaseItemDto, int, error) {
	if err := validateBrowseQuery(&q); err != nil {
		return nil, 0, err
	}
	if q.SeriesID != "" || q.SeasonID != "" {
		parent := q.SeriesID
		if q.SeasonID != "" {
			parent = q.SeasonID
		}
		if q.SeriesID != "" {
			if _, e := r.single(ctx, u, p, q.SeriesID, dto.Series); e != nil {
				return nil, 0, e
			}
		}
		if q.SeasonID != "" {
			v, e := r.single(ctx, u, p, q.SeasonID, dto.Season)
			if e != nil {
				return nil, 0, e
			}
			if q.SeriesID != "" && v.SeriesId != q.SeriesID {
				return nil, 0, nativelibrary.ErrNotFound
			}
		}
		if q.ParentID == "" {
			q.ParentID = parent
		}
		recursive := true
		q.Recursive = &recursive
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if options.nextUp {
		return r.nextUpPage(ctx, u, p, q, options.nextSeries, options.nextSpecial)
	}
	planScope, where, args, err := r.plan(ctx, u, p, q)
	if err != nil {
		return nil, 0, err
	}
	order := []string{}
	if len(q.SortBy) == 0 {
		q.SortBy = []string{"SortName"}
	}
	for i, name := range q.SortBy {
		expr, ok := browseSorts[name]
		if !ok {
			return nil, 0, nativelibrary.ErrInvalidQuery
		}
		dir := "ASC"
		if len(q.SortOrder) > 0 && q.SortOrder[min(i, len(q.SortOrder)-1)] == "Descending" {
			dir = "DESC"
		}
		order = append(order, expr+" "+dir+" NULLS LAST")
	}
	if len(options.order) > 0 {
		order = options.order
	}
	order = append(order, "id::bigint ASC NULLS LAST,kind,internal_key")
	if options.extra != nil {
		where, args = options.extra(where, args)
	}
	source, prefix, within := "item_rows", "", "true"
	searchBind := 0
	if options.search != "" {
		args = append(args, options.search)
		i := len(args)
		searchBind = i
		where += fmt.Sprintf(" AND (strpos(lower(normalize(name,NFKC)),lower(normalize($%d::text,NFKC)))>0 OR strpos(lower(normalize(coalesce(meta->>'originalTitle',''),NFKC)),lower(normalize($%d::text,NFKC)))>0)", i, i)
	}
	if options.bounded {
		prefix = fmt.Sprintf(",discovery_candidates AS MATERIALIZED(SELECT * FROM item_rows WHERE %s LIMIT %d)", where, discoveryCandidateLimit+1)
		source = "discovery_candidates"
		where = "true"
		within = fmt.Sprintf("(SELECT count(*) FROM discovery_candidates)<=%d", discoveryCandidateLimit)
	}

	prefix += ",filtered AS NOT MATERIALIZED(SELECT * FROM " + source + " WHERE " + where + ")"
	pageSource, anchor, offset := "filtered", "true", ""
	if options.startID != "" {
		args = append(args, options.startID)
		i := len(args)
		prefix += ",numbered AS(SELECT *,row_number() OVER(ORDER BY " + strings.Join(order, ",") + ")-1 ordinal FROM filtered)"
		pageSource = "numbered"
		anchor = fmt.Sprintf("EXISTS(SELECT 1 FROM filtered WHERE id=$%d::text)", i)
		offset = fmt.Sprintf("coalesce((SELECT ordinal FROM numbered WHERE id=$%d::text),0)+", i)
	}
	parentBind := 0
	pruneID := q.ParentID
	pruneKind := planScope.parentKind
	if q.SeriesID != "" {
		pruneID = q.SeriesID
		pruneKind = "series"
	}
	if q.SeasonID != "" {
		pruneID = q.SeasonID
		pruneKind = "season"
	}
	if pruneKind == "library" {
		pruneID = ""
	}
	if pruneID != "" {
		args = append(args, pruneID)
		parentBind = len(args)
	} else if len(q.IDs) == 1 {
		pruneKind = ""
		args = append(args, q.IDs[0])
		parentBind = len(args)
	}

	args = append(args, q.Limit, q.StartIndex)
	sql := pruneParentSQL(browseSearchSQL(q, searchBind, true), parentBind, pruneKind) + prefix + ",page AS(SELECT " + browseColumns + " FROM " + pageSource + " ORDER BY " + strings.Join(order, ",") + fmt.Sprintf(" LIMIT $%d OFFSET "+offset+"$%d)", len(args)-1, len(args)) + " SELECT (SELECT count(*) FROM filtered),coalesce((SELECT jsonb_agg(row_to_json(page)) FROM page),'[]')," + within + "," + anchor
	var total int
	var raw []byte
	var withinCap, anchorFound bool
	if err := r.DB.QueryRow(ctx, sql, args...).Scan(&total, &raw, &withinCap, &anchorFound); err != nil {
		return nil, 0, contracts.ErrUnavailable
	}
	if !withinCap {
		return nil, 0, nativelibrary.ErrDiscoveryTooBroad
	}
	if !anchorFound {
		return nil, 0, nativelibrary.ErrNotFound
	}
	var records []browseRow
	if err := json.Unmarshal(raw, &records); err != nil {
		return nil, 0, contracts.ErrUnavailable
	}
	items, err := r.hydrate(ctx, records, q.Fields, !options.skipProbes)
	if err != nil {
		return nil, 0, err
	}
	for i, item := range items {
		data := nativelibrary.CompleteUserData(records[i].UserData, item.Id, item.RunTimeTicks)
		items[i].UserData = &data
		var err error
		if item.Type == dto.CollectionFolder {
			err = p.CanBrowse(ctx, u, records[i].Key)
		} else {
			err = p.ItemAllowed(ctx, u, contracts.ItemAccess{ID: item.Id, LibraryIDs: records[i].Libraries, OfficialRating: records[i].Rating})
		}
		if err != nil {
			return nil, 0, err
		}
	}
	return items, total, nil
}
func (r BrowseRepository) Counts(ctx context.Context, u contracts.User, p contracts.PermissionChecker, filters ...nativelibrary.ItemQuery) (nativelibrary.ItemCounts, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var out nativelibrary.ItemCounts
	q := nativelibrary.ItemQuery{}
	if len(filters) > 1 {
		return out, nativelibrary.ErrInvalidQuery
	}
	if len(filters) == 1 {
		q = filters[0]
	}
	if err := validateBrowseQuery(&q); err != nil {
		return out, err
	}
	if q.Recursive == nil {
		value := true
		q.Recursive = &value
	}
	_, where, args, err := r.plan(ctx, u, p, q)
	if err != nil {
		return out, err
	}
	err = r.DB.QueryRow(ctx, browseSQLFor(q)+`SELECT count(*) FILTER(WHERE kind='movie'),count(*) FILTER(WHERE kind='series'),count(*) FILTER(WHERE kind='episode') FROM item_rows WHERE `+where, args...).Scan(&out.MovieCount, &out.SeriesCount, &out.EpisodeCount)
	if err != nil {
		return out, contracts.ErrUnavailable
	}
	return out, nil
}

func itemKind(value string) string {
	switch strings.ToLower(value) {
	case "collectionfolder", "folder":
		return "library"
	default:
		return strings.ToLower(value)
	}
}
func validateBrowseQuery(q *nativelibrary.ItemQuery) error {
	if q.Limit < 0 || q.Limit > 200 || q.StartIndex < 0 || q.StartIndex > 1000000 {
		return nativelibrary.ErrInvalidQuery
	}
	if len(q.ParentID) > 64 {
		return nativelibrary.ErrInvalidQuery
	}
	for _, id := range q.IDs {
		if len(id) == 0 || len(id) > 64 {
			return nativelibrary.ErrInvalidQuery
		}
	}
	for _, genre := range q.Genres {
		if len(genre) == 0 || len(genre) > 128 {
			return nativelibrary.ErrInvalidQuery
		}
	}
	for _, year := range q.Years {
		n, err := strconv.Atoi(year)
		if err != nil || len(year) != 4 || n < 1000 || n > 9999 {
			return nativelibrary.ErrInvalidQuery
		}
	}
	if len(q.IDs) > 200 || len(q.Genres) > 64 || len(q.Years) > 100 || len(q.Fields) > 64 || len(q.SortBy) > 64 || len(q.SortOrder) > 64 || len(q.SearchTerm) > 256 {
		return nativelibrary.ErrInvalidQuery
	}
	for _, k := range append(slices.Clone(q.IncludeItemTypes), q.ExcludeItemTypes...) {
		if !slices.Contains([]string{"movie", "series", "season", "episode", "library"}, itemKind(k)) {
			return nativelibrary.ErrInvalidQuery
		}
	}
	for _, order := range q.SortOrder {
		if order != "Ascending" && order != "Descending" {
			return nativelibrary.ErrInvalidQuery
		}
	}
	if len(q.SortOrder) > 1 && len(q.SortOrder) != len(q.SortBy) {
		return nativelibrary.ErrInvalidQuery
	}
	sorts, orders := []string{}, []string{}
	for i, name := range q.SortBy {
		if strings.EqualFold(name, "Random") {
			continue
		}
		if _, ok := browseSorts[name]; !ok {
			return nativelibrary.ErrInvalidQuery
		}
		sorts = append(sorts, name)
		if len(q.SortOrder) > 0 {
			orders = append(orders, q.SortOrder[min(i, len(q.SortOrder)-1)])
		}
	}
	q.SortBy, q.SortOrder = sorts, orders
	return nil
}

// Query and Counts share the complete permission, parent and item-filter plan.
func (r BrowseRepository) plan(ctx context.Context, u contracts.User, p contracts.PermissionChecker, q nativelibrary.ItemQuery) (s browseScope, where string, args []any, err error) {
	s, err = scope(ctx, u, p)
	if err != nil {
		return s, "", nil, err
	}
	if r.DB == nil {
		return s, "", nil, contracts.ErrUnavailable
	}

	var parentKind, parentKey string
	if q.ParentID != "" {
		err := r.DB.QueryRow(ctx, `SELECT entity_type,internal_key FROM native_item_ids WHERE id::text=$1`, q.ParentID).Scan(&parentKind, &parentKey)
		if errors.Is(err, pgx.ErrNoRows) {
			return s, "", nil, nativelibrary.ErrNotFound
		}
		if err != nil {
			return s, "", nil, contracts.ErrUnavailable
		}
		if parentKind == "library" {
			if !slices.Contains(s.ids, parentKey) {
				return s, "", nil, nativelibrary.ErrNotFound
			}
			s.library = parentKey
		}
	}
	s.parentKind = parentKind
	if parentKind != "" {
		check, binds := conditions(nativelibrary.ItemQuery{IDs: []string{q.ParentID}}, s)
		var exists bool
		preflight := browseCTE
		if parentKind != "library" {
			binds = append(binds, q.ParentID)
			preflight = pruneParentSQL(browseCTE, len(binds), parentKind)
		}
		if err := r.DB.QueryRow(ctx, preflight+`SELECT EXISTS(SELECT 1 FROM item_rows WHERE `+check+`)`, binds...).Scan(&exists); err != nil {
			return s, "", nil, contracts.ErrUnavailable
		}
		if !exists {
			return s, "", nil, nativelibrary.ErrNotFound
		}
	}
	where, args = conditions(q, s)
	switch {
	case parentKind == "library":
		where += " AND kind<>'library'"
		if q.Recursive == nil || !*q.Recursive {
			where += " AND kind IN('movie','series')"
		}
	case parentKind != "":
		args = append(args, parentKey)
		if parentKind == "series" {
			where += fmt.Sprintf(" AND internal_key LIKE $%d::text||':s%%'", len(args))
			if q.Recursive == nil || !*q.Recursive {
				where += " AND kind='season'"
			}
		} else if parentKind == "season" {
			where += fmt.Sprintf(" AND internal_key LIKE $%d::text||'e%%' AND kind='episode'", len(args))
		} else {
			where += " AND false"
		}
	case len(q.IDs) == 0 && (q.Recursive == nil || !*q.Recursive):
		where += " AND kind='library'"
	case len(q.IDs) == 0:
		where += " AND kind<>'library'"
	}
	return s, where, args, nil
}
