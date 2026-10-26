package settings

import (
	"context"
	"errors"
	"net"
	"net/url"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Access struct {
	ManagementURL string `json:"managementUrl"`
	PlaybackURL   string `json:"playbackUrl"`
	MemberURL     string `json:"memberUrl"`
}

var domainLabel = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

// OriginKey matches browser normalization for explicit default ports.
func OriginKey(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if u.Scheme == "https" && u.Port() == "443" || u.Scheme == "http" && u.Port() == "80" {
		return u.Scheme + "://" + u.Hostname()
	}
	return raw
}
func ValidateAccess(v Access) error {
	if v.MemberURL != "" && (OriginKey(v.MemberURL) == OriginKey(v.ManagementURL) || OriginKey(v.MemberURL) == OriginKey(v.PlaybackURL)) {
		return errors.New("内置媒体入口须使用不同域名或端口，根路径不能与后台共用")
	}

	for _, raw := range []string{v.ManagementURL, v.PlaybackURL, v.MemberURL} {
		if raw == "" {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil || len(raw) > 512 || raw != strings.TrimSpace(raw) || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || u.Path != "" || u.RawPath != "" || raw != u.Scheme+"://"+u.Host {
			return errors.New("域名地址须为完整HTTP(S)来源，不含路径、账号、查询或片段")
		}
		host := u.Hostname()
		if host != strings.ToLower(host) || net.ParseIP(host) != nil || !strings.Contains(host, ".") || strings.HasSuffix(host, ".") {
			return errors.New("请填写规范域名，不使用IP或localhost")
		}
		for _, label := range strings.Split(host, ".") {
			if !domainLabel.MatchString(label) {
				return errors.New("域名格式无效，请使用小写域名或Punycode")
			}
		}
		if u.Port() != "" {
			if _, err := net.LookupPort("tcp", u.Port()); err != nil {
				return errors.New("域名端口无效")
			}
		}
	}
	return nil
}
func PlaybackOrigin(ctx context.Context, db *pgxpool.Pool, fallback string) (string, error) {
	if db == nil {
		return fallback, nil
	}
	var value string
	if err := db.QueryRow(ctx, `SELECT coalesce(value#>>'{access,playbackUrl}','') FROM application_settings WHERE singleton`).Scan(&value); err != nil {
		return "", err
	}
	if value == "" {
		return fallback, nil
	}
	return value, nil
}
