package playbackstats

import (
	"context"
	_ "embed"
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

// IPFactsSQL is the shared SELECT used by the report and operator read-only verification.
// IPs are recorded canonically by the original recorders. Trust/public validation
// is reused, never inferred from a matching session IP or a spoofable UA.
//
//go:embed ip_sources.sql
var ipSourcesSQL string

//go:embed ip_facts.sql
var ipAggregateSQL string

var IPFactsSQL = ipSourcesSQL + ipAggregateSQL

// IPReport merges retained playback sessions and verified allowed public 302s.
// The latter carry no user/device identity; never guess one from IP/UA proximity.
// Reading the original records includes retained historical traffic without writes.
func (r Repository) IPReport(ctx context.Context, f Filter, locate func(string) ipgeo.Location) (IPReport, error) {
	return r.ipReportAt(ctx, f, locate, time.Now())
}

func (r Repository) ipReportAt(ctx context.Context, f Filter, locate func(string) ipgeo.Location, now time.Time) (IPReport, error) {
	all, e := r.ipFactsAt(ctx, f, now)
	if e != nil {
		return IPReport{Page: Page[IPRow]{Items: []IPRow{}, StartIndex: f.Start, Limit: f.Limit}}, e
	}
	return buildIPReport(all, f, locate, now), nil
}

func ipWindowArgs(f Filter, now time.Time) []any {
	since := time.Unix(0, 0)
	if f.Days > 0 {
		since = now.AddDate(0, 0, -f.Days)
	}
	return []any{f.UserID, f.DeviceID, since, now}
}
func (r Repository) ipFactsAt(ctx context.Context, f Filter, now time.Time) ([]IPRow, error) {
	if e := f.Validate(); e != nil {
		return nil, e
	}
	rows, e := r.DB.Query(ctx, IPFactsSQL, ipWindowArgs(f, now)...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	all := []IPRow{}
	scanned := 0
	for rows.Next() {
		scanned++
		if scanned > 100000 {
			return nil, errors.New("IP report scope exceeds the bounded read limit")
		}
		var v IPRow
		var accounts, clients []byte
		if e = rows.Scan(&v.IP, &v.Users, &v.Devices, &v.Count, &v.FirstSeenAt, &v.LastSeenAt, &accounts, &clients, &v.SessionCount, &v.Direct302Count); e != nil {
			return nil, e
		}
		if !clientip.PublicString(v.IP) {
			continue
		}
		if e = json.Unmarshal(accounts, &v.Accounts); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(clients, &v.Clients); e != nil {
			return nil, e
		}
		all = append(all, v)
	}
	return all, rows.Err()
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
		province[p]++
		city[c]++
		operator[o]++
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
