package qbittorrent

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
)

type Assistance struct {
	Tag, Category              string
	DownloadLimit, UploadLimit *int64
	RatioLimit                 *float64
	SeedingMinutes             *int64
	InactiveMinutes            int64
}

// ApplyAssistance contains fixed endpoints only. Compound partial failures are
// unknown; callers must not replay or claim rollback of previous remote effects.
func (a *Adapter) ApplyAssistance(ctx context.Context, hash string, v Assistance, admit func() error) error {
	post := func(endpoint, key, value string) error {
		if e := admit(); e != nil {
			return e
		}
		_, e := a.request(ctx, "POST", "torrents/"+endpoint, "application/x-www-form-urlencoded", []byte(url.Values{"hashes": {hash}, key: {value}}.Encode()))
		return e
	}
	if v.Tag != "" {
		if e := post("addTags", "tags", v.Tag); e != nil {
			return e
		}
	}
	if v.Category != "" {
		if e := post("setCategory", "category", v.Category); e != nil {
			return e
		}
	}
	for _, p := range []struct {
		endpoint string
		n        *int64
	}{{"setDownloadLimit", v.DownloadLimit}, {"setUploadLimit", v.UploadLimit}} {
		if p.n != nil {
			if e := post(p.endpoint, "limit", strconv.FormatInt(*p.n, 10)); e != nil {
				return e
			}
		}
	}
	if v.RatioLimit != nil || v.SeedingMinutes != nil {
		ratio, minutes := "-2", "-2"
		if v.RatioLimit != nil {
			ratio = strconv.FormatFloat(*v.RatioLimit, 'f', -1, 64)
		}
		if v.SeedingMinutes != nil {
			minutes = strconv.FormatInt(*v.SeedingMinutes, 10)
		}
		if e := admit(); e != nil {
			return e
		}
		form := url.Values{"hashes": {hash}, "ratioLimit": {ratio}, "seedingTimeLimit": {minutes}, "inactiveSeedingTimeLimit": {strconv.FormatInt(v.InactiveMinutes, 10)}}
		_, e := a.request(ctx, "POST", "torrents/setShareLimits", "application/x-www-form-urlencoded", []byte(form.Encode()))
		if e != nil {
			return e
		}
	}
	return nil
}
func (a *Adapter) RecheckOwned(ctx context.Context, hash string) error {
	_, e := a.request(ctx, "POST", "torrents/recheck", "application/x-www-form-urlencoded", []byte(url.Values{"hashes": {hash}}.Encode()))
	return e
}
func (a *Adapter) CategorySafe(ctx context.Context, category, save string) bool {
	raw, e := a.request(ctx, "GET", "torrents/categories", "", nil)
	if e != nil {
		return false
	}
	var rows map[string]struct {
		SavePath string `json:"savePath"`
	}
	if json.Unmarshal(raw, &rows) != nil {
		return false
	}
	v, ok := rows[category]
	return ok && (v.SavePath == "" || strings.TrimRight(v.SavePath, "/") == strings.TrimRight(save, "/"))
}
