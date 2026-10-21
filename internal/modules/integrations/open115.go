package integrations

import (
	"context"
	"errors"
	deferutil "media-tool/internal/platform/cleanup"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"media-tool/internal/adapters/open115"
	"media-tool/internal/modules/storage"
)

func (m *Manager) officialClient() open115.Client { return open115.Client{HTTP: m.OutboundHTTP} }

// officialToken serializes refreshes across all processes using the integration
// row lock. Rotated access/refresh tokens are encrypted and committed together.
func (m *Manager) officialToken(ctx context.Context, id, rejected string, playback bool) (tokenResult string, tokenErr error) {
	started := time.Now()
	defer func() {
		tokenErr = storage.ContextFailure(ctx, tokenErr)
		storage.ObserveResolve(ctx, storage.StageOfficialToken, started, tokenErr)
	}()
	tx, err := m.Repository.DB.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", ErrUnavailable
	}
	defer deferutil.Rollback(context.Background(), tx)
	c, err := m.Repository.scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM integrations WHERE id=$1 FOR UPDATE`, id))
	if err != nil {
		return "", err
	}
	if c.Record.Kind != "open115" || (playback && !c.Record.Enabled) {
		return "", open115.ErrAuthorization
	}
	values := c.Values
	if values["refreshBlocked"] != "" {
		return "", open115.ErrAuthorization
	}
	expires, _ := time.Parse(time.RFC3339Nano, values["accessExpiresAt"])
	usable := values["accessToken"] != "" && (expires.IsZero() || time.Until(expires) > 45*time.Second)
	if usable && (rejected == "" || rejected != values["accessToken"]) {
		return values["accessToken"], nil
	}
	if values["refreshToken"] == "" {
		return "", open115.ErrAuthorization
	}
	after, _ := time.Parse(time.RFC3339Nano, values["refreshAfter"])
	if time.Now().Before(after) {
		return "", open115.ErrRateLimited
	}
	// Record a cooldown even for nonterminal errors; never busy-loop refresh.
	values["refreshAfter"] = time.Now().Add(time.Minute).UTC().Format(time.RFC3339Nano)
	refreshStarted := time.Now()
	fresh, refreshErr := m.officialClient().Refresh(ctx, values["refreshToken"])
	storage.ObserveResolve(ctx, storage.StageOfficialRefresh, refreshStarted, refreshErr)
	if refreshErr == nil {
		values["accessToken"], values["refreshToken"] = fresh.AccessToken, fresh.RefreshToken
		values["accessExpiresAt"] = time.Now().Add(time.Duration(fresh.ExpiresIn) * time.Second).UTC().Format(time.RFC3339Nano)
		delete(values, "refreshBlocked")
	} else {
		var api *open115.APIError
		if errors.As(refreshErr, &api) {
			if api.TerminalRefresh() {
				values["refreshBlocked"] = strconv.FormatInt(api.Code, 10)
			}
		} else {
			// The refresh may have committed upstream before a lost response. Do not
			// reuse potentially invalid refresh tokens indefinitely; reauthorize.
			values["refreshBlocked"] = "outcome_unknown"
		}
	}
	encrypted, err := m.Repository.Vault.Seal(id, values)
	if err != nil {
		return "", ErrUnavailable
	}
	// Persist a received token pair even if the requesting player disconnected.
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_, err = tx.Exec(saveCtx, `UPDATE integrations SET credentials_ciphertext=$2,revision=revision+1,updated_at=now() WHERE id=$1`, id, encrypted)
	if err != nil {
		return "", ErrUnavailable
	}
	if err = tx.Commit(saveCtx); err != nil {
		return "", ErrUnavailable
	}
	if refreshErr != nil {
		return "", refreshErr
	}
	return fresh.AccessToken, nil
}

func (m *Manager) officialCall(ctx context.Context, id string, playback bool, call func(string) error) error {
	token, err := m.officialToken(ctx, id, "", playback)
	if err != nil {
		return err
	}
	err = call(token)
	var api *open115.APIError
	if errors.As(err, &api) && api.ExpiredAccess() {
		latest, refreshErr := m.officialToken(ctx, id, token, playback)
		if refreshErr != nil {
			return refreshErr
		}
		return call(latest) // one retry only, never reuse a known rejected token
	}
	return err
}

func (m *Manager) resolveOfficial(ctx context.Context, c Credentials, ref storage.FileRef) (storage.DownloadLink, error) {
	prefix := c.String("cloudMountPrefix")
	if prefix == "" || path.Clean(ref.Path) != ref.Path || !strings.HasPrefix(ref.Path, prefix+"/") {
		return storage.DownloadLink{}, storage.ErrForbidden
	}
	remotePath := strings.TrimPrefix(ref.Path, prefix)
	var info open115.FileInfo
	err := m.officialCall(ctx, c.Record.ID, true, func(token string) error {
		var err error
		started := time.Now()
		info, err = m.officialClient().FileByPath(ctx, token, remotePath)
		storage.ObserveResolve(ctx, storage.StageOfficialPath, started, err)
		return err
	})
	if err != nil {
		return storage.DownloadLink{}, err
	}
	identityStarted := time.Now()
	if ref.ExpectedSize > 0 && info.Size != ref.ExpectedSize || ref.PickCode != "" && ref.PickCode != info.PickCode {
		storage.ObserveResolve(ctx, storage.StageFileIdentity, identityStarted, storage.ErrInvalidResponse)
		return storage.DownloadLink{}, storage.ErrInvalidResponse
	}
	storage.ObserveResolve(ctx, storage.StageFileIdentity, identityStarted, nil)
	agent := ref.UserAgent
	if agent == "" {
		agent = "MediaTool-115-Open/1.0"
	}
	var direct open115.Download
	err = m.officialCall(ctx, c.Record.ID, true, func(token string) error {
		var err error
		started := time.Now()
		direct, err = m.officialClient().Download(ctx, token, info.PickCode, agent)
		storage.ObserveResolve(ctx, storage.StageOfficialDownload, started, err)
		return err
	})
	if err != nil {
		return storage.DownloadLink{}, err
	}
	identityStarted = time.Now()
	if direct.FileName != info.Name || direct.FileSize != info.Size {
		storage.ObserveResolve(ctx, storage.StageDownloadIdentity, identityStarted, storage.ErrInvalidResponse)
		return storage.DownloadLink{}, storage.ErrInvalidResponse
	}
	storage.ObserveResolve(ctx, storage.StageDownloadIdentity, identityStarted, nil)
	// The API request uses the player UA. This source is intentionally direct-only:
	// failures must not silently become server media traffic.
	return storage.DownloadLink{URL: direct.URL.URL, UserAgent: agent, CallerUserAgent: true, DirectOnly: true, CacheUntil: time.Now().Add(45 * time.Second)}, nil
}

// PlaybackVersion partitions caches across provider selection and credential
// updates while keeping CloudDrive2 as the scanner/upload source.
func (m *Manager) PlaybackVersion(ctx context.Context, ref storage.FileRef) (string, error) {
	var base string
	if ref.CloudFileID != "" {
		if err := m.Repository.DB.QueryRow(ctx, `SELECT i.id::text||':'||i.revision::text||':'||f.updated_at::text FROM cloud_files f JOIN integrations i ON i.id=f.storage_id WHERE f.id=$1 AND f.state='present'`, ref.CloudFileID).Scan(&base); err != nil {
			return "", err
		}
	}

	if ref.AssetID != "" {
		err := m.Repository.DB.QueryRow(ctx, `SELECT i.id::text||':'||i.revision::text||':'||f.updated_at::text FROM media_assets a JOIN cloud_files f ON f.id=a.cloud_file_id JOIN integrations i ON i.id=f.storage_id WHERE a.id=$1 AND f.state='present'`, ref.AssetID).Scan(&base)
		if err != nil {
			return "", err
		}
	} else if ref.CloudFileID == "" {
		c, err := m.Repository.Default(ctx, "clouddrive2")
		if err != nil {
			return "", err
		}
		base = c.Record.ID + ":" + strconv.FormatInt(c.Record.Revision, 10)
	}
	var selected string
	err := m.Repository.DB.QueryRow(ctx, `SELECT id::text||':'||revision::text FROM integrations WHERE kind='open115' AND enabled LIMIT 1`).Scan(&selected)
	if errors.Is(err, pgx.ErrNoRows) {
		return "cd2:" + base, nil
	}
	if err != nil {
		return "", err
	}
	return "open115:" + selected + ":" + base, nil
}

// Read-only account query through the authorized official identity. Never use a
// CloudDrive mount's quota as fallback or assume the two accounts are identical.
func (m *Manager) OfficialAccountInfo(ctx context.Context) (open115.AccountInfo, Record, error) {
	c, err := m.Repository.Default(ctx, "open115")
	if err != nil {
		return open115.AccountInfo{}, Record{}, err
	}
	var out open115.AccountInfo
	err = m.officialCall(ctx, c.Record.ID, true, func(token string) error { var e error; out, e = m.officialClient().AccountInfo(ctx, token); return e })
	return out, c.Record, err
}
