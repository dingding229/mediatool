package playbackgrants

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/storage"
	"media-tool/internal/modules/strm"
	"media-tool/internal/platform/secrets"
)

var ErrDenied = errors.New("file playback credential denied")

type Service struct {
	DB        *pgxpool.Pool
	Vault     *secrets.Vault
	PublicURL string
}
type Ref struct{ AssetID, StorageID, CloudPath string }

func (s Service) Prepare(ctx context.Context, tx pgx.Tx, assetID, cloudPath string) (string, string, error) {
	var id string
	var encrypted []byte
	var revoked bool
	err := tx.QueryRow(ctx, `SELECT id::text,token_ciphertext,revoked_at IS NOT NULL FROM playback_grants WHERE asset_id=$1 FOR UPDATE`, assetID).Scan(&id, &encrypted, &revoked)
	if err == nil && revoked {
		return "", "", ErrDenied
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", "", err
	}
	var token string
	if id == "" {
		id = jobs.UUID()
		data := make([]byte, 32)
		if _, err = rand.Read(data); err != nil {
			return "", "", err
		}
		token = hex.EncodeToString(data)
		digest := sha256.Sum256([]byte(token))
		encrypted, err = s.Vault.Seal(id, map[string]string{"token": token})
		if err != nil {
			return "", "", err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO playback_grants(id,asset_id,token_digest,token_ciphertext)VALUES($1,$2,$3,$4)`, id, assetID, digest[:], encrypted); err != nil {
			return "", "", err
		}
	} else {
		values, err := s.Vault.Open(id, encrypted)
		if err != nil {
			return "", "", err
		}
		token = values["token"]
	}
	link, err := s.build(assetID, token, cloudPath)
	return link, id, err
}
func (s Service) build(assetID, token, cloudPath string) (string, error) {
	builder := strm.Builder{BaseURL: s.PublicURL, APIKey: token, AllowedRoots: []string{"/"}}
	link, err := builder.Build(storage.FileRef{Path: cloudPath})
	if err != nil {
		return "", err
	}
	u, _ := url.Parse(link)
	q := u.Query()
	q.Set("assetId", assetID)
	u.RawQuery = q.Encode()
	return u.String(), nil
}
func (s Service) Authorize(ctx context.Context, assetID, token string) (Ref, error) {
	var ref Ref
	var digest []byte
	var state string
	err := s.DB.QueryRow(ctx, `SELECT a.id::text,f.storage_id::text,f.full_path,f.state,g.token_digest FROM media_assets a JOIN cloud_files f ON f.id=a.cloud_file_id JOIN playback_grants g ON g.asset_id=a.id WHERE a.id=$1 AND g.revoked_at IS NULL`, assetID).Scan(&ref.AssetID, &ref.StorageID, &ref.CloudPath, &state, &digest)
	if err != nil || state != "present" {
		return ref, ErrDenied
	}
	candidate := sha256.Sum256([]byte(token))
	if len(digest) != len(candidate) || subtle.ConstantTimeCompare(digest, candidate[:]) != 1 {
		return ref, ErrDenied
	}
	return ref, nil
}
func (s Service) VerifyManagedURL(ctx context.Context, raw string) (storage.FileRef, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return storage.FileRef{}, ErrDenied
	}
	q := u.Query()
	if q.Get("assetId") == "" || q.Get("apiKey") == "" {
		return storage.FileRef{}, ErrDenied
	}
	ref, err := s.Authorize(ctx, q.Get("assetId"), q.Get("apiKey"))
	if err != nil {
		return storage.FileRef{}, err
	}
	return storage.FileRef{Path: ref.CloudPath, AssetID: ref.AssetID}, nil
}
func (s Service) Revoke(ctx context.Context, assetID string) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE playback_grants SET revoked_at=now(),updated_at=now()WHERE asset_id=$1`, assetID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','playback.revoke',jsonb_build_object('assetId',$1::text))`, assetID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s Service) Rotate(ctx context.Context, assetID string) (string, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	link, err := s.RotateTx(ctx, tx, assetID)
	if err != nil {
		return "", err
	}
	if err = tx.Commit(ctx); err != nil {
		return "", err
	}
	return link, nil
}
func (s Service) RotateTx(ctx context.Context, tx pgx.Tx, assetID string) (string, error) {
	var err error
	var id, cloudPath string
	if err = tx.QueryRow(ctx, `SELECT g.id::text,f.full_path FROM playback_grants g JOIN media_assets a ON a.id=g.asset_id JOIN cloud_files f ON f.id=a.cloud_file_id WHERE g.asset_id=$1 FOR UPDATE OF g`, assetID).Scan(&id, &cloudPath); err != nil {
		return "", err
	}
	data := make([]byte, 32)
	if _, err = rand.Read(data); err != nil {
		return "", err
	}
	token := hex.EncodeToString(data)
	digest := sha256.Sum256([]byte(token))
	encrypted, err := s.Vault.Seal(id, map[string]string{"token": token})
	if err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `UPDATE playback_grants SET token_digest=$2,token_ciphertext=$3,revoked_at=NULL,generation=generation+1,updated_at=now()WHERE id=$1`, id, digest[:], encrypted); err != nil {
		return "", err
	}
	return s.build(assetID, token, cloudPath)
}
func (s Service) ExistingURL(ctx context.Context, assetID string) (string, error) {
	var encrypted []byte
	var id, cloudPath string
	var revoked bool
	if err := s.DB.QueryRow(ctx, `SELECT g.id::text,g.token_ciphertext,f.full_path,g.revoked_at IS NOT NULL FROM playback_grants g JOIN media_assets a ON a.id=g.asset_id JOIN cloud_files f ON f.id=a.cloud_file_id WHERE a.id=$1`, assetID).Scan(&id, &encrypted, &cloudPath, &revoked); err != nil {
		return "", err
	}
	if revoked {
		return "", ErrDenied
	}
	values, err := s.Vault.Open(id, encrypted)
	if err != nil {
		return "", err
	}
	return s.build(assetID, values["token"], cloudPath)
}
func SecretFreeKeyRef(id string) string { return "grant:" + strings.TrimSpace(id) }

// ExistingBody never creates/rotates a grant. The caller must compare its SHA256
// with the owned original STRM manifest before using it for recovery.
func (s Service) ExistingBody(ctx context.Context, tx pgx.Tx, asset string) ([]byte, error) {
	var id, p string
	var raw, digest []byte
	var revoked bool
	if err := tx.QueryRow(ctx, `SELECT g.id::text,g.token_ciphertext,g.token_digest,g.revoked_at IS NOT NULL,f.full_path FROM playback_grants g JOIN media_assets a ON a.id=g.asset_id JOIN cloud_files f ON f.id=a.cloud_file_id WHERE g.asset_id=$1`, asset).Scan(&id, &raw, &digest, &revoked, &p); err != nil || revoked {
		return nil, ErrDenied
	}
	values, err := s.Vault.Open(id, raw)
	if err != nil {
		return nil, err
	}
	token := values["token"]
	sum := sha256.Sum256([]byte(token))
	if subtle.ConstantTimeCompare(sum[:], digest) != 1 {
		return nil, ErrDenied
	}
	link, err := s.build(asset, token, p)
	if err != nil {
		return nil, err
	}
	return []byte(link + "\n"), nil
}
