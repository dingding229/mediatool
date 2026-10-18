package v2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	b "github.com/dingding229/mediatool-plugin/sdk/host/business"
	"github.com/jackc/pgx/v5"
	"media-tool/internal/modules/jobs"
	dbutil "media-tool/internal/platform/database"
	"net/url"
	"strings"
	"time"
)

type BusinessBridge interface {
	Call(context.Context, contracts.HostCall, contracts.Permission, func() error) (any, error)
}
type businessRecord struct {
	Request string          `json:"request"`
	Ready   bool            `json:"ready"`
	Result  json.RawMessage `json:"result,omitempty"`
	Code    contracts.Code  `json:"code,omitempty"`
}

// Reserved, encrypted instance KV is an operation receipt, never an alternative
// business database. Unknown outcomes do not replay, and records expire after 7d.
func (s Store) business(ctx context.Context, l Lease, call contracts.HostCall, p contracts.Permission) (any, error) {
	spec, _ := b.Lookup(call.Method)
	if !spec.Supported {
		return nil, contracts.NewError(contracts.Unsupported)
	}
	if s.Business == nil {
		return nil, contracts.NewError(contracts.Unavailable)
	}
	guard := func(c context.Context, tx pgx.Tx) error { return fenceHost(c, tx, l, p.Scope) }
	admit := func() error {
		tx, e := s.DB.Begin(ctx)
		if e != nil {
			return e
		}
		defer rollback(ctx, tx)
		if e = guard(ctx, tx); e != nil {
			return e
		}
		return tx.Commit(ctx)
	}
	if spec.Level == "R" {
		out, e := s.Business.Call(ctx, call, p, admit)
		if e != nil {
			return nil, e
		}
		var current bool
		e = s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM plugin_v2_instances i JOIN plugin_v2_definitions d ON d.id=i.plugin_id JOIN plugin_v2_grants g ON g.instance_id=i.id AND g.scope=$6 AND g.fingerprint=$5 WHERE i.id=$1 AND i.generation=$2 AND i.owner_id=$3 AND i.grant_revision=$4 AND i.approved_fingerprint=$5 AND d.fingerprint=$5 AND d.availability='available'AND i.desired_state='enabled'AND i.lease_until>clock_timestamp())`, l.ID, l.Token, l.Owner, l.GrantRevision, l.Fingerprint, p.Scope).Scan(&current)
		if e != nil {
			return nil, e
		}
		if !current {
			return nil, contracts.NewError(contracts.PermissionDenied)
		}
		raw, e := json.Marshal(out)
		if e != nil || len(raw) > 512<<10 {
			return nil, contracts.NewError(contracts.PayloadTooLarge)
		}
		return out, nil
	}
	if len(call.OperationKey) < 8 || len(call.OperationKey) > 100 || strings.ContainsAny(call.OperationKey, "\r\n\x00") {
		return nil, contracts.NewError(contracts.InvalidParams)
	}
	request, _ := json.Marshal(struct {
		Fingerprint string
		Call        contracts.HostCall
	}{l.Fingerprint, call})
	hash := digest(request)
	suffix := "/" + digest([]byte(call.Method+":"+call.OperationKey))
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer rollback(ctx, tx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,62))`, l.ID); e != nil {
		return nil, e
	}
	var id, key string
	var cipher []byte
	e = tx.QueryRow(ctx, `SELECT id::text,key,ciphertext FROM plugin_v2_kv WHERE instance_id=$1 AND key LIKE '.host/%' AND right(key,65)=$2 ORDER BY key LIMIT 1`, l.ID, suffix).Scan(&id, &key, &cipher)
	if e == nil {
		raw, e := s.open("kv", id, cipher)
		if e != nil {
			return nil, e
		}
		var prior businessRecord
		if json.Unmarshal(raw, &prior) != nil || prior.Request != hash {
			return nil, contracts.NewError(contracts.Conflict)
		}
		if e = guard(ctx, tx); e != nil {
			return nil, e
		}
		if e = tx.Commit(ctx); e != nil {
			return nil, e
		}
		if !prior.Ready {
			return nil, contracts.NewError(contracts.ResultUnknown)
		}
		if prior.Code != 0 {
			return nil, contracts.NewError(prior.Code)
		}
		return prior.Result, nil
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return nil, e
	}
	// Only this reserved machine-generated key shape is eligible for bounded GC.
	cutoff := fmt.Sprintf(".host/%010d/", time.Now().Add(-7*24*time.Hour).Unix())
	if _, e = tx.Exec(ctx, `DELETE FROM plugin_v2_kv WHERE id IN(SELECT id FROM plugin_v2_kv WHERE instance_id=$1 AND key ~ '^\.host/[0-9]{10}/[a-f0-9]{64}$' AND key<$2 ORDER BY key LIMIT 32)`, l.ID, cutoff); e != nil {
		return nil, e
	}
	var count int
	var used int64
	if e = tx.QueryRow(ctx, `SELECT count(*),coalesce(sum(octet_length(ciphertext)),0) FROM plugin_v2_kv WHERE instance_id=$1`, l.ID).Scan(&count, &used); e != nil {
		return nil, e
	}
	if count >= 1024 || used > 9<<20 {
		return nil, contracts.NewError(contracts.Busy)
	}
	id = jobs.UUID()
	key = fmt.Sprintf(".host/%010d%s", time.Now().Unix(), suffix)
	raw, _ := json.Marshal(businessRecord{Request: hash})
	cipher, e = s.seal("kv", id, raw)
	if e != nil {
		return nil, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO plugin_v2_kv(id,instance_id,key,ciphertext,revision)VALUES($1,$2,$3,$4,1)`, id, l.ID, key, cipher); e != nil {
		return nil, e
	}
	details, _ := json.Marshal(map[string]string{"instanceId": l.ID, "method": call.Method})
	if _, e = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('plugin','plugins.v2.business.admit',$1)`, details); e != nil {
		return nil, e
	}
	if e = guard(ctx, tx); e != nil {
		return nil, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, e
	}
	effectCall := call
	effectCall.OperationKey = id // globally unique reserved receipt ID; request digest remains original
	result, callErr := s.Business.Call(dbutil.WithEffectGuard(dbutil.WithCommitGuard(ctx, guard), admit), effectCall, p, admit)
	record := businessRecord{Request: hash, Ready: true}
	if callErr != nil {
		record.Code = contracts.Unavailable
		var rpc *contracts.Error
		if errors.As(callErr, &rpc) {
			record.Code = contracts.SafeError(rpc).Code
		}
	} else {
		record.Result, e = json.Marshal(result)
		if e != nil || len(record.Result) > 128<<10 {
			return nil, contracts.NewError(contracts.PayloadTooLarge)
		}
	}
	raw, e = json.Marshal(record)
	if e != nil {
		return nil, e
	}
	cipher, e = s.seal("kv", id, raw)
	if e != nil {
		return nil, e
	}
	finish, e := s.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer rollback(ctx, finish)
	if _, e = finish.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,62))`, l.ID); e != nil {
		return nil, e
	}
	var usedBefore int64
	if e = finish.QueryRow(ctx, `SELECT coalesce(sum(octet_length(ciphertext)),0)FROM plugin_v2_kv WHERE instance_id=$1 AND id<>$2`, l.ID, id).Scan(&usedBefore); e != nil {
		return nil, e
	}
	if usedBefore+int64(len(cipher)) > 10<<20 {
		return nil, contracts.NewError(contracts.Busy)
	}
	if _, e = finish.Exec(ctx, `UPDATE plugin_v2_kv SET ciphertext=$2,revision=revision+1 WHERE id=$1`, id, cipher); e != nil {
		return nil, e
	}
	if e = guard(ctx, finish); e != nil {
		return nil, e
	}
	details, _ = json.Marshal(map[string]string{"instanceId": l.ID, "method": call.Method})
	if _, e = finish.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('plugin','plugins.v2.business',$1)`, details); e != nil {
		return nil, e
	}
	if e = finish.Commit(ctx); e != nil {
		return nil, e
	}
	if callErr != nil {
		return nil, contracts.NewError(record.Code)
	}
	return result, nil
}
func supportedBusinessScope(scope string) bool {
	for _, m := range b.Methods() {
		if m.Scope == scope && m.Supported {
			return true
		}
	}
	return false
}

func validBusinessResources(p contracts.Permission) bool {
	for key, values := range p.Resources {
		allowed := false
		switch key {
		case "siteIds":
			allowed = p.Scope == "sites.read" || p.Scope == "sites.refresh" || p.Scope == "sites.request" || p.Scope == "sites.templates.read" || p.Scope == "sites.templates.select" || p.Scope == "search.run" || p.Scope == "downloads.add" || p.Scope == "subscriptions.write"
		case "downloaderIds":
			allowed = strings.HasPrefix(p.Scope, "downloads.")
		case "rootIds", "storageIds":
			allowed = strings.HasPrefix(p.Scope, "fs.media:") || strings.HasPrefix(p.Scope, "storage.")
		case "hosts":
			allowed = p.Scope == "sites.request"
		}
		if !allowed || len(values) == 0 || len(values) > 64 {
			return false
		}
		for _, v := range values {
			if strings.ContainsAny(v, "*\r\n\x00") {
				return false
			}
			if key == "hosts" {
				u, e := url.Parse(v)
				if e != nil || u.Host == "" || u.User != nil || u.Scheme != "https" && u.Scheme != "http" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
					return false
				}
			}
		}
	}
	return true
}
