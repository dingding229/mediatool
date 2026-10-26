package externalapi

// Operator-only acceptance probe: creates exactly one temporary status.read
// credential, never serializes the bearer, and cleans it even on failure.
import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"net"
	"net/http"
	"time"
)

type Verification struct {
	CredentialID  string `json:"credentialId"`
	SameToken     bool   `json:"sameToken"`
	ViewAudits    int    `json:"viewAudits"`
	ReadStatus    int    `json:"readStatus"`
	ClientIP      string `json:"clientIP"`
	IPSource      string `json:"ipSource"`
	RevokedStatus int    `json:"revokedStatus"`
	Deleted       bool   `json:"deleted"`
}

func VerifyLocalLifecycle(ctx context.Context, db *pgxpool.Pool, master, addr string) (out Verification, result error) {
	_, port, e := net.SplitHostPort(addr)
	if e != nil || port == "" {
		return out, ErrUnavailable
	}
	signer, e := NewSigner(master)
	if e != nil {
		return out, e
	}
	repo := Repository{DB: db, Signer: signer}
	key, e := repo.Create(ctx, "t15-check-"+time.Now().UTC().Format("20060102T150405"), []string{"status.read"}, "offline-operator:t15-check")
	if e != nil {
		return out, e
	}
	out.CredentialID = key.ID
	defer func() {
		finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if out.Deleted {
			return
		}
		if e := repo.Revoke(finish, key.ID, "offline-operator:t15-check"); e != nil && !out.Deleted {
			result = errors.Join(result, ErrUnavailable)
		}
		if !out.Deleted {
			if e := repo.Delete(finish, key.ID, "offline-operator:t15-check"); e != nil {
				result = errors.Join(result, ErrUnavailable)
			} else {
				out.Deleted = true
			}
		}
	}()
	first, e := repo.Reveal(ctx, key.ID, "offline-operator:t15-check", "view")
	if e != nil {
		return out, e
	}
	second, e := repo.Reveal(ctx, key.ID, "offline-operator:t15-check", "view")
	if e != nil {
		return out, e
	}
	out.SameToken = key.Token == first.Token && first.Token == second.Token
	first.Token = ""
	second.Token = ""
	if !out.SameToken {
		return out, ErrUnavailable
	}
	if e = db.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='external_api.revealed.view' AND details->>'id'=$1`, key.ID).Scan(&out.ViewAudits); e != nil {
		return out, e
	}
	if out.ViewAudits != 2 {
		return out, ErrUnavailable
	}
	client := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	request := func() (int, error) {
		r, e := http.NewRequestWithContext(ctx, "GET", "http://"+net.JoinHostPort("127.0.0.1", port)+"/api/external/v1/status", nil)
		if e != nil {
			return 0, ErrUnavailable
		}
		r.Header.Set("Authorization", "Bearer "+key.Token)
		resp, e := client.Do(r)
		if e != nil {
			return 0, ErrUnavailable
		}
		_, readErr := io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		closeErr := resp.Body.Close()
		if readErr != nil || closeErr != nil {
			return 0, ErrUnavailable
		}
		return resp.StatusCode, nil
	}
	out.ReadStatus, e = request()
	if e != nil || out.ReadStatus != 200 {
		return out, ErrUnavailable
	}
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
wait:
	for {
		if e = db.QueryRow(ctx, `SELECT coalesce(host(client_ip),''),ip_source FROM external_api_calls WHERE key_id=$1 AND status=200 ORDER BY id DESC LIMIT 1`, key.ID).Scan(&out.ClientIP, &out.IPSource); e == nil && out.ClientIP != "" {
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-deadline.C:
			return out, ErrUnavailable
		case <-tick.C:
			continue wait
		}
	}
	if e = repo.Revoke(ctx, key.ID, "offline-operator:t15-check"); e != nil {
		return out, e
	}
	out.RevokedStatus, e = request()
	key.Token = ""
	if e != nil || out.RevokedStatus != 401 && out.RevokedStatus != 403 {
		return out, ErrUnavailable
	}
	if e = repo.Delete(ctx, key.ID, "offline-operator:t15-check"); e != nil {
		return out, e
	}
	out.Deleted = true
	select {
	case <-ctx.Done():
		return out, ctx.Err()
	case <-time.After(2200 * time.Millisecond):
	}
	var n int
	if e = db.QueryRow(ctx, `SELECT (SELECT count(*)FROM external_api_keys WHERE id=$1)+(SELECT count(*)FROM external_api_calls WHERE key_id=$1)`, key.ID).Scan(&n); e != nil || n != 0 {
		return out, ErrUnavailable
	}
	return out, nil
}
