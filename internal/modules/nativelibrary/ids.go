package nativelibrary

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

type Key struct {
	EntityType  EntityType `json:"entity_type"`
	InternalKey string     `json:"internal_key"`
}

// IDRows/IDQuery keep SQL drivers out of the protocol dependency graph.
type IDRows interface {
	Next() bool
	Scan(...any) error
	Err() error
	Close()
}
type IDQuery interface {
	Query(context.Context, string, ...any) (IDRows, error)
}

var ErrInvalidKey = errors.New("invalid native entity key or batch")
var keyPattern = regexp.MustCompile(`^tmdb:([1-9][0-9]*)(?::s([0-9]+)(?:e([1-9][0-9]*))?)?$`)
var libraryPattern = regexp.MustCompile(`^(?:g:[a-zA-Z0-9_-]{1,80}|r:[a-zA-Z0-9_-]{1,80}:[a-zA-Z0-9_-]{1,80}|f:movie|f:series)$`)

func (k Key) Valid() bool {
	if k.EntityType == EntityLibrary {
		return libraryPattern.MatchString(k.InternalKey)
	}
	m := keyPattern.FindStringSubmatch(k.InternalKey)
	if m == nil {
		return false
	}
	if _, err := strconv.ParseInt(m[1], 10, 64); err != nil {
		return false
	}
	for _, part := range m[2:] {
		if part == "" {
			continue
		}
		number, err := strconv.ParseInt(part, 10, 32)
		if err != nil || strconv.FormatInt(number, 10) != part {
			return false
		}
	}

	switch k.EntityType {
	case EntityMovie, EntitySeries:
		return m[2] == ""
	case EntitySeason:
		return m[2] != "" && m[3] == ""
	case EntityEpisode:
		return m[2] != "" && m[3] != ""
	default:
		return false
	}
}

// Callers supply ONLY entities evidenced by currently exposable assets (libraries
// follow T11). Two statements are intentional: an INSERT-conflict wait may expose
// a concurrently committed row only to the following READ COMMITTED snapshot.
func EnsureIDs(ctx context.Context, q IDQuery, keys []Key) (map[Key]int64, error) {
	out := map[Key]int64{}
	if len(keys) > 500 {
		return nil, ErrInvalidKey
	}
	if len(keys) == 0 {
		return out, nil
	}
	if q == nil {
		return nil, ErrInvalidKey
	}
	ordered := slices.Clone(keys)
	for _, key := range ordered {
		if !key.Valid() {
			return nil, ErrInvalidKey
		}
	}
	slices.SortFunc(ordered, func(a, b Key) int {
		if c := strings.Compare(string(a.EntityType), string(b.EntityType)); c != 0 {
			return c
		}
		return strings.Compare(a.InternalKey, b.InternalKey)
	})
	ordered = slices.Compact(ordered)
	raw, err := json.Marshal(ordered)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `INSERT INTO native_item_ids(entity_type,internal_key)
 SELECT entity_type,internal_key FROM jsonb_to_recordset($1::jsonb) AS k(entity_type text,internal_key text)
 ORDER BY entity_type,internal_key ON CONFLICT(entity_type,internal_key) DO NOTHING RETURNING id`, string(raw))
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = q.Query(ctx, `WITH wanted AS (SELECT * FROM jsonb_to_recordset($1::jsonb) AS k(entity_type text,internal_key text)),
 restored AS (UPDATE native_item_ids n SET retired_at=NULL FROM wanted k
 WHERE n.entity_type=k.entity_type AND n.internal_key=k.internal_key AND n.retired_at IS NOT NULL RETURNING n.id)
 SELECT n.entity_type,n.internal_key,n.id FROM native_item_ids n JOIN wanted k
 ON n.entity_type=k.entity_type AND n.internal_key=k.internal_key ORDER BY n.entity_type,n.internal_key`, string(raw))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key Key
		var id int64
		if err := rows.Scan(&key.EntityType, &key.InternalKey, &id); err != nil {
			return nil, err
		}
		out[key] = id
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) != len(ordered) {
		return nil, errors.New("native ID allocation incomplete; retry transaction")
	}
	return out, nil
}

func MovieKey(tmdbID int64) (Key, error)  { return tmdbKey(EntityMovie, tmdbID, "", "") }
func SeriesKey(tmdbID int64) (Key, error) { return tmdbKey(EntitySeries, tmdbID, "", "") }
func SeasonKey(seriesTMDBID int64, season int) (Key, error) {
	return tmdbKey(EntitySeason, seriesTMDBID, strconv.Itoa(season), "")
}
func EpisodeKey(seriesTMDBID int64, season, episode int) (Key, error) {
	return tmdbKey(EntityEpisode, seriesTMDBID, strconv.Itoa(season), strconv.Itoa(episode))
}
func tmdbKey(kind EntityType, id int64, season, episode string) (Key, error) {
	if id <= 0 {
		return Key{}, ErrInvalidKey
	}
	if kind == EntitySeason && season == "" || kind == EntityEpisode && (season == "" || episode == "") {
		return Key{}, ErrInvalidKey
	}
	key := "tmdb:" + strconv.FormatInt(id, 10)
	if season != "" {
		key += ":s" + season
	}
	if episode != "" {
		key += "e" + episode
	}
	k := Key{EntityType: kind, InternalKey: key}
	if !k.Valid() {
		return Key{}, ErrInvalidKey
	}
	return k, nil
}
