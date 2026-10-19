package qbittorrent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	deferutil "media-tool/internal/platform/cleanup"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"

	"media-tool/internal/modules/downloads"
)

var ErrUnavailable = errors.New("qBittorrent request failed; check credentials and connectivity")
var ErrNotFound = errors.New("torrent not found in qBittorrent")

type Adapter struct {
	Endpoint, Username, Password string
	mu                           sync.Mutex
	client                       *http.Client
}

var _ downloads.Downloader = (*Adapter)(nil)

func (a *Adapter) init() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.client == nil {
		jar, _ := cookiejar.New(nil)
		a.client = &http.Client{Jar: jar, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
}
func (a *Adapter) login(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	body := url.Values{"username": {a.Username}, "password": {a.Password}}
	r, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(a.Endpoint, "/")+"/api/v2/auth/login", strings.NewReader(body.Encode()))
	if err != nil {
		return ErrUnavailable
	}
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Referer", strings.TrimRight(a.Endpoint, "/")+"/")
	r.Header.Set("Origin", strings.TrimRight(a.Endpoint, "/"))
	response, err := a.client.Do(r)
	if err != nil {
		return ErrUnavailable
	}
	defer deferutil.Close(response.Body, "Read/descriptor teardown only; primary operations handle their own I/O result")
	raw, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
	if response.StatusCode != http.StatusNoContent && (response.StatusCode != 200 || strings.TrimSpace(string(raw)) != "Ok.") {
		return ErrUnavailable
	}
	authenticated := false
	if a.client.Jar != nil {
		for _, cookie := range a.client.Jar.Cookies(r.URL) {
			if (cookie.Name == "SID" || strings.HasPrefix(cookie.Name, "QBT_SID_")) && cookie.Value != "" {
				authenticated = true
			}
		}
	}
	if !authenticated {
		return ErrUnavailable
	}
	return nil
}
func (a *Adapter) request(ctx context.Context, method, endpoint, contentType string, body []byte) ([]byte, error) {
	a.init()
	for attempt := 0; attempt < 2; attempt++ {
		r, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(a.Endpoint, "/")+"/api/v2/"+endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, ErrUnavailable
		}
		if contentType != "" {
			r.Header.Set("Content-Type", contentType)
		}
		r.Header.Set("Referer", strings.TrimRight(a.Endpoint, "/")+"/")
		r.Header.Set("Origin", strings.TrimRight(a.Endpoint, "/"))
		response, err := a.client.Do(r)
		if err != nil {
			return nil, ErrUnavailable
		}
		raw, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
		deferutil.Close(response.Body, "Read/descriptor teardown only; primary operations handle their own I/O result")
		if (response.StatusCode == 403 || response.StatusCode == 401) && attempt == 0 {
			if err = a.login(ctx); err != nil {
				return nil, err
			}
			continue
		}
		if err != nil || (response.StatusCode != 200 && response.StatusCode != http.StatusNoContent) {
			return nil, ErrUnavailable
		}
		return raw, nil
	}
	return nil, ErrUnavailable
}
func (a *Adapter) Probe(ctx context.Context) (string, error) {
	raw, err := a.request(ctx, "GET", "app/version", "", nil)
	if err != nil {
		return "", err
	}
	v := strings.TrimSpace(string(raw))
	if !strings.HasPrefix(v, "v4.") && !strings.HasPrefix(v, "v5.") {
		return "", errors.New("unsupported qBittorrent version")
	}
	return v, nil
}
func (a *Adapter) Add(ctx context.Context, in downloads.Request) (string, error) {
	meta, err := downloads.ParseTorrent(in.Torrent)
	if err != nil {
		return "", err
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("torrents", "media-tool.torrent")
	if err != nil {
		return "", ErrUnavailable
	}
	if _, err = part.Write(in.Torrent); err != nil {
		return "", ErrUnavailable
	}
	if err = writer.WriteField("savepath", in.SavePath); err != nil {
		return "", ErrUnavailable
	}
	if err = writer.WriteField("category", in.Category); err != nil {
		return "", ErrUnavailable
	}
	if len(in.Tags) > 0 {
		if err = writer.WriteField("tags", strings.Join(in.Tags, ",")); err != nil {
			return "", ErrUnavailable
		}
	}
	if err = writer.Close(); err != nil {
		return "", ErrUnavailable
	}
	raw, err := a.request(ctx, "POST", "torrents/add", writer.FormDataContentType(), body.Bytes())
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(string(raw)) != "Ok." {
		var result struct {
			Success int      `json:"success_count"`
			Failure int      `json:"failure_count"`
			IDs     []string `json:"added_torrent_ids"`
		}
		if json.Unmarshal(raw, &result) != nil || result.Success != 1 || result.Failure != 0 || len(result.IDs) != 1 || !strings.EqualFold(result.IDs[0], meta.InfoHash) {
			return "", ErrUnavailable
		}
	}
	return meta.InfoHash, nil
}
func (a *Adapter) Inspect(ctx context.Context, id string) (downloads.Task, error) {
	raw, err := a.request(ctx, "GET", "torrents/info?"+url.Values{"hashes": {id}}.Encode(), "", nil)
	if err != nil {
		return downloads.Task{}, err
	}
	var items []struct {
		DownloadLimit               *int64   `json:"dl_limit"`
		UploadLimit                 *int64   `json:"up_limit"`
		RatioLimit                  *float64 `json:"ratio_limit"`
		EffectiveRatioLimit         *float64 `json:"max_ratio"`
		EffectiveSeedingMinutes     *int64   `json:"max_seeding_time"`
		SeedingMinutes              *int64   `json:"seeding_time_limit"`
		InactiveMinutes             *int64   `json:"inactive_seeding_time_limit"`
		AutoManaged                 *bool    `json:"auto_tmm"`
		Hash, Name, State, Category string
		Progress                    *float64
		DownloadSpeed               int64  `json:"dlspeed"`
		UploadSpeed                 int64  `json:"upspeed"`
		Seeds                       int    `json:"num_seeds"`
		Peers                       int    `json:"num_leechs"`
		SavePath                    string `json:"save_path"`
		ContentPath                 string `json:"content_path"`
		AmountLeft                  int64  `json:"amount_left"`
		CompletionOn                int64  `json:"completion_on"`
		Size                        int64
		Ratio                       float64
		SeedingTime                 int64 `json:"seeding_time"`
	}
	if json.Unmarshal(raw, &items) != nil {
		return downloads.Task{}, ErrUnavailable
	}
	if len(items) == 0 {
		return downloads.Task{}, ErrNotFound
	}
	match := -1
	for index, candidate := range items {
		if strings.EqualFold(candidate.Hash, id) {
			if match != -1 {
				return downloads.Task{}, ErrUnavailable
			}
			match = index
		}
	}
	if match < 0 {
		return downloads.Task{}, ErrUnavailable
	}
	it := items[match]
	if it.Progress == nil || math.IsNaN(*it.Progress) || *it.Progress < 0 || *it.Progress > 1 || it.State == "" {
		return downloads.Task{}, ErrUnavailable
	}
	return downloads.Task{DownloadLimit: it.DownloadLimit, UploadLimit: it.UploadLimit, RatioLimit: it.RatioLimit, EffectiveRatioLimit: it.EffectiveRatioLimit, EffectiveSeedingMinutes: it.EffectiveSeedingMinutes, SeedingMinutes: it.SeedingMinutes, InactiveMinutes: it.InactiveMinutes, AutoManaged: it.AutoManaged, ID: it.Hash, State: it.State, Name: it.Name, Category: it.Category, Progress: *it.Progress, DownloadSpeed: it.DownloadSpeed, UploadSpeed: it.UploadSpeed, Seeds: it.Seeds, Peers: it.Peers, SavePath: it.SavePath, ContentPath: it.ContentPath, Size: it.Size, Ratio: it.Ratio, SeedingTime: it.SeedingTime, Complete: *it.Progress >= 1 && it.AmountLeft == 0 && it.CompletionOn > 0, Seeding: it.State == "stalledUP" || it.State == "forcedUP" || it.State == "uploading"}, nil
}
func (a *Adapter) Files(ctx context.Context, id string) ([]downloads.File, error) {
	raw, err := a.request(ctx, "GET", "torrents/files?"+url.Values{"hash": {id}}.Encode(), "", nil)
	if err != nil {
		return nil, err
	}
	var files []downloads.File
	if json.Unmarshal(raw, &files) != nil {
		return nil, ErrUnavailable
	}
	return files, nil
}
func (a *Adapter) SetPaused(ctx context.Context, id string, paused bool) error {
	version, err := a.Probe(ctx)
	if err != nil {
		return err
	}
	action := "resume"
	if paused {
		action = "pause"
	}
	if strings.HasPrefix(version, "v5.") {
		action = "start"
		if paused {
			action = "stop"
		}
	}
	_, err = a.request(ctx, "POST", "torrents/"+action, "application/x-www-form-urlencoded", []byte(url.Values{"hashes": {id}}.Encode()))
	return err
}

func (a *Adapter) InspectAliases(ctx context.Context, id string, aliases []string) (downloads.Task, error) {
	seen := map[string]bool{}
	for _, candidate := range append([]string{id}, aliases...) {
		if seen[candidate] || candidate == "" {
			continue
		}
		seen[candidate] = true
		task, err := a.Inspect(ctx, candidate)
		if err == nil {
			return task, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return downloads.Task{}, err
		}
	}
	return downloads.Task{}, ErrNotFound
}

func (a *Adapter) Trackers(ctx context.Context, id string) ([]downloads.Tracker, error) {
	raw, err := a.request(ctx, "GET", "torrents/trackers?"+url.Values{"hash": {id}}.Encode(), "", nil)
	if err != nil {
		return nil, err
	}
	var values []struct {
		URL     string `json:"url"`
		Status  int    `json:"status"`
		Message string `json:"msg"`
		Peers   int    `json:"num_peers"`
		Seeds   int    `json:"num_seeds"`
		Leeches int    `json:"num_leeches"`
	}
	if json.Unmarshal(raw, &values) != nil || len(values) > 200 {
		return nil, ErrUnavailable
	}
	out := []downloads.Tracker{}
	for _, v := range values {
		out = append(out, downloads.Tracker{URL: v.URL, Status: v.Status, Message: v.Message, Peers: v.Peers, Seeds: v.Seeds, Leeches: v.Leeches})
	}
	return out, nil
}

func (a *Adapter) DeleteOwned(ctx context.Context, hash string, deleteFiles bool) error {
	body := url.Values{"hashes": {hash}, "deleteFiles": {fmt.Sprint(deleteFiles)}}
	_, err := a.request(ctx, "POST", "torrents/delete", "application/x-www-form-urlencoded", []byte(body.Encode()))
	return err
}
func (a *Adapter) CleanupInfo(ctx context.Context, hash string) (string, string, error) {
	raw, err := a.request(ctx, "GET", "torrents/info?"+url.Values{"hashes": {hash}}.Encode(), "", nil)
	if err != nil {
		return "", "", err
	}
	var rows []struct {
		Hash     string
		Tags     string
		Category string
		SavePath string `json:"save_path"`
	}
	if json.Unmarshal(raw, &rows) != nil || len(rows) != 1 || !strings.EqualFold(rows[0].Hash, hash) {
		return "", "", ErrUnavailable
	}
	return rows[0].Tags, rows[0].SavePath, nil
}
func (a *Adapter) AllHashes(ctx context.Context) ([]string, error) {
	raw, err := a.request(ctx, "GET", "torrents/info", "", nil)
	if err != nil {
		return nil, err
	}
	var rows []struct{ Hash string }
	if json.Unmarshal(raw, &rows) != nil || len(rows) > 1000 {
		return nil, ErrUnavailable
	}
	out := []string{}
	for _, r := range rows {
		out = append(out, r.Hash)
	}
	return out, nil
}

func (a *Adapter) AddMagnet(ctx context.Context, magnet, hash, save, category string, tags []string) (string, error) {
	u, err := url.Parse(magnet)
	if err != nil || u.Scheme != "magnet" || len(magnet) > 8192 || len(hash) != 40 && len(hash) != 64 {
		return "", ErrUnavailable
	}
	values := url.Values{"urls": {magnet}, "savepath": {save}, "category": {category}, "tags": {strings.Join(tags, ",")}}
	raw, err := a.request(ctx, "POST", "torrents/add", "application/x-www-form-urlencoded", []byte(values.Encode()))
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(string(raw)) != "Ok." {
		var response struct {
			Success int `json:"success_count"`
			Failure int `json:"failure_count"`
		}
		if json.Unmarshal(raw, &response) != nil || response.Success != 1 || response.Failure != 0 {
			return "", ErrUnavailable
		}
	}
	return hash, nil
}
