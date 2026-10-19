package v2

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/jackc/pgx/v5"
	"media-tool/internal/platform/pluginstate"
	"slices"
	"time"
)

// Metadata is a read-only projection, never a repository, version selector or code source.
type Metadata struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Version     string                 `json:"version"`
	Author      string                 `json:"author"`
	Icon        string                 `json:"icon"`
	Level       int                    `json:"level"`
	Description string                 `json:"description"`
	History     map[string]string      `json:"history"`
	Permissions []contracts.Permission `json:"permissions"`
}

func metadata(m contracts.Manifest) Metadata {
	return Metadata{m.ID, m.Name, m.Version, m.Author, m.Icon, m.Level, m.Description, m.History, m.Permissions}
}

type reviewedScope struct {
	Scope        string `json:"scope"`
	SelectorHash string `json:"selectorHash"`
}
type approvalMetadata struct {
	Version string          `json:"version"`
	Scopes  []reviewedScope `json:"scopes"`
}
type ReviewPlan struct {
	Instance        Instance     `json:"instance"`
	Current         Metadata     `json:"current"`
	Availability    Availability `json:"availability"`
	Fingerprint     string       `json:"fingerprint"`
	ApprovedVersion string       `json:"approvedVersion"`
	BaselineKnown   bool         `json:"baselineKnown"`
	ApprovedScopes  []string     `json:"approvedScopes"`
	Added           []string     `json:"added"`
	Removed         []string     `json:"removed"`
	Changed         []string     `json:"changed"`
	NeedsReview     bool         `json:"needsReview"`
}

func approvalKey(id string) string               { return "plugins.review.approved." + id }
func selectorHash(p contracts.Permission) string { b, _ := json.Marshal(p.Resources); return digest(b) }

// Only version and selector digests are retained; no config/KV, code or credentials.
func saveApprovalMetadata(ctx context.Context, tx pgx.Tx, id string, m contracts.Manifest, replace bool) error {
	var old approvalMetadata
	if e := pluginstate.Lock(ctx, tx, approvalKey(id), &old); e != nil {
		return e
	}
	if !replace && old.Version != "" {
		return nil
	}
	v := approvalMetadata{Version: m.Version, Scopes: []reviewedScope{}}
	for _, p := range m.Permissions {
		v.Scopes = append(v.Scopes, reviewedScope{p.Scope, selectorHash(p)})
	}
	return pluginstate.Put(ctx, tx, approvalKey(id), v)
}

// Adopt an old approved definition before refresh overwrites it. Pending reviews
// are never guessed from the new manifest. Instance lock precedes framework KV.
func captureApprovalMetadata(ctx context.Context, tx pgx.Tx, plugin, fp string, m contracts.Manifest, replace bool) error {
	rows, e := tx.Query(ctx, `SELECT id::text FROM plugin_v2_instances WHERE plugin_id=$1 AND approved_fingerprint=$2 AND desired_state<>'absent' ORDER BY id FOR NO KEY UPDATE`, plugin, fp)
	if e != nil {
		return e
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			break
		}
		ids = append(ids, id)
	}
	other := rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	if other != nil {
		return other
	}
	for _, id := range ids {
		if e = saveApprovalMetadata(ctx, tx, id, m, replace); e != nil {
			return e
		}
	}
	return nil
}
func (s Store) InstalledCounts(ctx context.Context, ids []string) (map[string]int, error) {
	out := map[string]int{}
	if len(ids) > 200 {
		return nil, ErrInvalid
	}
	rows, e := s.DB.Query(ctx, `SELECT plugin_id,count(*) FROM plugin_v2_instances WHERE desired_state<>'absent' AND plugin_id=ANY($1::text[]) GROUP BY plugin_id LIMIT 200`, ids)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var count int
		if e = rows.Scan(&id, &count); e != nil {
			return nil, e
		}
		out[id] = count
	}
	return out, rows.Err()
}
func (s Store) ReviewPlan(ctx context.Context, id string) (ReviewPlan, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out := ReviewPlan{ApprovedScopes: []string{}, Added: []string{}, Removed: []string{}, Changed: []string{}}
	if !validUUID(id) {
		return out, ErrInvalid
	}
	tx, e := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return out, e
	}
	defer rollback(ctx, tx)
	out.Instance, e = readInstance(ctx, tx, id)
	if e != nil {
		return out, e
	}
	var raw []byte
	var approved string
	e = tx.QueryRow(ctx, `SELECT d.manifest,d.fingerprint,d.availability,i.approved_fingerprint FROM plugin_v2_instances i JOIN plugin_v2_definitions d ON d.id=i.plugin_id WHERE i.id=$1`, id).Scan(&raw, &out.Fingerprint, &out.Availability, &approved)
	if e != nil {
		return out, e
	}
	var m contracts.Manifest
	if json.Unmarshal(raw, &m) != nil {
		return out, ErrUnavailable
	}
	out.Current = metadata(m)
	// A stale/missing compiled definition cannot be reviewed or activated.
	current, exists := definition(m.ID)
	if errors.Is(exists, ErrNotFound) {
		out.Availability = Missing
	} else if exists != nil {
		return out, exists
	} else if current.Fingerprint != out.Fingerprint {
		return out, ErrConflict
	}
	rows, e := tx.Query(ctx, `SELECT scope FROM plugin_v2_grants WHERE instance_id=$1 AND fingerprint=$2 ORDER BY scope LIMIT 200`, id, approved)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var scope string
		if e = rows.Scan(&scope); e != nil {
			break
		}
		out.ApprovedScopes = append(out.ApprovedScopes, scope)
	}
	other := rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	if other != nil {
		return out, other
	}
	var base approvalMetadata
	e = tx.QueryRow(ctx, `SELECT value FROM app_settings WHERE key=$1`, approvalKey(id)).Scan(&raw)
	if e == nil {
		if json.Unmarshal(raw, &base) != nil {
			return out, ErrUnavailable
		}
		out.ApprovedVersion = base.Version
		out.BaselineKnown = base.Version != ""
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return out, e
	}
	out.NeedsReview = approved != out.Fingerprint || out.Instance.Reason == PermissionReview || out.Availability != Available
	if out.Availability == Available {
		before := map[string]string{}
		for _, p := range base.Scopes {
			before[p.Scope] = p.SelectorHash
		}
		for _, p := range m.Permissions {
			if !slices.Contains(out.ApprovedScopes, p.Scope) {
				out.Added = append(out.Added, p.Scope)
			} else if hash, ok := before[p.Scope]; ok && hash != selectorHash(p) {
				out.Changed = append(out.Changed, p.Scope)
			}
		}
		for _, p := range out.ApprovedScopes {
			found := false
			for _, next := range m.Permissions {
				found = found || next.Scope == p
			}
			if !found {
				out.Removed = append(out.Removed, p)
			}
		}
	}
	out.NeedsReview = out.NeedsReview || len(out.Added)+len(out.Removed)+len(out.Changed) > 0
	slices.Sort(out.Added)
	slices.Sort(out.Removed)
	slices.Sort(out.Changed)
	return out, nil
}
