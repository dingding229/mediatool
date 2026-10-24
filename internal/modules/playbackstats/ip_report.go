package playbackstats

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"media-tool/internal/modules/ipgeo"
	"media-tool/internal/platform/clientip"
)

type IPAccount struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type IPClient struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Client string `json:"client"`
}
type Distribution struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}
type IPOverview struct {
	UniqueIPs     int            `json:"uniqueIPs"`
	Regions       int            `json:"regions"`
	ActiveUsers   int            `json:"activeUsers"`
	PrimaryRegion string         `json:"primaryRegion,omitempty"`
	Provinces     []Distribution `json:"provinces"`
	Cities        []Distribution `json:"cities"`
	Operators     []Distribution `json:"operators"`
}
type IPReport struct {
	Page[IPRow]
	Summary IPOverview `json:"summary"`
}

// IPReport counts sessions per IP, never redirects or repeated progress updates.
// Geography is resolved offline at read time. Search/KPIs/ranks use the complete
// filtered set, not the visible page. One snapshot also supplies all-time firsts
// within the retained history, so selecting 7 days does not turn every old IP new.
func (r Repository) IPReport(ctx context.Context, f Filter, locate func(string) ipgeo.Location) (IPReport, error) {
	out := IPReport{Page: Page[IPRow]{Items: []IPRow{}, StartIndex: f.Start, Limit: f.Limit}}
	if e := f.Validate(); e != nil {
		return out, e
	}
	rows, e := r.DB.Query(ctx, `WITH facts AS MATERIALIZED (
 SELECT p.ip,p.first_seen_at,p.last_seen_at,e.id,e.user_id,e.device_id,e.started_at
 FROM media_playback_ips p JOIN media_playback_events e ON e.id=p.event_id
 WHERE ($1::text='' OR e.user_id=nullif($1,'')::uuid) AND ($2::text='' OR e.device_id=nullif($2,'')::uuid)
 ), firsts AS (SELECT ip,min(first_seen_at) AS first_seen_at FROM facts GROUP BY ip)
 SELECT host(p.ip),count(DISTINCT p.user_id),count(DISTINCT p.device_id),count(DISTINCT p.id),min(f.first_seen_at),max(p.last_seen_at),
 jsonb_agg(DISTINCT jsonb_build_object('id',u.id::text,'name',coalesce(nullif(u.display_name,''),u.username))),
 jsonb_agg(DISTINCT jsonb_build_object('id',d.id::text,'name',d.name,'client',d.client))
 FROM facts p JOIN firsts f ON f.ip=p.ip JOIN media_users u ON u.id=p.user_id JOIN media_user_devices d ON d.user_id=p.user_id AND d.id=p.device_id
 WHERE p.started_at >= $3 GROUP BY p.ip ORDER BY max(p.last_seen_at) DESC,p.ip LIMIT 100001`, f.args()[:3]...)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	all := []IPRow{}
	scanned := 0
	for rows.Next() {
		scanned++
		if scanned > 100000 {
			return out, errors.New("IP report scope exceeds the bounded read limit")
		}
		var v IPRow
		var accounts, clients []byte
		if e = rows.Scan(&v.IP, &v.Users, &v.Devices, &v.Count, &v.FirstSeenAt, &v.LastSeenAt, &accounts, &clients); e != nil {
			return out, e
		}
		if !clientip.PublicString(v.IP) {
			continue
		}
		if e = json.Unmarshal(accounts, &v.Accounts); e != nil {
			return out, e
		}
		if e = json.Unmarshal(clients, &v.Clients); e != nil {
			return out, e
		}
		all = append(all, v)
	}
	if e = rows.Err(); e != nil {
		return out, e
	}
	return buildIPReport(all, f, locate, time.Now()), nil
}
func geography(parts ...string) string {
	out := []string{}
	for _, s := range parts {
		if s != "" && (len(out) == 0 || out[len(out)-1] != s) {
			out = append(out, s)
		}
	}
	return strings.Join(out, " ")
}
func distribution(values map[string]int) []Distribution {
	out := []Distribution{}
	for name, count := range values {
		out = append(out, Distribution{name, count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			return out[i].Name < out[j].Name
		}
		return out[i].Count > out[j].Count
	})
	return out
}
func buildIPReport(all []IPRow, f Filter, locate func(string) ipgeo.Location, now time.Time) IPReport {
	out := IPReport{Page: Page[IPRow]{Items: []IPRow{}, StartIndex: f.Start, Limit: f.Limit}, Summary: IPOverview{Provinces: []Distribution{}, Cities: []Distribution{}, Operators: []Distribution{}}}
	matched := []IPRow{}
	users, regions := map[string]bool{}, map[string]bool{}
	province, city, operator, primary := map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}
	for _, v := range all {
		if !clientip.PublicString(v.IP) {
			continue
		}
		l := locate(v.IP)
		v.IP = clientip.Sanitize(v.IP)
		v.Country, v.Province, v.City, v.ISP = l.Country, l.Province, l.City, l.ISP
		v.Location = geography(l.Country, l.Province, l.City)
		v.Label = l.Label
		v.NewIP = !v.FirstSeenAt.IsZero() && !v.FirstSeenAt.Before(now.Add(-7*24*time.Hour)) && !v.FirstSeenAt.After(now)
		sort.Slice(v.Accounts, func(i, j int) bool { return v.Accounts[i].ID < v.Accounts[j].ID })
		sort.Slice(v.Clients, func(i, j int) bool { return v.Clients[i].ID < v.Clients[j].ID })
		// Dominant region is independent of the search term, avoiding inverted tags
		// when an administrator searches for a less frequently observed location.
		if v.Location != "" {
			primary[v.Location] += v.Count
		}
		text := v.IP + " " + v.Location + " " + v.ISP
		for _, u := range v.Accounts {
			text += " " + u.Name
		}
		for _, d := range v.Clients {
			text += " " + d.Name + " " + d.Client
		}
		if !strings.Contains(strings.ToLower(text), strings.ToLower(strings.TrimSpace(f.Query))) {
			continue
		}
		matched = append(matched, v)
		for _, u := range v.Accounts {
			users[u.ID] = true
		}
		if v.Location != "" {
			regions[v.Location] = true
		}
		p, c, o := geography(l.Country, l.Province), v.Location, l.ISP
		if p == "" {
			p = "未知归属地"
		}
		if c == "" {
			c = "未知归属地"
		}
		if o == "" {
			o = "未知运营商"
		}
		province[p] += v.Count
		city[c] += v.Count
		operator[o] += v.Count
	}
	ranked := distribution(primary)
	if len(ranked) > 0 {
		out.Summary.PrimaryRegion = ranked[0].Name
	}
	for i := range matched {
		matched[i].DifferentRegion = f.UserID != "" && matched[i].Location != "" && out.Summary.PrimaryRegion != "" && matched[i].Location != out.Summary.PrimaryRegion
	}
	out.Total = len(matched)
	out.Summary.UniqueIPs = len(matched)
	out.Summary.Regions = len(regions)
	out.Summary.ActiveUsers = len(users)
	out.Summary.Provinces, out.Summary.Cities, out.Summary.Operators = distribution(province), distribution(city), distribution(operator)
	start := min(f.Start, len(matched))
	out.Items = matched[start:min(start+f.Limit, len(matched))]
	return out
}
