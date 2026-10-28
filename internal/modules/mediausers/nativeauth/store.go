// Package nativeauth implements native Emby authentication, not browsing.
package nativeauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"media-tool/internal/platform/clientip"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/accounts"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/mediausers"
	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/platform/auth"
)

const Lifetime = 365 * 24 * time.Hour
const TouchInterval = 10 * time.Minute

var passwordSlots = make(chan struct{}, 2)

type DeviceObserver interface{ ObserveDevice(user, device, ip string) }
type Store struct {
	Observer DeviceObserver
	DB       *pgxpool.Pool
	Secret   string
}

var _ contracts.AccountService = (*Store)(nil)

const profileColumns = `replace(u.id::text,'-',''),u.username,u.enabled,u.credential_version,u.max_streams,u.expires_at,
 EXISTS(SELECT 1 FROM media_user_library_grants g JOIN native_libraries l ON l.id=g.library_id WHERE g.user_id=u.id AND g.can_play AND NOT l.hidden),
 EXISTS(SELECT 1 FROM media_user_library_grants g JOIN native_libraries l ON l.id=g.library_id WHERE g.user_id=u.id AND g.can_play AND g.can_download AND NOT l.hidden)`

func profileDest(u *contracts.User) []any {
	return []any{&u.ID, &u.Name, &u.Enabled, &u.CredentialVersion, &u.MaxStreams, &u.ExpiresAt, &u.CanPlay, &u.CanDownload}
}
func safeText(s string, limit int) bool {
	return len(s) <= limit && utf8.ValidString(s) && !strings.ContainsFunc(s, unicode.IsControl)
}
func validInfo(d contracts.DeviceInfo, login bool) bool {
	return (!login || d.DeviceID != "") && safeText(d.DeviceID, 256) && safeText(d.Name, 256) && safeText(d.Client, 128) && safeText(d.Version, 64) && safeText(d.UserAgent, 2048)
}
func ipValue(raw string) any {
	ip := clientip.Sanitize(raw)
	if ip == "" {
		return nil
	}
	return ip
}
func passwordPermit() (func(), error) {
	select {
	case passwordSlots <- struct{}{}:
		return func() { <-passwordSlots }, nil
	default:
		return nil, contracts.ErrRateLimited
	}
}
func rollback(ctx context.Context, tx pgx.Tx, err *error) {
	bounded, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	if e := tx.Rollback(bounded); e != nil && !errors.Is(e, pgx.ErrTxClosed) {
		*err = contracts.ErrUnavailable
	}
}

func (s *Store) Login(ctx context.Context, username, password string, info contracts.DeviceInfo) (out contracts.LoginResult, err error) {
	if s == nil || s.DB == nil || s.Secret == "" {
		return out, contracts.ErrUnavailable
	}
	if !validInfo(info, true) {
		return out, contracts.ErrUnauthenticated
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	name, nameErr := accounts.NormalizeUsername(username)
	if len(username) > 256 || len(password) > 512 {
		return out, contracts.ErrUnauthenticated
	}
	if err = s.checkLimits(ctx, name, info.ClientIP); err != nil {
		return out, err
	}
	release, err := passwordPermit()
	if err != nil {
		return out, err
	}
	defer release()
	var hash string
	var version int64
	var id string
	e := s.DB.QueryRow(ctx, `SELECT id::text,password_hash,credential_version FROM media_users WHERE username=$1`, name).Scan(&id, &hash, &version)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return out, contracts.ErrUnavailable
	}
	hasPassword := hash != ""
	if !hasPassword {
		hash = auth.DummyPasswordHash
	}
	match := auth.VerifyPassword(hash, password)
	if nameErr != nil || e != nil || !hasPassword || !match {
		return out, s.failed(ctx, name, info.ClientIP)
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return out, contracts.ErrUnavailable
	}
	defer rollback(ctx, tx, &err)
	var mustChange bool
	dest := append(profileDest(&out.User), &mustChange)
	err = tx.QueryRow(ctx, `SELECT `+profileColumns+`,u.must_change_password FROM media_users u WHERE u.id=$1 AND u.credential_version=$2 AND u.enabled AND (u.expires_at IS NULL OR u.expires_at>now()) FOR UPDATE OF u`, id, version).Scan(dest...)
	if errors.Is(err, pgx.ErrNoRows) {
		// Release the checked-out transaction connection before the legacy rate
		// repository needs another pool connection (also safe with a two-slot pool).
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			return out, contracts.ErrUnavailable
		}
		return out, s.failed(ctx, name, info.ClientIP)
	}
	if err != nil {
		return out, contracts.ErrUnavailable
	}
	if mustChange {
		return out, contracts.ErrPasswordChangeRequired
	}
	deviceID := jobs.UUID()
	err = tx.QueryRow(ctx, `INSERT INTO media_user_devices(id,user_id,device_key,name,client,app_version,last_ip,last_user_agent) VALUES($1,$2,$3,$4,$5,$6,$7,$8)
 ON CONFLICT(user_id,device_key) DO UPDATE SET name=excluded.name,client=excluded.client,app_version=excluded.app_version,last_seen_at=now(),last_ip=excluded.last_ip,last_user_agent=excluded.last_user_agent RETURNING id::text`, deviceID, id, info.DeviceID, info.Name, info.Client, info.Version, ipValue(info.ClientIP), info.UserAgent).Scan(&deviceID)
	if err != nil {
		return out, contracts.ErrUnavailable
	}
	if _, err = tx.Exec(ctx, `UPDATE media_user_tokens SET revoked_at=now() WHERE user_id=$1 AND device_id=$2 AND revoked_at IS NULL`, id, deviceID); err != nil {
		return out, contracts.ErrUnavailable
	}
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return out, contracts.ErrUnavailable
	}
	out.AccessToken = hex.EncodeToString(raw)
	digest := sha256.Sum256([]byte(out.AccessToken))
	out.Token = contracts.Token{Digest: digest, CredentialVersion: version}
	err = tx.QueryRow(ctx, `INSERT INTO media_user_tokens(token_digest,user_id,device_id,credential_version,expires_at) VALUES($1,$2,$3,$4,now()+interval '365 days') RETURNING expires_at`, digest[:], id, deviceID, version).Scan(&out.Token.ExpiresAt)
	if err != nil {
		return contracts.LoginResult{}, contracts.ErrUnavailable
	}
	if err = tx.Commit(ctx); err != nil {
		return contracts.LoginResult{}, contracts.ErrUnavailable
	}
	out.Device = contracts.Device{ID: deviceID, UserID: out.User.ID, DeviceKey: info.DeviceID}
	if s.Observer != nil {
		s.Observer.ObserveDevice(out.User.ID, out.Device.ID, info.ClientIP)
	}
	return out, nil
}

func (s *Store) Authenticate(ctx context.Context, raw string, info contracts.DeviceInfo) (u contracts.User, d contracts.Device, t contracts.Token, err error) {
	if s == nil || s.DB == nil {
		return u, d, t, contracts.ErrUnavailable
	}
	if len(raw) != 64 || !validInfo(info, false) {
		return u, d, t, contracts.ErrUnauthenticated
	}
	if _, e := hex.DecodeString(raw); e != nil {
		return u, d, t, contracts.ErrUnauthenticated
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	t.Digest = sha256.Sum256([]byte(raw))
	var seen time.Time
	dest := append(profileDest(&u), &d.ID, &d.DeviceKey, &t.CredentialVersion, &t.ExpiresAt, &seen)
	err = s.DB.QueryRow(ctx, `SELECT `+profileColumns+`,d.id::text,d.device_key,t.credential_version,t.expires_at,t.last_seen_at FROM media_user_tokens t JOIN media_users u ON u.id=t.user_id JOIN media_user_devices d ON d.id=t.device_id AND d.user_id=u.id
 WHERE t.token_digest=$1 AND t.revoked_at IS NULL AND t.expires_at>now() AND u.enabled AND NOT u.must_change_password AND (u.expires_at IS NULL OR u.expires_at>now()) AND t.credential_version=u.credential_version`, t.Digest[:]).Scan(dest...)
	if errors.Is(err, pgx.ErrNoRows) {
		return contracts.User{}, contracts.Device{}, contracts.Token{}, contracts.ErrUnauthenticated
	}
	if err != nil {
		return u, d, t, contracts.ErrUnavailable
	}
	if info.DeviceID != "" && info.DeviceID != d.DeviceKey {
		return contracts.User{}, contracts.Device{}, contracts.Token{}, contracts.ErrUnauthenticated
	}
	d.UserID = u.ID
	if time.Since(seen) >= TouchInterval {
		// Commit the token touch before updating device evidence. Keeping token and
		// device locks in one statement would invert Login's device→token ordering.
		var renewedDevice string
		touchErr := s.DB.QueryRow(ctx, `UPDATE media_user_tokens t SET last_seen_at=now(),expires_at=now()+interval '365 days' FROM media_users u WHERE t.token_digest=$1 AND t.user_id=u.id AND t.revoked_at IS NULL AND t.expires_at>now() AND t.credential_version=u.credential_version AND u.enabled AND NOT u.must_change_password AND (u.expires_at IS NULL OR u.expires_at>now()) AND t.last_seen_at<=now()-interval '10 minutes' RETURNING t.device_id::text,t.expires_at`, t.Digest[:]).Scan(&renewedDevice, &t.ExpiresAt)
		if touchErr != nil && !errors.Is(touchErr, pgx.ErrNoRows) {
			return u, d, t, contracts.ErrUnavailable
		}
		if touchErr == nil {
			if _, err = s.DB.Exec(ctx, `UPDATE media_user_devices SET last_seen_at=now(),last_ip=$2::inet,last_user_agent=coalesce(nullif($3,''),last_user_agent),name=coalesce(nullif($4,''),name),client=coalesce(nullif($5,''),client),app_version=coalesce(nullif($6,''),app_version) WHERE id=$1::uuid`, renewedDevice, ipValue(info.ClientIP), info.UserAgent, info.Name, info.Client, info.Version); err != nil {
				return u, d, t, contracts.ErrUnavailable
			}
		}
	}
	if s.Observer != nil {
		s.Observer.ObserveDevice(u.ID, d.ID, info.ClientIP)
	}
	return u, d, t, nil
}
func (s *Store) Logout(ctx context.Context, t contracts.Token) error {
	if s == nil || s.DB == nil {
		return contracts.ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tag, err := s.DB.Exec(ctx, `UPDATE media_user_tokens SET revoked_at=now() WHERE token_digest=$1 AND revoked_at IS NULL`, t.Digest[:])
	if err != nil {
		return contracts.ErrUnavailable
	}
	if tag.RowsAffected() != 1 {
		return contracts.ErrUnauthenticated
	}
	return nil
}
func (s *Store) GetUser(ctx context.Context, id string) (contracts.User, error) {
	if s == nil || s.DB == nil {
		return contracts.User{}, contracts.ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var u contracts.User
	var must bool
	err := s.DB.QueryRow(ctx, `SELECT `+profileColumns+`,u.must_change_password FROM media_users u WHERE replace(u.id::text,'-','')=$1`, id).Scan(append(profileDest(&u), &must)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return contracts.User{}, contracts.ErrUnauthenticated
	}
	if err != nil {
		return contracts.User{}, contracts.ErrUnavailable
	}
	u.MustChangePassword = must
	return u, nil
}
func (s *Store) PublicUsers(ctx context.Context) ([]contracts.PublicUser, error) {
	if s == nil || s.DB == nil {
		return nil, contracts.ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	rows, err := s.DB.Query(ctx, `SELECT username,replace(id::text,'-',''),password_hash<>'' FROM media_users WHERE enabled AND (expires_at IS NULL OR expires_at>now()) ORDER BY username,id LIMIT 500`)
	if err != nil {
		return nil, contracts.ErrUnavailable
	}
	defer rows.Close()
	out := []contracts.PublicUser{}
	for rows.Next() {
		var u contracts.PublicUser
		if err := rows.Scan(&u.Name, &u.Id, &u.HasPassword); err != nil {
			return nil, contracts.ErrUnavailable
		}
		out = append(out, u)
	}
	if rows.Err() != nil {
		return nil, contracts.ErrUnavailable
	}
	return out, nil
}
func (s *Store) checkLimits(ctx context.Context, name, ip string) error {
	// Reuse the legacy HMAC scopes and one-minute window. Successful logins never
	// call TakeAttempt and therefore do not consume the failure allowance.
	for _, scope := range []struct {
		key   string
		limit int
	}{{"ip:" + ip, 20}, {"user:" + name, 10}} {
		digest := limitDigest(s.Secret, scope.key)
		var blocked bool
		err := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM media_user_auth_limits WHERE scope_digest=$1 AND window_start>now()-interval '1 minute' AND attempts>=$2)`, digest, scope.limit).Scan(&blocked)
		if err != nil {
			return contracts.ErrUnavailable
		}
		if blocked {
			return contracts.ErrRateLimited
		}
	}
	return nil
}
func (s *Store) failed(ctx context.Context, name, ip string) error {
	for _, scope := range []struct {
		key   string
		limit int
	}{{"ip:" + ip, 20}, {"user:" + name, 10}} {
		if _, err := (mediausers.Repository{DB: s.DB}).TakeAttempt(ctx, s.Secret, scope.key, scope.limit); err != nil {
			return contracts.ErrUnavailable
		}
	}
	return contracts.ErrUnauthenticated
}
