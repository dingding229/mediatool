// Package networkproxy provides explicitly scoped, encrypted outbound proxy settings.
package networkproxy

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/platform/secrets"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var ErrInvalid = errors.New("代理地址须使用 http/https/socks5/socks5h，不含路径、查询或片段")
var ErrRevision = errors.New("代理设置已变更，请刷新")

type Record struct {
	URL      string `json:"url"`
	TMDB     bool   `json:"tmdb"`
	Sites    bool   `json:"sites"`
	Revision int64  `json:"revision"`
}
type Repository struct {
	DB    *pgxpool.Pool
	Vault *secrets.Vault
}

func Validate(r Record) error {
	if r.URL == "" {
		if r.TMDB || r.Sites {
			return ErrInvalid
		}
		return nil
	}
	if len(r.URL) > 4096 || strings.ContainsAny(r.URL, "\r\n\x00") {
		return ErrInvalid
	}
	u, e := url.Parse(r.URL)
	if e != nil || u.Hostname() == "" || u.Opaque != "" || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" {
		return ErrInvalid
	}
	switch u.Scheme {
	case "http", "https", "socks5", "socks5h":
	default:
		return ErrInvalid
	}
	if p := u.Port(); p != "" {
		n, e := strconv.Atoi(p)
		if e != nil || n < 1 || n > 65535 {
			return ErrInvalid
		}
	}
	return nil
}
func (r Repository) Get(ctx context.Context) (Record, error) {
	out := Record{Revision: 1}
	if r.DB == nil {
		return out, nil
	}
	var raw []byte
	if e := r.DB.QueryRow(ctx, `SELECT tmdb_enabled,sites_enabled,url_ciphertext,revision FROM network_proxy_settings WHERE id='main'`).Scan(&out.TMDB, &out.Sites, &raw, &out.Revision); e != nil {
		return out, e
	}
	if len(raw) > 0 {
		values, e := r.Vault.Open("network-proxy:main", raw)
		if e != nil {
			return out, e
		}
		out.URL = values["url"]
	}
	return out, Validate(out)
}
func (r Repository) Save(ctx context.Context, in Record) (Record, error) {
	if e := Validate(in); e != nil {
		return in, e
	}
	var raw []byte
	var e error
	if in.URL != "" {
		raw, e = r.Vault.Seal("network-proxy:main", map[string]string{"url": in.URL})
		if e != nil {
			return in, e
		}
	} else {
		raw = []byte{}
	}
	tag, e := r.DB.Exec(ctx, `UPDATE network_proxy_settings SET tmdb_enabled=$1,sites_enabled=$2,url_ciphertext=$3,revision=revision+1,updated_at=now() WHERE id='main' AND revision=$4`, in.TMDB, in.Sites, raw, in.Revision)
	if e != nil {
		return in, e
	}
	if tag.RowsAffected() != 1 {
		return in, ErrRevision
	}
	return r.Get(ctx)
}
func Client(r Record, scope string) (*http.Client, error) {
	if e := Validate(r); e != nil {
		return nil, e
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	if (scope == "tmdb" && r.TMDB) || (scope == "ptsite" && r.Sites) {
		u, _ := url.Parse(r.URL)
		transport.Proxy = http.ProxyURL(u)
	}
	return &http.Client{Transport: transport, Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, nil
}

func (r Repository) Revision(ctx context.Context) (int64, error) {
	if r.DB == nil {
		return 1, nil
	}
	var n int64
	err := r.DB.QueryRow(ctx, `SELECT revision FROM network_proxy_settings WHERE id='main'`).Scan(&n)
	return n, err
}
