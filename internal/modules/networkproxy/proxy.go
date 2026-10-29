// Package networkproxy provides explicitly scoped, encrypted outbound proxy settings.
package networkproxy

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/playback"
	deferutil "media-tool/internal/platform/cleanup"
	"media-tool/internal/platform/secrets"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var ErrRevision = errors.New("代理设置已变更，请刷新")

type Repository struct {
	DB    *pgxpool.Pool
	Vault *secrets.Vault
}

func (r Repository) open(out Record, raw []byte) Record {
	if len(raw) == 0 {
		return out
	}
	values, err := r.Vault.Open("network-proxy:main", raw)
	if err != nil {
		out.Configured = true
		out.NeedsInput = true
		return out
	}
	return decodePayload(out, values)
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
	return r.open(out, raw), nil
}
func (r Repository) prepare(old Record, raw []byte, in Update) ([]byte, error) {
	if in.Clear {
		if in.TMDB || in.Sites {
			return nil, ErrInvalid
		}
		return []byte{}, nil
	}
	if in.Protocol != "" && in.Protocol != "http" && in.Protocol != "socks5" {
		return nil, ErrInvalid
	}
	if old.NeedsInput && empty(in.Fields) && in.Password == "" && !in.ClearPassword {
		return raw, nil
	}
	if strings.HasPrefix(in.Host, "[") && strings.HasSuffix(in.Host, "]") {
		in.Host = in.Host[1 : len(in.Host)-1]
	}
	in.Host = strings.ToLower(in.Host)
	if in.ClearPassword && in.Password != "" {
		return nil, ErrInvalid
	}
	if in.Password == "" && !in.ClearPassword {
		if old.NeedsInput {
			return nil, ErrNeedsInput
		}
		if old.HasPassword && in.Fields != old.Fields {
			return nil, ErrPassword
		}
		in.Password = old.Password
	}
	next := Record{Fields: in.Fields, Password: in.Password, TMDB: in.TMDB, Sites: in.Sites}
	if err := Validate(next); err != nil {
		return nil, err
	}
	if empty(next.Fields) {
		return []byte{}, nil
	}
	return r.Vault.Seal("network-proxy:main", map[string]string{"v": "2", "protocol": in.Protocol, "host": in.Host, "port": strconv.Itoa(in.Port), "username": in.Username, "password": in.Password})
}
func (r Repository) Save(ctx context.Context, in Update) (Record, error) {
	out := Record{}
	var raw []byte
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer deferutil.Rollback(ctx, tx)
	if err = tx.QueryRow(ctx, `SELECT tmdb_enabled,sites_enabled,url_ciphertext,revision FROM network_proxy_settings WHERE id='main' FOR UPDATE`).Scan(&out.TMDB, &out.Sites, &raw, &out.Revision); err != nil {
		return out, err
	}
	if out.Revision != in.Revision {
		return out, ErrRevision
	}
	raw, err = r.prepare(r.open(out, raw), raw, in)
	if err != nil {
		return Record{}, err
	}
	_, err = tx.Exec(ctx, `UPDATE network_proxy_settings SET tmdb_enabled=$1,sites_enabled=$2,url_ciphertext=$3,revision=revision+1,updated_at=now() WHERE id='main'`, in.TMDB, in.Sites, raw)
	if err != nil {
		return Record{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Record{}, err
	}
	out.TMDB = in.TMDB
	out.Sites = in.Sites
	out.Revision++
	return r.open(out, raw), nil
}
func Client(r Record, scope string) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	if (scope == "tmdb" && r.TMDB) || (scope == "ptsite" && r.Sites) || (scope == "plugin" && (r.Configured || r.NeedsInput || !empty(r.Fields))) {
		if e := Validate(r); e != nil {
			return nil, e
		}
		if empty(r.Fields) {
			return nil, ErrInvalid
		}
		u, _ := url.Parse(r.proxyURL())
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

// PublicClient reuses the existing DNS-pinned direct/proxy transport. A proxy
// must not perform a second target lookup that could resolve a private address.
func PublicClient(r Record, scope string, policy playback.Policy) (*http.Client, error) {
	client, err := Client(r, scope)
	if err != nil {
		return nil, err
	}
	transport := client.Transport.(*http.Transport)
	raw := ""
	if transport.Proxy != nil {
		raw = r.proxyURL()
	}
	if err = pinHostTransport(transport, policy, raw); err != nil {
		return nil, err
	}
	return client, nil
}
