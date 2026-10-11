package search

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/sites"
)

var ErrSnapshotUnavailable = errors.New("搜索结果缺少有效快照，请重新搜索或重新评估订阅")
var ErrSnapshotChanged = errors.New("站点配置或搜索快照已变化，请重新搜索或重新评估订阅")
var ErrSnapshotExpired = errors.New("搜索快照已过期，请重新搜索或重新评估订阅")

const SnapshotMaxAge = 10 * time.Minute
const snapshotAAD = "search-run:"

type runSnapshot struct {
	ID, DefinitionFingerprint string
	Resource                  Resource
	Hint                      []byte
	AccountRevision           int64
	ObservedAt                time.Time
}

type rowQueryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// A credential edit and a definition reimport both invalidate the source proof.
// Only a fingerprint is persisted: never duplicate cookies/API keys/config data.
func sourceProof(ctx context.Context, q rowQueryer, accountID string) (int64, string, error) {
	var revision int64
	var definitionID string
	if err := q.QueryRow(ctx, `SELECT revision,coalesce(settings->>'definitionId','') FROM integrations WHERE id=$1 AND kind='ptsite' AND enabled FOR SHARE`, accountID).Scan(&revision, &definitionID); err != nil {
		return 0, "", ErrSnapshotChanged
	}
	var definition, common []byte
	var commit string
	err := q.QueryRow(ctx, `SELECT definition,common_definition,source_commit FROM site_definitions WHERE id=$1 FOR SHARE`, definitionID).Scan(&definition, &common, &commit)
	if errors.Is(err, pgx.ErrNoRows) {
		definition, common = []byte("null"), []byte("null")
	} else if err != nil {
		return 0, "", ErrSnapshotChanged
	}
	return revision, sites.DefinitionFingerprint(definition, common, commit), nil
}

func decodeRunSnapshot(row pgx.Row) (runSnapshot, error) {
	var s runSnapshot
	var raw []byte
	var revision *int64
	var fingerprint *string
	var observed *time.Time
	if err := row.Scan(&s.ID, &raw, &s.Hint, &revision, &fingerprint, &observed); err != nil {
		return s, ErrSnapshotUnavailable
	}
	if len(raw) == 0 || len(s.Hint) == 0 || revision == nil || fingerprint == nil || observed == nil || json.Unmarshal(raw, &s.Resource) != nil {
		return s, ErrSnapshotUnavailable
	}
	s.AccountRevision, s.DefinitionFingerprint, s.ObservedAt = *revision, *fingerprint, *observed
	return s, nil
}

func (e Engine) snapshot(ctx context.Context, runID, resourceID string) (runSnapshot, error) {
	s, err := decodeRunSnapshot(e.DB.QueryRow(ctx, `SELECT id::text,public_snapshot,download_hint_ciphertext,account_revision,definition_fingerprint,observed_at FROM search_run_resources WHERE run_id=$1 AND resource_id=$2`, runID, resourceID))
	if err != nil {
		return s, err
	}
	if s.Resource.ID != resourceID || s.Resource.AccountID == "" || s.Resource.TorrentID == "" {
		return s, ErrSnapshotUnavailable
	}
	return s, nil
}

// ValidateRunResource does not acquire a temporary torrent URL or reveal hints.
func (e Engine) ValidateRunResource(ctx context.Context, runID, resourceID string) error {
	return e.validateRunResourceWith(ctx, e.DB, runID, resourceID)
}

func (e Engine) ValidateRunResourceTx(ctx context.Context, tx pgx.Tx, runID, resourceID string) error {
	var lockedRun string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM search_runs WHERE id=$1 FOR SHARE`, runID).Scan(&lockedRun); err != nil {
		return ErrSnapshotUnavailable
	}
	return e.validateRunResourceWith(ctx, tx, runID, resourceID)
}

func (e Engine) validateRunResourceWith(ctx context.Context, q rowQueryer, runID, resourceID string) error {
	s, err := decodeRunSnapshot(q.QueryRow(ctx, `SELECT id::text,public_snapshot,download_hint_ciphertext,account_revision,definition_fingerprint,observed_at FROM search_run_resources WHERE run_id=$1 AND resource_id=$2`, runID, resourceID))
	if err != nil {
		return err
	}
	if s.Resource.ID != resourceID || s.Resource.AccountID == "" || s.Resource.TorrentID == "" {
		return ErrSnapshotUnavailable
	}
	if time.Until(s.ObservedAt) > time.Minute || time.Since(s.ObservedAt) > SnapshotMaxAge {
		return ErrSnapshotExpired
	}
	revision, fingerprint, err := sourceProof(ctx, q, s.Resource.AccountID)
	if err != nil || revision != s.AccountRevision || fingerprint != s.DefinitionFingerprint {
		return ErrSnapshotChanged
	}
	return nil
}

// TorrentFromRun uses the encrypted hint belonging to this exact result, never
// resources.download_hint_ciphertext overwritten by another concurrent search.
func (e Engine) TorrentFromRun(ctx context.Context, runID, resourceID string) ([]byte, error) {
	if err := e.ValidateRunResource(ctx, runID, resourceID); err != nil {
		return nil, err
	}
	s, err := e.snapshot(ctx, runID, resourceID)
	if err != nil {
		return nil, err
	}
	hint, err := e.Vault.Open(snapshotAAD+s.ID, s.Hint)
	if err != nil {
		return nil, ErrSnapshotUnavailable
	}
	client, err := e.Connections.Site(ctx, s.Resource.AccountID)
	if err != nil {
		return nil, err
	}
	if client.SourceRevision != s.AccountRevision || client.DefinitionFingerprint != s.DefinitionFingerprint {
		return nil, ErrSnapshotChanged
	}
	return client.Torrent(ctx, s.Resource.TorrentID, hint["url"])
}

func publicResource(item sites.Resource, id, account, name string) Resource {
	metadata := map[string]any{}
	if item.HR != nil {
		metadata["hr"] = *item.HR
	}
	if item.Subtitle != "" {
		metadata["subtitle"] = item.Subtitle
	}
	if len(item.Labels) > 0 {
		metadata["labels"] = item.Labels
	}
	if detail := publicURL(item.DetailsURL); detail != "" {
		metadata["detailsUrl"] = detail
	}
	return Resource{ID: id, AccountID: account, SiteName: name, TorrentID: item.TorrentID, Title: item.Title, Size: max(0, item.Size), Seeders: max(0, item.Seeders), Leechers: max(0, item.Leechers), DownloadFactor: item.DownloadFactor, Metadata: metadata}
}

func mergePartialResource(base Resource, incoming sites.Resource) Resource {
	result := publicResource(incoming, base.ID, base.AccountID, base.SiteName)
	if incoming.Size <= 0 {
		result.Size = base.Size
	}
	if incoming.Seeders < 0 {
		result.Seeders = base.Seeders
	}
	if incoming.Leechers < 0 {
		result.Leechers = base.Leechers
	}
	if incoming.DownloadFactor == nil {
		result.DownloadFactor = base.DownloadFactor
	}
	for key, value := range base.Metadata {
		if _, exists := result.Metadata[key]; !exists {
			result.Metadata[key] = value
		}
	}
	return result
}

func (e Engine) saveResourceTx(ctx context.Context, tx pgx.Tx, runID, accountID string, item sites.Resource) error {
	var id string
	err := tx.QueryRow(ctx, `SELECT id::text FROM resources WHERE account_id=$1 AND torrent_id=$2 FOR UPDATE`, accountID, item.TorrentID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		id = jobs.UUID()
		hint, sealErr := e.Vault.Seal(id, map[string]string{"url": item.DownloadURL})
		if sealErr != nil {
			return sealErr
		}
		tag, insertErr := tx.Exec(ctx, `INSERT INTO resources(id,account_id,torrent_id,title,download_hint_ciphertext)VALUES($1,$2,$3,$4,$5)ON CONFLICT(account_id,torrent_id)DO NOTHING`, id, accountID, item.TorrentID, item.Title, hint)
		if insertErr != nil {
			return insertErr
		}
		if tag.RowsAffected() == 0 {
			if err = tx.QueryRow(ctx, `SELECT id::text FROM resources WHERE account_id=$1 AND torrent_id=$2 FOR UPDATE`, accountID, item.TorrentID).Scan(&id); err != nil {
				return err
			}
		}
	} else if err != nil {
		return err
	}
	var name string
	if err = tx.QueryRow(ctx, `SELECT name FROM integrations WHERE id=$1`, accountID).Scan(&name); err != nil {
		return err
	}
	revision, fingerprint, err := sourceProof(ctx, tx, accountID)
	if err != nil {
		return err
	}
	snapshotID, observed := jobs.UUID(), time.Now().UTC()
	public := publicResource(item, id, accountID, name)
	downloadURL := item.DownloadURL
	base, baseErr := decodeRunSnapshot(tx.QueryRow(ctx, `SELECT id::text,public_snapshot,download_hint_ciphertext,account_revision,definition_fingerprint,observed_at FROM search_run_resources WHERE run_id=$1 AND resource_id=$2 FOR UPDATE`, runID, id))
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM search_run_resources WHERE run_id=$1 AND resource_id=$2)`, runID, id).Scan(&exists); err != nil {
		return err
	}
	if exists {
		if baseErr != nil {
			return ErrSnapshotUnavailable
		}
		snapshotID = base.ID
		if item.Partial {
			if base.AccountRevision != revision || base.DefinitionFingerprint != fingerprint {
				return ErrSnapshotChanged
			}
			public, observed = mergePartialResource(base.Resource, item), base.ObservedAt
		}
		if downloadURL == "" {
			previous, openErr := e.Vault.Open(snapshotAAD+base.ID, base.Hint)
			if openErr != nil {
				return ErrSnapshotUnavailable
			}
			downloadURL = previous["url"]
		}
	}
	// Missing hints in a new run stay missing. Do not silently recover a prior
	// account/run's expired signed URL; API-token sites generate their own URL.
	hint, err := e.Vault.Seal(snapshotAAD+snapshotID, map[string]string{"url": downloadURL})
	if err != nil {
		return err
	}
	rawPublic, err := json.Marshal(public)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO search_run_resources(id,run_id,resource_id,public_snapshot,download_hint_ciphertext,account_revision,definition_fingerprint,observed_at)VALUES($1,$2,$3,$4,$5,$6,$7,$8)ON CONFLICT(run_id,resource_id)DO UPDATE SET public_snapshot=EXCLUDED.public_snapshot,download_hint_ciphertext=EXCLUDED.download_hint_ciphertext,account_revision=EXCLUDED.account_revision,definition_fingerprint=EXCLUDED.definition_fingerprint,observed_at=EXCLUDED.observed_at`, snapshotID, runID, id, rawPublic, hint, revision, fingerprint, observed); err != nil {
		return err
	}
	// Keep the global identity index/latest projection for legacy manual tasks;
	// current evaluations, previews and download jobs read their run snapshot.
	globalHint, err := e.Vault.Seal(id, map[string]string{"url": downloadURL})
	if err != nil {
		return err
	}
	rawMetadata, _ := json.Marshal(public.Metadata)
	_, err = tx.Exec(ctx, `UPDATE resources SET title=$2,size_bytes=$3,seeders=$4,leechers=$5,download_factor=$6,public_metadata=$7,download_hint_ciphertext=$8,updated_at=now()WHERE id=$1`, id, public.Title, public.Size, public.Seeders, public.Leechers, public.DownloadFactor, rawMetadata, globalHint)
	return err
}

// Validate every recorded collection proof, including empty RSS feeds whose
// aggregate site status retains an earlier search failure. A UI warning is not
// evidence that no successful HTTP collection occurred.
func (e Engine) ValidateRunSources(ctx context.Context, runID string) error {
	return e.validateRunSourcesWith(ctx, e.DB, runID)
}

// ValidateRunSourcesTx keeps the run, collection proofs, account revisions and
// definition rows locked until the caller commits its source-dependent facts.
func (e Engine) ValidateRunSourcesTx(ctx context.Context, tx pgx.Tx, runID string) error {
	var lockedRun string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM search_runs WHERE id=$1 FOR SHARE`, runID).Scan(&lockedRun); err != nil {
		return ErrSnapshotUnavailable
	}
	return e.validateRunSourcesWith(ctx, tx, runID)
}

type sourceQueryer interface {
	rowQueryer
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func (e Engine) validateRunSourcesWith(ctx context.Context, q sourceQueryer, runID string) error {
	rows, err := q.Query(ctx, `SELECT account_id::text,source_revision,definition_fingerprint,observed_at FROM search_site_results WHERE run_id=$1 AND (state IN('succeeded','partial') OR source_revision IS NOT NULL OR definition_fingerprint IS NOT NULL OR observed_at IS NOT NULL) ORDER BY account_id FOR SHARE`, runID)
	if err != nil {
		return err
	}
	type proof struct {
		account     string
		revision    *int64
		fingerprint *string
		observed    *time.Time
	}
	var proofs []proof
	for rows.Next() {
		var p proof
		if err = rows.Scan(&p.account, &p.revision, &p.fingerprint, &p.observed); err != nil {
			rows.Close()
			return err
		}
		proofs = append(proofs, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, p := range proofs {
		if p.revision == nil || p.fingerprint == nil || p.observed == nil {
			return ErrSnapshotUnavailable
		}
		if time.Until(*p.observed) > time.Minute || time.Since(*p.observed) > SnapshotMaxAge {
			return ErrSnapshotExpired
		}
		revision, fingerprint, err := sourceProof(ctx, q, p.account)
		if err != nil || revision != *p.revision || fingerprint != *p.fingerprint {
			return ErrSnapshotChanged
		}
	}
	return nil
}

func (e Engine) LatestRunForResource(ctx context.Context, resourceID string) (string, error) {
	var runID string
	if err := e.DB.QueryRow(ctx, `SELECT run_id::text FROM search_run_resources WHERE resource_id=$1 AND public_snapshot IS NOT NULL ORDER BY observed_at DESC,id DESC LIMIT 1`, resourceID).Scan(&runID); err != nil {
		return "", ErrSnapshotUnavailable
	}
	return runID, nil
}

// RunResource returns public evidence only. Signed URLs/cookies are never copied
// into preview DTOs or approval digests.
func (e Engine) RunResource(ctx context.Context, runID, resourceID string) (Resource, error) {
	s, err := e.snapshot(ctx, runID, resourceID)
	return s.Resource, err
}

// SourceSnapshotDigest identifies the exact persisted hint, metadata and origin
// proof, without returning its plaintext. Replacing a same-run snapshot changes it.
func (e Engine) SourceSnapshotDigest(ctx context.Context, runID, resourceID string) (string, error) {
	s, err := e.snapshot(ctx, runID, resourceID)
	if err != nil {
		return "", err
	}
	hint, err := e.Vault.Open(snapshotAAD+s.ID, s.Hint)
	if err != nil {
		return "", ErrSnapshotUnavailable
	}
	// Bind semantic evidence, not randomized encryption bytes. Offline rekey
	// retains URL/ID/AAD and must not invent a changed-source approval problem.
	proof := struct {
		ID                    string
		Resource              Resource
		Hint                  map[string]string
		AccountRevision       int64
		DefinitionFingerprint string
		ObservedAt            time.Time
	}{s.ID, s.Resource, hint, s.AccountRevision, s.DefinitionFingerprint, s.ObservedAt}
	raw, err := json.Marshal(proof)
	if err != nil {
		return "", ErrSnapshotUnavailable
	}
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:]), nil
}
