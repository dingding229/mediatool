// Package plugins exposes a versioned capability/event contract. Extensions are
// declarative HTTPS webhooks, not downloaded Go binaries or shell commands.
package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/playback"
	"media-tool/internal/modules/releasename"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var ErrInvalid = errors.New("invalid extension manifest")
var identifier = regexp.MustCompile(`^[a-z][a-z0-9-]{2,63}$`)

type Manifest struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Contract     int      `json:"contractVersion"`
	Endpoint     string   `json:"endpoint"`
	Capabilities []string `json:"capabilities"`
}
type Record struct {
	Manifest Manifest `json:"manifest"`
	Enabled  bool     `json:"enabled"`
	Builtin  bool     `json:"builtin"`
	Revision int64    `json:"revision"`
}
type Event struct {
	Version int            `json:"contractVersion"`
	Kind    string         `json:"kind"`
	Data    map[string]any `json:"data"`
}
type Extension interface {
	Manifest() Manifest
	Handle(context.Context, Event) (json.RawMessage, error)
}
type Registry struct {
	DB   *pgxpool.Pool
	HTTP *http.Client
}

func builtins() []Record {
	return []Record{{Manifest: Manifest{ID: "release-name", Name: "0day 文件名识别", Version: "1.0.0", Contract: 1, Capabilities: []string{"media.recognize"}}, Enabled: true, Builtin: true, Revision: 1}, {Manifest: Manifest{ID: "naming-template", Name: "电影与剧集命名模板", Version: "1.0.0", Contract: 1, Capabilities: []string{"media.naming"}}, Enabled: true, Builtin: true, Revision: 1}}
}
func Validate(m Manifest) error {
	if !identifier.MatchString(m.ID) || m.ID == "release-name" || m.ID == "naming-template" || m.Contract != 1 || len(m.Name) < 1 || len(m.Name) > 120 || len(m.Version) < 1 || len(m.Version) > 40 || len(m.Capabilities) == 0 || len(m.Capabilities) > 2 {
		return ErrInvalid
	}
	u, err := url.Parse(m.Endpoint)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return ErrInvalid
	}
	if _, err = (playback.Policy{}).Syntax(m.Endpoint); err != nil {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, c := range m.Capabilities {
		if seen[c] || c != "media.recognize" && c != "media.organized" {
			return ErrInvalid
		}
		seen[c] = true
	}
	return nil
}
func (r Registry) List(ctx context.Context) ([]Record, error) {
	out := builtins()
	if r.DB == nil {
		return out, nil
	}
	rows, err := r.DB.Query(ctx, `SELECT manifest,enabled,revision FROM extension_plugins ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		var item Record
		if err = rows.Scan(&raw, &item.Enabled, &item.Revision); err != nil {
			return nil, err
		}
		if json.Unmarshal(raw, &item.Manifest) != nil {
			return nil, ErrInvalid
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
func (r Registry) Save(ctx context.Context, m Manifest, enabled, consent bool, revision int64) error {
	if Validate(m) != nil || enabled && !consent || r.DB == nil {
		return ErrInvalid
	}
	raw, _ := json.Marshal(m)
	if revision == 0 {
		if enabled {
			return ErrInvalid
		}
		_, err := r.DB.Exec(ctx, `INSERT INTO extension_plugins(id,name,version,manifest)VALUES($1,$2,$3,$4)`, m.ID, m.Name, m.Version, raw)
		return err
	}
	tag, err := r.DB.Exec(ctx, `UPDATE extension_plugins SET name=$2,version=$3,manifest=$4,enabled=$5,revision=revision+1,updated_at=now() WHERE id=$1 AND revision=$6`, m.ID, m.Name, m.Version, raw, enabled, revision)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrInvalid
	}
	return nil
}
func (r Registry) Delete(ctx context.Context, id string) error {
	if !identifier.MatchString(id) || id == "release-name" || id == "naming-template" {
		return ErrInvalid
	}
	_, err := r.DB.Exec(ctx, `DELETE FROM extension_plugins WHERE id=$1`, id)
	return err
}
func (r Registry) invoke(ctx context.Context, m Manifest, event Event) (json.RawMessage, error) {
	if Validate(m) != nil {
		return nil, ErrInvalid
	}
	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	policy := playback.Policy{}
	if err := policy.Check(bounded, m.Endpoint); err != nil {
		return nil, ErrInvalid
	}
	payload, err := json.Marshal(event)
	if err != nil || len(payload) > 32<<10 {
		return nil, ErrInvalid
	}
	req, err := http.NewRequestWithContext(bounded, "POST", m.Endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, ErrInvalid
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "MediaTool-Extension/1")
	client := r.HTTP
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{Proxy: nil, DialContext: policy.DialContext, TLSHandshakeTimeout: 2 * time.Second}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, errors.New("extension request unavailable")
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, (64<<10)+1))
	if err != nil || len(raw) > 64<<10 || res.StatusCode != 200 {
		return nil, errors.New("extension response invalid")
	}
	if len(raw) > 0 && !json.Valid(raw) {
		return nil, ErrInvalid
	}
	return raw, nil
}
func (r Registry) Recognize(ctx context.Context, name string) (releasename.Hint, []string) {
	hint := releasename.Parse(name)
	messages := []string{}
	items, err := r.List(ctx)
	if err != nil {
		return hint, []string{"扩展配置读取失败，使用内置解析"}
	}
	calls := 0
	for _, item := range items {
		if item.Builtin || !item.Enabled || !contains(item.Manifest.Capabilities, "media.recognize") {
			continue
		}
		calls++
		if calls > 3 {
			messages = append(messages, "仅执行前三个已启用识别扩展")
			break
		}
		raw, err := r.invoke(ctx, item.Manifest, Event{Version: 1, Kind: "media.recognize", Data: map[string]any{"filename": name, "hint": hint}})
		if err != nil {
			messages = append(messages, item.Manifest.Name+" 不可用，保留内置结果")
			continue
		}
		var out struct {
			Title     string `json:"title"`
			Year      string `json:"year"`
			MediaType string `json:"mediaType"`
		}
		if json.Unmarshal(raw, &out) != nil {
			continue
		}
		if len(out.Title) > 0 && len(out.Title) <= 200 && !strings.ContainsAny(out.Title, "/\\\r\n\x00") {
			hint.Title = out.Title
		}
		if regexp.MustCompile(`^(19|20)\d{2}$`).MatchString(out.Year) {
			hint.Year = out.Year
		}
		if out.MediaType == "movie" || out.MediaType == "series" {
			hint.MediaType = out.MediaType
		}
		hint.Confidence = "extension-hint"
		messages = append(messages, "扩展结果仍需核对真实媒体身份")
	}
	return hint, messages
}
func (r Registry) Organized(ctx context.Context, event Event) {
	items, err := r.List(ctx)
	if err != nil {
		return
	}
	count := 0
	for _, item := range items {
		if !item.Enabled || item.Builtin || !contains(item.Manifest.Capabilities, "media.organized") {
			continue
		}
		count++
		if count > 3 {
			return
		}
		eventID, _ := event.Data["eventId"].(string)
		tag, err := r.DB.Exec(ctx, `INSERT INTO extension_deliveries(event_id,plugin_id,state)VALUES($1,$2,'pending')ON CONFLICT DO NOTHING`, eventID, item.Manifest.ID)
		if err != nil || tag.RowsAffected() != 1 {
			continue
		}
		_, err = r.invoke(ctx, item.Manifest, event)
		state, message := "succeeded", "扩展事件已得到有效确认"
		if err != nil {
			state, message = "unknown", "扩展事件未确认成功；不自动重复发送"
		}
		finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		r.DB.Exec(finish, `UPDATE extension_deliveries SET state=$3,message=$4,updated_at=now() WHERE event_id=$1 AND plugin_id=$2`, eventID, item.Manifest.ID, state, message)
		cancel()
	}
}
func contains(xs []string, x string) bool {
	for _, s := range xs {
		if s == x {
			return true
		}
	}
	return false
}

func (r Registry) Process(ctx context.Context, job jobs.Job) error {
	var in struct {
		ID string `json:"operationId"`
	}
	if json.Unmarshal(job.Payload, &in) != nil || len(in.ID) != 36 {
		return ErrInvalid
	}
	var operation, state string
	var raw []byte
	if err := r.DB.QueryRow(ctx, `SELECT operation,state,plan->'items' FROM file_operations WHERE id=$1`, in.ID).Scan(&operation, &state, &raw); err != nil {
		return err
	}
	if state != "succeeded" {
		return nil
	}
	var items []struct {
		Source struct {
			Path string `json:"path"`
		} `json:"source"`
		Target string `json:"target"`
	}
	if json.Unmarshal(raw, &items) != nil {
		return ErrInvalid
	}
	paths := []map[string]string{}
	for _, item := range items {
		if len(paths) == 64 {
			break
		}
		paths = append(paths, map[string]string{"source": item.Source.Path, "target": item.Target})
	}
	r.Organized(ctx, Event{Version: 1, Kind: "media.organized", Data: map[string]any{"eventId": in.ID, "operation": operation, "state": state, "items": paths}})
	return nil // External notification is best-effort; do not retry ambiguous delivery.
}
