package v2

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	b "github.com/dingding229/mediatool-plugin/sdk/host/business"
	"github.com/jackc/pgx/v5"
	"reflect"
	"sync"
	"time"
)

const supportedContractVersion = 2

// SelfCheck is metadata-only: no factories, network or instance allocation.
//
//go:embed library.json
var libraryInventory json.RawMessage

func Resources() json.RawMessage { return append(json.RawMessage{}, libraryInventory...) }

func SelfCheck() error {
	if contracts.ContractVersion != supportedContractVersion {
		return ErrUnavailable
	}
	defs, e := Catalog()
	if e != nil {
		return e
	}
	var inventory struct {
		HostMethods     []b.Method        `json:"hostMethods"`
		Pin             string            `json:"pin"`
		ContractVersion int               `json:"contractVersion"`
		Manifests       []json.RawMessage `json:"manifests"`
	}
	if json.Unmarshal(libraryInventory, &inventory) != nil || !reflect.DeepEqual(inventory.HostMethods, b.Methods()) || inventory.Pin != BuildID() || inventory.ContractVersion != contracts.ContractVersion || len(inventory.Manifests) != len(defs) {
		return ErrUnavailable
	}
	for i, d := range defs {
		m, e := contracts.ParseManifest(inventory.Manifests[i])
		if e != nil || !reflect.DeepEqual(m, d.Manifest) {
			return ErrUnavailable
		}
	}
	return nil
}

// Refresh only visits definitions selected by an administrator. The compiled
// catalog is never persisted wholesale. Locks and revisions serialize publishers.
func (s Store) Refresh(ctx context.Context) error {
	defer invalidateNameHooks(s.DB)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	defs, e := Catalog()
	if e != nil {
		return e
	}
	return s.refresh(ctx, defs)
}
func (s Store) refresh(ctx context.Context, defs []Definition) error {
	compiled := map[string]Definition{}
	for _, d := range defs {
		compiled[d.Manifest.ID] = d
	}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer rollback(ctx, tx)
	// Match install/clone's metadata lock order before touching definition/instance rows.
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(7062001)`); e != nil {
		return e
	}
	rows, e := tx.Query(ctx, `SELECT id,manifest,fingerprint,availability,revision FROM plugin_v2_definitions ORDER BY id FOR NO KEY UPDATE`)
	if e != nil {
		return e
	}
	type saved struct {
		id, fp       string
		raw          []byte
		availability Availability
		revision     int64
	}
	list := []saved{}
	for rows.Next() {
		var v saved
		if e = rows.Scan(&v.id, &v.raw, &v.fp, &v.availability, &v.revision); e != nil {
			break
		}
		list = append(list, v)
	}
	other := rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	if other != nil {
		return other
	}
	for _, old := range list {
		d, exists := compiled[old.id]
		availability, reason := Available, NoReason
		raw, fp := old.raw, old.fp
		review := false
		if !exists {
			availability, reason = Missing, DefinitionMissing
		} else {
			raw, e = json.Marshal(d.Manifest)
			if e != nil {
				return e
			}
			fp = d.Fingerprint
			var before contracts.Manifest
			if json.Unmarshal(old.raw, &before) != nil {
				return ErrUnavailable
			}
			review = requiresReview(before, d.Manifest)
		}

		// Legacy build identities only rebind silently when locked build-time source
		// evidence proves content equality; matching manifests alone is insufficient.
		if exists && old.availability == Available && legacyContent(old.fp) == fp {
			var before contracts.Manifest
			if json.Unmarshal(old.raw, &before) != nil {
				return ErrUnavailable
			}
			a, _ := canonicalManifest(before)
			b, _ := canonicalManifest(d.Manifest)
			if bytes.Equal(a, b) {
				if e = captureApprovalMetadata(ctx, tx, old.id, old.fp, before, false); e != nil {
					return e
				}
				if _, e = tx.Exec(ctx, `UPDATE plugin_v2_definitions SET fingerprint=$2 WHERE id=$1`, old.id, fp); e != nil {
					return e
				}
				if _, e = tx.Exec(ctx, `UPDATE plugin_v2_instances SET approved_fingerprint=$2 WHERE plugin_id=$1 AND approved_fingerprint=$3`, old.id, fp, old.fp); e != nil {
					return e
				}
				if _, e = tx.Exec(ctx, `UPDATE plugin_v2_grants g SET fingerprint=$2 FROM plugin_v2_instances i WHERE i.id=g.instance_id AND i.plugin_id=$1 AND g.fingerprint=$3`, old.id, fp, old.fp); e != nil {
					return e
				}
				continue
			}
		}
		var previous contracts.Manifest
		if json.Unmarshal(old.raw, &previous) != nil {
			return ErrUnavailable
		}
		if e = captureApprovalMetadata(ctx, tx, old.id, old.fp, previous, old.fp != fp || old.availability != availability); e != nil {
			return e
		}
		if old.fp == fp && old.availability == availability {
			continue
		}
		tag, e := tx.Exec(ctx, `UPDATE plugin_v2_definitions SET manifest=$2,fingerprint=$3,availability=$4,revision=revision+1 WHERE id=$1 AND revision=$5 AND fingerprint=$6`, old.id, raw, fp, availability, old.revision, old.fp)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return ErrConflict
		}
		if reason == NoReason && review {
			reason = PermissionReview
		}
		// A pending approval never disappears on a repeated sync or a later metadata bump.
		_, e = tx.Exec(ctx, `UPDATE plugin_v2_instances SET approved_fingerprint=CASE WHEN $3='' AND approved_fingerprint=$4 THEN $2 ELSE approved_fingerprint END, desired_state=CASE WHEN $3<>'' OR approved_fingerprint<>$4 THEN 'disabled' ELSE desired_state END, runtime_state=CASE WHEN generation IS NOT NULL THEN 'stopping' WHEN $3<>'' OR approved_fingerprint<>$4 THEN 'needs_action' ELSE 'stopped' END,reason=CASE WHEN $3<>'' THEN $3 WHEN approved_fingerprint<>$4 THEN 'permission_review_required' ELSE '' END,grant_revision=grant_revision+1,revision=revision+1,updated_at=now() WHERE plugin_id=$1 AND desired_state<>'absent'`, old.id, fp, reason, old.fp)
		if e != nil {
			return e
		}
		if reason == NoReason {
			if _, e = tx.Exec(ctx, `UPDATE plugin_v2_grants g SET fingerprint=$2 FROM plugin_v2_instances i WHERE i.id=g.instance_id AND i.plugin_id=$1 AND i.approved_fingerprint=$2`, old.id, fp); e != nil {
				return e
			}
		}
	}
	return tx.Commit(ctx)
}
func requiresReview(a, b contracts.Manifest) bool {
	// Conservative: any change in the execution surface, even a restriction,
	// requests fresh consent; release notes/version/name alone do not.
	return !reflect.DeepEqual(a.Permissions, b.Permissions) || !reflect.DeepEqual(a.Resources, b.Resources) || !reflect.DeepEqual(a.Extensions, b.Extensions) || !reflect.DeepEqual(a.UI, b.UI)
}
func reviewGrants(ctx context.Context, tx pgx.Tx, id string, d Definition) error {
	if _, e := tx.Exec(ctx, `DELETE FROM plugin_v2_grants WHERE instance_id=$1`, id); e != nil {
		return e
	}
	for _, scope := range scopes(d) {
		if _, e := tx.Exec(ctx, `INSERT INTO plugin_v2_grants(instance_id,scope,fingerprint)VALUES($1,$2,$3)`, id, scope, d.Fingerprint); e != nil {
			return e
		}
	}
	return nil
}

var legacyContentIndex = sync.OnceValue(func() map[string]string {
	var inventory struct {
		LegacyContents map[string]string `json:"legacyContents"`
	}
	if json.Unmarshal(libraryInventory, &inventory) != nil {
		return nil
	}
	return inventory.LegacyContents
})

func legacyContent(fp string) string { return legacyContentIndex()[fp] }
