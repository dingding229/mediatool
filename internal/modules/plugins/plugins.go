// Package plugins runs installed WASI code in a capability-limited WASM sandbox.
// Filename recognition and naming templates are core application features.
package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/releasename"
	"media-tool/internal/platform/secrets"
	"regexp"
	"strings"
	"time"
)

var ErrInvalid = errors.New("invalid extension manifest")
var identifier = regexp.MustCompile(`^[a-z][a-z0-9-]{2,63}$`)

type Manifest struct {
	SourcePluginID string `json:"sourcePluginId,omitempty"`

	CustomUI bool `json:"customUI,omitempty"`

	Events      []string `json:"events,omitempty"`
	Permissions []string `json:"permissions,omitempty"`

	Author      string        `json:"author,omitempty"`
	Description string        `json:"description,omitempty"`
	Commands    []Command     `json:"commands,omitempty"`
	Services    []Service     `json:"services,omitempty"`
	Routes      []PluginRoute `json:"routes,omitempty"`
	Pages       []PluginPage  `json:"pages,omitempty"`
	Dashboard   bool          `json:"dashboard,omitempty"`
	Outbound    []string      `json:"outbound,omitempty"`

	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Version      string         `json:"version"`
	Contract     int            `json:"contractVersion"`
	Capabilities []string       `json:"capabilities"`
	Runtime      string         `json:"runtime,omitempty"`
	Settings     []SettingField `json:"settings,omitempty"`
}
type Record struct {
	Manifest      Manifest `json:"manifest"`
	Enabled       bool     `json:"enabled"`
	Revision      int64    `json:"revision"`
	PackageDigest string   `json:"packageDigest,omitempty"`
}
type Event struct {
	Version  int               `json:"contractVersion"`
	Kind     string            `json:"kind"`
	Data     map[string]any    `json:"data"`
	Settings map[string]string `json:"settings,omitempty"`
}
type Extension interface {
	Manifest() Manifest
	Handle(context.Context, Event) (json.RawMessage, error)
}
type Registry struct {
	Host HostFunc

	DB    *pgxpool.Pool
	Vault *secrets.Vault
}

func Validate(m Manifest) error {
	if validateFramework(m) != nil {
		return ErrInvalid
	}
	if !identifier.MatchString(m.ID) || m.ID == "release-name" || m.ID == "naming-template" || m.Contract != 1 || len(m.Name) < 1 || len(m.Name) > 120 || len(m.Version) < 1 || len(m.Version) > 40 || len(m.Capabilities) == 0 || len(m.Capabilities) > 10 {
		return ErrInvalid
	}
	if validateFields(m.Settings) != nil {
		return ErrInvalid
	}
	if m.Runtime != "wasm" {
		return ErrInvalid
	}
	return nil
}

func (r Registry) List(ctx context.Context) ([]Record, error) {
	out := []Record{}
	if r.DB == nil {
		return out, nil
	}
	rows, err := r.DB.Query(ctx, `SELECT manifest,enabled,revision,package_digest FROM extension_plugins ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		var item Record
		if err = rows.Scan(&raw, &item.Enabled, &item.Revision, &item.PackageDigest); err != nil {
			return nil, err
		}
		if strictJSON(raw, &item.Manifest) != nil || Validate(item.Manifest) != nil {
			return nil, ErrInvalid
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
func (r Registry) Save(ctx context.Context, m Manifest, enabled, consent bool, revision int64) error {
	if Validate(m) != nil || enabled && !consent || r.DB == nil || revision < 1 {
		return ErrInvalid
	}
	if revision > 0 {
		var raw []byte
		var current int64
		if e := r.DB.QueryRow(ctx, `SELECT manifest,revision FROM extension_plugins WHERE id=$1 AND octet_length(wasm_module)>=8`, m.ID).Scan(&raw, &current); e != nil || current != revision {
			return ErrInvalid
		}
		var stored Manifest
		if json.Unmarshal(raw, &stored) != nil {
			return ErrInvalid
		}
		left, _ := json.Marshal(stored)
		right, _ := json.Marshal(m)
		if !bytes.Equal(left, right) {
			return ErrInvalid
		}
	}
	if enabled {
		values, _, rev, e := r.Settings(ctx, m.ID)
		if e != nil || rev != revision {
			return ErrInvalid
		}
		if m.Runtime == "wasm" {
			for _, service := range m.Services {
				if _, err := nextService(service, values, time.Now()); err != nil {
					return err
				}
			}
		}
		if _, e = ValidateSettings(m.Settings, values, true); e != nil {
			return e
		}
	}
	raw, _ := json.Marshal(m)

	tag, err := r.DB.Exec(ctx, `UPDATE extension_plugins SET name=$2,version=$3,manifest=$4,enabled=$5,revision=revision+1,updated_at=now() WHERE id=$1 AND revision=$6`, m.ID, m.Name, m.Version, raw, enabled, revision)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrInvalid
	}
	if m.Runtime == "wasm" {
		kind := "plugin.stop"
		if enabled {
			kind = "plugin.init"
		}
		if _, e := r.Invoke(ctx, m.ID, kind, map[string]any{}, !enabled); e != nil && enabled {
			r.DB.Exec(ctx, `UPDATE extension_plugins SET enabled=false,revision=revision+1 WHERE id=$1 AND revision=$2`, m.ID, revision+1)
			return e
		}
	}
	return nil
}
func (r Registry) Delete(ctx context.Context, id string) error {
	if r.DB == nil || !identifier.MatchString(id) || id == "release-name" || id == "naming-template" {
		return ErrInvalid
	}
	_, err := r.DB.Exec(ctx, `DELETE FROM extension_plugins WHERE id=$1`, id)
	return err
}
func (r Registry) invoke(ctx context.Context, m Manifest, event Event) (json.RawMessage, error) {
	if Validate(m) != nil {
		return nil, ErrInvalid
	}
	return r.invokeFramework(ctx, m, event, false)
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
		if !item.Enabled || !contains(item.Manifest.Capabilities, "media.recognize") {
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
		if !item.Enabled || !contains(item.Manifest.Capabilities, "media.organized") {
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
