package integrations

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	qrcode "github.com/skip2/go-qrcode"
	"media-tool/internal/adapters/open115"
	"media-tool/internal/modules/jobs"
)

var ErrAuthSession = errors.New("115 authorization session invalid, expired or replaced")

type OfficialAuthorization struct {
	SessionID   string    `json:"sessionId"`
	State       string    `json:"state"`
	QRCodeImage string    `json:"qrCodeImage,omitempty"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

func (m *Manager) StartOfficialAuthorization(ctx context.Context, id, owner string, version int64) (OfficialAuthorization, error) {
	c, err := m.Repository.Get(ctx, id)
	if err != nil {
		return OfficialAuthorization{}, err
	}
	if c.Record.Kind != "open115" || c.String("appId") == "" {
		return OfficialAuthorization{}, errors.New("请先保存自己的115 AppID，再扫码授权")
	}
	verifier, challenge, err := open115.NewPKCE()
	if err != nil {
		return OfficialAuthorization{}, ErrUnavailable
	}
	device, err := m.officialClient().DeviceCode(ctx, c.String("appId"), challenge)
	if err != nil {
		return OfficialAuthorization{}, err
	}
	// Generate locally; never send the QR content or verifier to an image service.
	png, err := qrcode.Encode(device.QRCode, qrcode.Medium, 256)
	if err != nil {
		return OfficialAuthorization{}, ErrUnavailable
	}
	session := jobs.UUID()
	expires := time.Now().Add(5 * time.Minute)
	payload, err := m.Repository.Vault.Seal("open115-auth:"+session, map[string]string{"uid": device.UID, "time": strconv.FormatInt(device.Time, 10), "sign": device.Sign, "verifier": verifier})
	if err != nil {
		return OfficialAuthorization{}, err
	}
	// Clear expired verifiers opportunistically; no ongoing source/credential
	// deletion. Completed/cancelled sessions are already scrubbed.
	if _, err := m.Repository.DB.Exec(ctx, `UPDATE open115_authorizations SET state='expired',payload_ciphertext='\x'::bytea WHERE state IN ('pending','exchanging') AND expires_at<now()`); err != nil {
		return OfficialAuthorization{}, ErrUnavailable
	}
	tx, err := m.Repository.DB.Begin(ctx)
	if err != nil {
		return OfficialAuthorization{}, ErrUnavailable
	}
	defer tx.Rollback(context.Background())
	// Lock the integration before replacing old pending sessions, so two start
	// requests cannot leave both sessions active. App edits invalidate the flow.
	current, err := m.Repository.scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM integrations WHERE id=$1 FOR UPDATE`, id))
	if err != nil {
		return OfficialAuthorization{}, err
	}
	if current.Record.Revision != c.Record.Revision {
		return OfficialAuthorization{}, ErrRevision
	}
	if _, err = tx.Exec(ctx, `UPDATE open115_authorizations SET state='cancelled',payload_ciphertext='\x'::bytea WHERE integration_id=$1 AND state IN ('pending','exchanging')`, id); err != nil {
		return OfficialAuthorization{}, ErrUnavailable
	}
	_, err = tx.Exec(ctx, `INSERT INTO open115_authorizations(id,integration_id,owner_account_id,owner_version,integration_revision,payload_ciphertext,expires_at)VALUES($1,$2,$3,$4,$5,$6,$7)`, session, id, owner, version, c.Record.Revision, payload, expires)
	if err != nil {
		return OfficialAuthorization{}, ErrUnavailable
	}
	if err = tx.Commit(ctx); err != nil {
		return OfficialAuthorization{}, ErrUnavailable
	}
	return OfficialAuthorization{SessionID: session, State: "waiting", QRCodeImage: "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), ExpiresAt: expires}, nil
}

// The QR status request may long-poll. Never hold a database transaction or
// integration lock over it: existing playback and cancellation must stay usable.
func (m *Manager) PollOfficialAuthorization(ctx context.Context, id, session, owner string, version int64, cancelled bool) (OfficialAuthorization, error) {
	var poll open115.Poll
	var values map[string]string
	if !cancelled {
		c, err := m.Repository.Get(ctx, id)
		if err != nil {
			return OfficialAuthorization{}, err
		}
		if c.Record.Kind != "open115" {
			return OfficialAuthorization{}, ErrAuthSession
		}
		snapshot, err := scanOfficialAuth(m.Repository.DB.QueryRow(ctx, `SELECT `+officialAuthColumns+` FROM open115_authorizations WHERE id=$1 AND integration_id=$2`, session, id), owner, version)
		if err != nil {
			return OfficialAuthorization{}, err
		}
		if snapshot.state == "pending" && time.Now().Before(snapshot.expires) && snapshot.revision == c.Record.Revision {
			values, err = m.Repository.Vault.Open("open115-auth:"+session, snapshot.encrypted)
			if err != nil {
				return OfficialAuthorization{}, ErrAuthSession
			}
			timestamp, _ := strconv.ParseInt(values["time"], 10, 64)
			poll, err = m.officialClient().Poll(ctx, open115.Device{UID: values["uid"], Sign: values["sign"], Time: timestamp})
			if err != nil {
				return snapshot.response(session), err
			}
		}
	}

	// Re-read after the network request. A cancellation, replacement, credential
	// version change or integration edit must win over a stale successful poll.
	tx, c, stored, err := m.lockOfficialAuth(ctx, id, session, owner, version)
	if err != nil {
		return OfficialAuthorization{}, err
	}
	defer tx.Rollback(context.Background())
	result := stored.response(session)
	if stored.state != "pending" && stored.state != "exchanging" {
		return result, nil
	}
	if cancelled {
		return finishOfficialAuth(ctx, tx, result, "cancelled")
	}
	currentOwner, err := officialOwnerCurrent(ctx, tx, owner, version)
	if err != nil {
		return result, err
	}
	if !currentOwner || !time.Now().Before(stored.expires) || stored.revision != c.Record.Revision {
		return finishOfficialAuth(ctx, tx, result, "expired")
	}
	if stored.state == "exchanging" {
		return result, nil // another process has already claimed this one-shot code
	}
	if values == nil {
		// Only a new QR session can return to pending. Never exchange without a
		// verified status response for this same session and revision.
		return result, ErrAuthSession
	}
	if !poll.Valid {
		return finishOfficialAuth(ctx, tx, result, "expired")
	}
	if poll.Status != 2 {
		result.State = "waiting"
		if poll.Status == 1 {
			result.State = "scanned"
		}
		return result, nil
	}

	// Persist the claim BEFORE the external one-shot exchange. Erasing the
	// verifier here makes a crash/lost response non-replayable. The request that
	// wins the claim retains it only in memory; other processes return exchanging.
	if _, err = finishOfficialAuth(ctx, tx, result, "exchanging"); err != nil {
		return result, err
	}
	fresh, exchangeErr := m.officialClient().Exchange(ctx, values["uid"], values["verifier"])
	return m.completeOfficialAuth(ctx, id, session, owner, version, fresh, exchangeErr)
}

type officialAuthSession struct {
	state     string
	revision  int64
	encrypted []byte
	expires   time.Time
}

const officialAuthColumns = `owner_account_id::text,owner_version,integration_revision,state,payload_ciphertext,expires_at`

func scanOfficialAuth(row pgx.Row, owner string, version int64) (officialAuthSession, error) {
	var s officialAuthSession
	var storedOwner string
	var storedVersion int64
	err := row.Scan(&storedOwner, &storedVersion, &s.revision, &s.state, &s.encrypted, &s.expires)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && (storedOwner != owner || storedVersion != version) {
		return s, ErrAuthSession
	}
	if err != nil {
		return s, ErrUnavailable
	}
	return s, nil
}

func (s officialAuthSession) response(session string) OfficialAuthorization {
	return OfficialAuthorization{SessionID: session, State: s.state, ExpiresAt: s.expires}
}

func (m *Manager) lockOfficialAuth(ctx context.Context, id, session, owner string, version int64) (pgx.Tx, Credentials, officialAuthSession, error) {
	tx, err := m.Repository.DB.Begin(ctx)
	if err != nil {
		return nil, Credentials{}, officialAuthSession{}, ErrUnavailable
	}
	// Always integration -> session lock order, matching Start/Save/refresh.
	c, err := m.Repository.scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM integrations WHERE id=$1 FOR UPDATE`, id))
	var s officialAuthSession
	if err == nil && c.Record.Kind != "open115" {
		err = ErrAuthSession
	}
	if err == nil {
		s, err = scanOfficialAuth(tx.QueryRow(ctx, `SELECT `+officialAuthColumns+` FROM open115_authorizations WHERE id=$1 AND integration_id=$2 FOR UPDATE`, session, id), owner, version)
	}
	if err != nil {
		tx.Rollback(context.Background())
		return nil, c, s, err
	}
	return tx, c, s, nil
}

func officialOwnerCurrent(ctx context.Context, tx pgx.Tx, owner string, version int64) (bool, error) {
	var current int64
	err := tx.QueryRow(ctx, `SELECT credential_version FROM administrator_accounts WHERE id=$1 FOR SHARE`, owner).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, ErrUnavailable
	}
	return current == version, nil
}

func finishOfficialAuth(ctx context.Context, tx pgx.Tx, result OfficialAuthorization, state string) (OfficialAuthorization, error) {
	if _, err := tx.Exec(ctx, `UPDATE open115_authorizations SET state=$2,payload_ciphertext='\x'::bytea WHERE id=$1`, result.SessionID, state); err != nil {
		return result, ErrUnavailable
	}
	if err := tx.Commit(ctx); err != nil {
		return result, ErrUnavailable
	}
	result.State = state
	return result, nil
}

func (m *Manager) completeOfficialAuth(ctx context.Context, id, session, owner string, version int64, fresh open115.Tokens, exchangeErr error) (OfficialAuthorization, error) {
	// Persist a known outcome even if the browser disconnected. Explicit cancel
	// and account/config changes are rechecked; they must not be overwritten.
	durable, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer stop()
	tx, c, stored, err := m.lockOfficialAuth(durable, id, session, owner, version)
	if err != nil {
		return OfficialAuthorization{}, err
	}
	defer tx.Rollback(context.Background())
	result := stored.response(session)
	if stored.state != "exchanging" {
		return result, nil
	}
	currentOwner, err := officialOwnerCurrent(durable, tx, owner, version)
	if err != nil {
		return result, err
	}
	if !currentOwner || !time.Now().Before(stored.expires) || stored.revision != c.Record.Revision {
		return finishOfficialAuth(durable, tx, result, "expired")
	}
	if exchangeErr != nil {
		if _, saveErr := finishOfficialAuth(durable, tx, result, "failed"); saveErr != nil {
			return result, saveErr
		}
		result.State = "failed"
		return result, exchangeErr
	}
	for key, value := range map[string]string{"accessToken": fresh.AccessToken, "refreshToken": fresh.RefreshToken, "accessExpiresAt": time.Now().Add(time.Duration(fresh.ExpiresIn) * time.Second).UTC().Format(time.RFC3339Nano)} {
		c.Values[key] = value
	}
	delete(c.Values, "refreshBlocked")
	delete(c.Values, "refreshAfter")
	tokens, err := m.Repository.Vault.Seal(id, c.Values)
	if err != nil {
		return result, ErrUnavailable
	}
	// No automatic provider enable: the user explicitly selects official playback.
	if _, err = tx.Exec(durable, `UPDATE integrations SET credentials_ciphertext=$2,revision=revision+1,last_check_state='unchecked',last_check_message='115扫码授权已保存；请验证并启用官方直链',updated_at=now() WHERE id=$1`, id, tokens); err != nil {
		return result, ErrUnavailable
	}
	details, _ := json.Marshal(map[string]string{"integrationId": id})
	if _, err = tx.Exec(durable, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','open115.authorize',$1)`, details); err != nil {
		return result, ErrUnavailable
	}
	return finishOfficialAuth(durable, tx, result, "authorized")
}
