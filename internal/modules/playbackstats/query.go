package playbackstats

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/platform/clientip"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrInvalid = errors.New("invalid playback statistics query")

type Filter struct {
	UserID, DeviceID   string
	Query              string
	Days, Start, Limit int
}

func (v Filter) Validate() error {
	if utf8.RuneCountInString(v.Query) > 100 || strings.ContainsRune(v.Query, 0) {
		return ErrInvalid
	}
	for _, id := range []string{v.UserID, v.DeviceID} {
		if id != "" {
			var u pgtype.UUID
			if u.Scan(id) != nil || !u.Valid {
				return ErrInvalid
			}
		}
	}
	if (v.Days != 0 && v.Days != 7 && v.Days != 30 && v.Days != 90) || v.Start < 0 || v.Start > 1000000 || v.Limit < 1 || v.Limit > 200 {
		return ErrInvalid
	}
	return nil
}
func (v Filter) args() []any {
	since := time.Unix(0, 0)
	if v.Days > 0 {
		since = time.Now().AddDate(0, 0, -v.Days)
	}
	return []any{v.UserID, v.DeviceID, since, v.Limit, v.Start}
}

const where = ` WHERE ($1::text='' OR e.user_id=nullif($1,'')::uuid) AND ($2::text='' OR e.device_id=nullif($2,'')::uuid) AND e.started_at>=$3 `

type Repository struct{ DB *pgxpool.Pool }
type Item struct {
	ItemID       string `json:"itemId"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	EpisodeTitle string `json:"episodeTitle"`
	Season       *int   `json:"season"`
	Episode      *int   `json:"episode"`
}
type Event struct {
	Item
	ID              string     `json:"id"`
	UserID          string     `json:"userId"`
	DeviceID        string     `json:"deviceId"`
	UserName        string     `json:"userName"`
	DeviceName      string     `json:"deviceName"`
	Client          string     `json:"client"`
	UserAgent       string     `json:"userAgent"`
	IP              string     `json:"ip"`
	State           string     `json:"state"`
	StartedAt       time.Time  `json:"startedAt"`
	LastSeenAt      time.Time  `json:"lastSeenAt"`
	EndedAt         *time.Time `json:"endedAt"`
	DurationSeconds float64    `json:"durationSeconds"`
	PositionTicks   int64      `json:"positionTicks"`
	RedirectCount   int64      `json:"redirectCount"`
}
type Page[T any] struct {
	Items      []T `json:"items"`
	Total      int `json:"total"`
	StartIndex int `json:"startIndex"`
	Limit      int `json:"limit"`
}
type Top struct {
	Item
	Count   int     `json:"count"`
	Seconds float64 `json:"seconds"`
}
type Day struct {
	Date    string  `json:"date"`
	Count   int     `json:"count"`
	Seconds float64 `json:"seconds"`
}
type Summary struct {
	Count    int     `json:"count"`
	Seconds  float64 `json:"seconds"`
	Movies   int     `json:"movies"`
	Series   int     `json:"series"`
	Episodes int     `json:"episodes"`
	Top      []Top   `json:"top"`
	Trend    []Day   `json:"trend"`
}

func (r Repository) Summary(ctx context.Context, f Filter) (Summary, error) {
	out := Summary{Top: []Top{}, Trend: []Day{}}
	if e := f.Validate(); e != nil {
		return out, e
	}
	args := f.args()[:3]
	e := r.DB.QueryRow(ctx, `SELECT count(*),coalesce(sum(duration_seconds),0),count(DISTINCT item_id)FILTER(WHERE media_type='movie'),count(DISTINCT series_id)FILTER(WHERE media_type='episode'),count(DISTINCT item_id)FILTER(WHERE media_type='episode') FROM media_playback_events e`+where, args...).Scan(&out.Count, &out.Seconds, &out.Movies, &out.Series, &out.Episodes)
	if e != nil {
		return out, e
	}
	rows, e := r.DB.Query(ctx, `SELECT item_id::text,name,media_type,episode_title,season,episode,count(*),sum(duration_seconds) FROM media_playback_events e`+where+`GROUP BY item_id,name,media_type,episode_title,season,episode ORDER BY count(*) DESC,max(started_at) DESC,item_id LIMIT 10`, args...)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var v Top
		if e = rows.Scan(&v.ItemID, &v.Name, &v.Type, &v.EpisodeTitle, &v.Season, &v.Episode, &v.Count, &v.Seconds); e != nil {
			break
		}
		out.Top = append(out.Top, v)
	}
	e = errors.Join(e, rows.Err())
	rows.Close()
	if e != nil {
		return out, e
	}
	rows, e = r.DB.Query(ctx, `SELECT to_char(started_at AT TIME ZONE 'Asia/Shanghai','YYYY-MM-DD'),count(*),sum(duration_seconds) FROM media_playback_events e`+where+`GROUP BY 1 ORDER BY 1`, args...)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var v Day
		if e = rows.Scan(&v.Date, &v.Count, &v.Seconds); e != nil {
			return out, e
		}
		out.Trend = append(out.Trend, v)
	}
	return out, rows.Err()
}
func (r Repository) Events(ctx context.Context, f Filter) (Page[Event], error) {
	out := Page[Event]{Items: []Event{}, StartIndex: f.Start, Limit: f.Limit}
	if e := f.Validate(); e != nil {
		return out, e
	}
	args := f.args()
	if e := r.DB.QueryRow(ctx, `SELECT count(*) FROM media_playback_events e`+where, args[:3]...).Scan(&out.Total); e != nil {
		return out, e
	}
	rows, e := r.DB.Query(ctx, `SELECT e.id::text,e.user_id::text,e.device_id::text,u.username,d.name,e.item_id::text,e.name,e.media_type,e.episode_title,e.season,e.episode,e.client,e.user_agent,coalesce(host(e.source_ip),''),CASE WHEN e.last_seen_at<now()-interval '5 minutes' THEN 'stopped' ELSE e.state END,e.started_at,e.last_seen_at,coalesce(e.ended_at,CASE WHEN e.last_seen_at<now()-interval '5 minutes' THEN e.last_seen_at END),e.duration_seconds,e.position_ticks,e.redirect_count FROM media_playback_events e JOIN media_users u ON u.id=e.user_id JOIN media_user_devices d ON d.id=e.device_id`+where+`ORDER BY e.started_at DESC,e.id LIMIT $4 OFFSET $5`, args...)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var v Event
		e = rows.Scan(&v.ID, &v.UserID, &v.DeviceID, &v.UserName, &v.DeviceName, &v.ItemID, &v.Name, &v.Type, &v.EpisodeTitle, &v.Season, &v.Episode, &v.Client, &v.UserAgent, &v.IP, &v.State, &v.StartedAt, &v.LastSeenAt, &v.EndedAt, &v.DurationSeconds, &v.PositionTicks, &v.RedirectCount)
		if e != nil {
			return out, e
		}
		v.IP = clientip.Sanitize(v.IP)
		out.Items = append(out.Items, v)
	}
	return out, rows.Err()
}

type IPRow struct {
	Country         string      `json:"country,omitempty"`
	Province        string      `json:"province,omitempty"`
	City            string      `json:"city,omitempty"`
	ISP             string      `json:"isp,omitempty"`
	Label           string      `json:"label,omitempty"`
	Accounts        []IPAccount `json:"accounts"`
	Clients         []IPClient  `json:"clients"`
	NewIP           bool        `json:"newIP"`
	DifferentRegion bool        `json:"differentRegion"`
	Location        string      `json:"location,omitempty"`
	IP              string      `json:"ip"`
	Users           int         `json:"users"`
	Devices         int         `json:"devices"`
	Count           int         `json:"count"`
	FirstSeenAt     time.Time   `json:"firstSeenAt"`
	LastSeenAt      time.Time   `json:"lastSeenAt"`
}

func (r Repository) IPs(ctx context.Context, f Filter) (Page[IPRow], error) {
	out := Page[IPRow]{Items: []IPRow{}, StartIndex: f.Start, Limit: f.Limit}
	if e := f.Validate(); e != nil {
		return out, e
	}
	args := f.args()
	base := ` FROM media_playback_ips p JOIN media_playback_events e ON e.id=p.event_id ` + where
	if e := r.DB.QueryRow(ctx, `SELECT count(DISTINCT p.ip)`+base, args[:3]...).Scan(&out.Total); e != nil {
		return out, e
	}
	rows, e := r.DB.Query(ctx, `SELECT host(p.ip),count(DISTINCT e.user_id),count(DISTINCT e.device_id),count(DISTINCT e.id),min(p.first_seen_at),max(p.last_seen_at)`+base+`GROUP BY p.ip ORDER BY max(p.last_seen_at) DESC,p.ip LIMIT $4 OFFSET $5`, args...)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var v IPRow
		if e = rows.Scan(&v.IP, &v.Users, &v.Devices, &v.Count, &v.FirstSeenAt, &v.LastSeenAt); e != nil {
			return out, e
		}
		v.IP = clientip.Sanitize(v.IP)
		out.Items = append(out.Items, v)
	}
	return out, rows.Err()
}

// Location grouping is resolved locally at read time, never persisted as stale
// geography. The database performs distinct counts after mapping IPs to groups.
func (r Repository) GroupedIPs(ctx context.Context, f Filter, locate func(string) string) (Page[IPRow], error) {
	out := Page[IPRow]{Items: []IPRow{}, StartIndex: f.Start, Limit: f.Limit}
	if e := f.Validate(); e != nil {
		return out, e
	}
	args := f.args()[:3]
	rows, e := r.DB.Query(ctx, `SELECT DISTINCT host(p.ip) FROM media_playback_ips p JOIN media_playback_events e ON e.id=p.event_id`+where, args...)
	if e != nil {
		return out, e
	}
	locations := map[string]string{}
	groups := map[string]bool{}
	for rows.Next() {
		var ip string
		if e = rows.Scan(&ip); e != nil {
			break
		}
		label := locate(clientip.Sanitize(ip))
		if label == "" {
			label = "未知归属地"
		}
		locations[ip] = label
		groups[label] = true
	}
	e = errors.Join(e, rows.Err())
	rows.Close()
	if e != nil {
		return out, e
	}
	out.Total = len(groups)
	raw, e := json.Marshal(locations)
	if e != nil {
		return out, e
	}
	args = append(args, raw, f.Limit, f.Start)
	rows, e = r.DB.Query(ctx, `SELECT coalesce($4::jsonb->>host(p.ip),'未知归属地') AS location,count(DISTINCT e.user_id),count(DISTINCT e.device_id),count(DISTINCT e.id),min(p.first_seen_at),max(p.last_seen_at) FROM media_playback_ips p JOIN media_playback_events e ON e.id=p.event_id`+where+`GROUP BY 1 ORDER BY max(p.last_seen_at) DESC,location LIMIT $5 OFFSET $6`, args...)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var v IPRow
		if e = rows.Scan(&v.Location, &v.Users, &v.Devices, &v.Count, &v.FirstSeenAt, &v.LastSeenAt); e != nil {
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	return out, rows.Err()
}

// DeviceIPs includes successful authenticated requests, even before playback.
// Count is authenticated requests, never described as a number of plays.
func (r Repository) DeviceIPs(ctx context.Context, f Filter) (Page[IPRow], error) {
	out := Page[IPRow]{Items: []IPRow{}, StartIndex: f.Start, Limit: f.Limit}
	if e := f.Validate(); e != nil {
		return out, e
	}
	args := f.args()
	clause := ` WHERE ($1::text='' OR p.user_id=nullif($1,'')::uuid) AND ($2::text='' OR p.device_id=nullif($2,'')::uuid) AND p.last_seen_at>=$3 `
	if e := r.DB.QueryRow(ctx, `SELECT count(*) FROM media_device_ips p`+clause, args[:3]...).Scan(&out.Total); e != nil {
		return out, e
	}
	rows, e := r.DB.Query(ctx, `SELECT host(p.ip),p.request_count,p.first_seen_at,p.last_seen_at FROM media_device_ips p`+clause+`ORDER BY p.last_seen_at DESC,p.device_id,p.ip LIMIT $4 OFFSET $5`, args...)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var v IPRow
		if e = rows.Scan(&v.IP, &v.Count, &v.FirstSeenAt, &v.LastSeenAt); e != nil {
			return out, e
		}
		v.IP = clientip.Sanitize(v.IP)
		v.Users, v.Devices = 1, 1
		out.Items = append(out.Items, v)
	}
	return out, rows.Err()
}
