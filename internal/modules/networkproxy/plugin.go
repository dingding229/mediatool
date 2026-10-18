package networkproxy

import (
	"context"
)

type Summary struct {
	Configured, TMDB, Sites bool
	Revision                int64
}

// Summary never decrypts or returns the proxy URL/account/password.
func (r Repository) Summary(ctx context.Context) (out Summary, err error) {
	err = r.DB.QueryRow(ctx, `SELECT octet_length(url_ciphertext)>0,tmdb_enabled,sites_enabled,revision FROM network_proxy_settings WHERE id='main'`).Scan(&out.Configured, &out.TMDB, &out.Sites, &out.Revision)
	return out, err
}
