package integrations

import (
	"encoding/json"
	"errors"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"media-tool/internal/modules/sites"
)

var ErrNotFound = errors.New("integration not found")
var ErrConflict = errors.New("another enabled integration exists for this kind")
var ErrRevision = errors.New("integration changed; reload before saving")
var ErrProbeRecord = errors.New("configuration saved but validation receipt unavailable; refresh before retry")
var ErrUnavailable = errors.New("integration storage unavailable")
var ErrInvalid = errors.New("invalid integration settings")

type Record struct {
	Readiness        Readiness      `json:"readiness"`
	ID               string         `json:"id"`
	Kind             string         `json:"kind"`
	Name             string         `json:"name"`
	Enabled          bool           `json:"enabled"`
	Settings         map[string]any `json:"settings"`
	SecretFields     []string       `json:"secretFields"`
	Revision         int64          `json:"revision"`
	LastCheckState   string         `json:"lastCheckState"`
	LastCheckMessage string         `json:"lastCheckMessage"`
	LastCheckedAt    *time.Time     `json:"lastCheckedAt,omitempty"`
	UpdatedAt        time.Time      `json:"updatedAt"`
}
type Input struct {
	Kind     string            `json:"kind"`
	Name     string            `json:"name"`
	Enabled  bool              `json:"enabled"`
	Settings map[string]any    `json:"settings"`
	Secrets  map[string]string `json:"secrets,omitempty"`
	Revision int64             `json:"revision,omitempty"`
}
type Credentials struct {
	Record Record
	Values map[string]string
}

func (c Credentials) String(key string) string { v, _ := c.Record.Settings[key].(string); return v }

var publicKeys = map[string][]string{
	"clouddrive2": {"endpoint", "cloudRoot", "caFile"},
	"open115":     {"appId", "cloudMountPrefix"},
	"local":       {"rootPath"},
	"qbittorrent": {"endpoint", "downloadPath", "localPath", "category"},
	"tmdb":        {"language"},
	"telegram":    {"chatId", "inboundEnabled", "allowedChatIds", "allowedUserIds"},
	"ptsite":      {"definitionId", "endpoint", "userId", "userAgent", "rssState", "rssSource"},
}
var secretKeys = map[string][]string{
	"local": {}, "open115": {"accessToken", "refreshToken"}, "clouddrive2": {"token", "username", "password"}, "qbittorrent": {"username", "password"}, "tmdb": {"token"}, "telegram": {"botToken"}, "ptsite": {"cookie", "apiKey", "passkey", "rssUrl", "requestHeaders"},
}

func Validate(in Input) error {
	keys, ok := publicKeys[in.Kind]
	if !ok || len(strings.TrimSpace(in.Name)) == 0 || len(in.Name) > 120 {
		return ErrInvalid
	}
	for k, v := range in.Settings {
		if !contains(keys, k) {
			return errors.New("unsupported public setting: " + k)
		}
		if k == "directPlayOnly" || k == "inboundEnabled" {
			if _, ok := v.(bool); !ok {
				return ErrInvalid
			}
		}
		switch typed := v.(type) {
		case string:
			if len(typed) > 4096 || strings.ContainsAny(typed, "\r\n\x00") {
				return ErrInvalid
			}
		case bool:
			if k != "gatewayEnabled" && k != "directPlayOnly" && k != "inboundEnabled" {
				return ErrInvalid
			}
		default:
			return ErrInvalid
		}
	}
	if in.Kind == "telegram" {
		for _, key := range []string{"allowedChatIds", "allowedUserIds"} {
			s, _ := in.Settings[key].(string)
			if strings.TrimSpace(s) == "" {
				continue
			}
			ids := strings.Split(s, ",")
			if len(ids) > 200 {
				return ErrInvalid
			}
			for _, part := range ids {
				part = strings.TrimSpace(part)
				id, e := strconv.ParseInt(part, 10, 64)
				if e != nil || id == 0 || strconv.FormatInt(id, 10) != part || key == "allowedUserIds" && id < 0 {
					return ErrInvalid
				}
			}
		}
	}
	for k, v := range in.Secrets {
		if !contains(secretKeys[in.Kind], k) || len(v) > 16384 || strings.ContainsRune(v, '\x00') {
			return ErrInvalid
		}
		if in.Kind == "ptsite" && k == "requestHeaders" {
			if _, err := sites.ParseAccountHeaders(v); err != nil {
				return ErrInvalid
			}
		}
	}
	endpoint, _ := in.Settings["endpoint"].(string)
	if endpoint != "" {
		raw := endpoint
		if !strings.Contains(raw, "://") && in.Kind == "clouddrive2" {
			raw = "http://" + raw
		}
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("endpoint must be an HTTP(S) URL without embedded credentials")
		}
	}
	if in.Enabled && in.Kind != "tmdb" && in.Kind != "telegram" && in.Kind != "open115" && in.Kind != "local" && endpoint == "" {
		return errors.New("enabled integration requires an endpoint")
	}
	for _, k := range []string{"downloadPath", "localPath"} {
		if p, ok := in.Settings[k].(string); ok && p != "" && !filepath.IsAbs(p) {
			return errors.New(k + " must be absolute")
		}
	}
	if p, ok := in.Settings["cloudRoot"].(string); ok && p != "" && (!strings.HasPrefix(p, "/") || filepath.Clean(p) != p || strings.Contains(p, "\\") || strings.Contains("/"+p+"/", "/.media-tool-trash/")) {
		return errors.New("cloudRoot must be absolute")
	}
	if in.Kind == "local" {
		root, _ := in.Settings["rootPath"].(string)
		if root == "" || !filepath.IsAbs(root) || filepath.Clean(root) != root || root == "/" || strings.Contains(root, "\\") {
			return errors.New("local root must be an explicit absolute directory, not the filesystem root")
		}
	}
	if in.Kind == "tmdb" {
		if lang, ok := in.Settings["language"].(string); ok && lang != "" && len(lang) > 12 {
			return ErrInvalid
		}
	}
	if in.Kind == "open115" {
		app, _ := in.Settings["appId"].(string)
		if app != "" && (len(app) > 40 || strings.Trim(app, "0123456789") != "") {
			return errors.New("115 AppID must be your own numeric application ID")
		}
		prefix, _ := in.Settings["cloudMountPrefix"].(string)
		if prefix == "" || !strings.HasPrefix(prefix, "/") || filepath.Clean(prefix) != prefix || prefix == "/" {
			return errors.New("115 cloud mount prefix must be an absolute normalized prefix such as /115")
		}
	}
	if _, err := json.Marshal(in.Settings); err != nil {
		return ErrInvalid
	}
	return nil
}
func contains(list []string, k string) bool {
	for _, s := range list {
		if s == k {
			return true
		}
	}
	return false
}
