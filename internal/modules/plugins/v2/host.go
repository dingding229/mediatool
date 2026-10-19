package v2

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	b "github.com/dingding229/mediatool-plugin/sdk/host/business"
	"github.com/jackc/pgx/v5"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/platform/buildinfo"
	deferutil "media-tool/internal/platform/cleanup"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

func (s Store) DataPath(id string) (string, error) {
	if !validUUID(id) || !filepath.IsAbs(s.Root) || filepath.Clean(s.Root) != s.Root {
		return "", ErrInvalid
	}
	root, e := os.OpenRoot(s.Root)
	if e != nil {
		return "", e
	}
	defer deferutil.Close(root, "Filesystem descriptor teardown only; operation errors are handled")
	for _, part := range []string{id, id + "/data", id + "/tmp"} {
		st, e := root.Lstat(part)
		if e != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			return "", ErrUnavailable
		}
	}
	return filepath.Join(s.Root, id, "data"), nil
}
func (s Store) EnsurePaths(id string) error {
	if !validUUID(id) || !filepath.IsAbs(s.Root) || filepath.Clean(s.Root) != s.Root {
		return ErrInvalid
	}
	if e := os.MkdirAll(s.Root, 0700); e != nil {
		return e
	}
	st, e := os.Lstat(s.Root)
	if e != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return ErrInvalid
	}
	root, e := os.OpenRoot(s.Root)
	if e != nil {
		return e
	}
	defer deferutil.Close(root, "Filesystem descriptor teardown only; operation errors are handled")
	for _, part := range []string{id, id + "/data", id + "/tmp"} {
		if e = root.Mkdir(part, 0700); e != nil && !errors.Is(e, os.ErrExist) {
			return e
		}
		st, e := root.Lstat(part)
		if e != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			return ErrInvalid
		}
	}
	_, e = s.DataPath(id)
	return e
}
func hostScope(m string) string {
	switch m {
	case "plugin.config.get":
		return "plugin.config.read"
	case "plugin.config.update":
		return "plugin.config.write"
	case "plugin.data.get", "plugin.data.list":
		return "plugin.data.read"
	case "plugin.data.put", "plugin.data.delete":
		return "plugin.data.write"
	case "plugin.data.path":
		return "fs.data"
	case "plugin.state.get", "runtime.capabilities":
		return "plugin.state.read"
	case "system.summary":
		return "system.read"
	case "notifications.send":
		return "notifications.send"
	}
	if spec, ok := b.Lookup(m); ok {
		return spec.Scope
	}
	return ""
}
func (s Store) Host(l Lease) func(context.Context, json.RawMessage) (any, *contracts.Error) {
	return func(ctx context.Context, raw json.RawMessage) (any, *contracts.Error) {
		out, e := s.hostCall(ctx, l, raw)
		if e == nil {
			return out, nil
		}
		var rpc *contracts.Error
		if errors.As(e, &rpc) {
			return nil, contracts.SafeError(rpc)
		}
		return nil, contracts.NewError(contracts.Unavailable)
	}
}
func (s Store) hostCall(parent context.Context, l Lease, raw json.RawMessage) (any, error) {
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	var call contracts.HostCall
	if contracts.Decode(raw, &call) != nil || len(call.Args) > 128<<10 {
		return nil, contracts.NewError(contracts.InvalidParams)
	}
	if call.Method == "plugin.config.update" {
		defer invalidateNameHooks(s.DB)
	}
	scope := hostScope(call.Method)
	if scope == "" {
		return nil, contracts.NewError(contracts.MethodNotFound)
	}
	d, e := definition(l.PluginID)
	if e != nil || d.Fingerprint != l.Fingerprint {
		return nil, contracts.NewError(contracts.PermissionDenied)
	}
	return s.hostDeclared(ctx, l, call, d.Manifest)
}

// hostDeclared receives only the compiled manifest resolved by hostCall, never
// worker JSON. Kept separate so contract tests exercise real live-grant SQL.
func (s Store) hostDeclared(ctx context.Context, l Lease, call contracts.HostCall, manifest contracts.Manifest) (any, error) {
	scope := hostScope(call.Method)
	declared := false
	var permit contracts.Permission
	for _, p := range manifest.Permissions {
		if p.Scope == scope {
			declared = true
			permit = p
			break
		}
	}
	if !declared {
		return nil, contracts.NewError(contracts.PermissionDenied)
	}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer rollback(ctx, tx)
	// Shared non-key authorization lock: readers and lease renewal do not serialize.
	// Writes revalidate under a short final fence before committing. No auth cache.
	var state string
	lock := " FOR KEY SHARE OF i"
	if hostWrites(call.Method) {
		lock = ""
	}
	e = tx.QueryRow(ctx, `SELECT runtime_state FROM plugin_v2_instances i JOIN plugin_v2_definitions d ON d.id=i.plugin_id WHERE i.id=$1 AND i.generation=$2 AND i.owner_id=$3 AND i.grant_revision=$4 AND i.desired_state='enabled' AND i.lease_until>now() AND i.approved_fingerprint=d.fingerprint AND d.availability='available'`+lock, l.ID, l.Token, l.Owner, l.GrantRevision).Scan(&state)
	if e != nil {
		if errors.Is(e, pgx.ErrNoRows) {
			return nil, contracts.NewError(contracts.PermissionDenied)
		}
		return nil, e
	}
	var granted bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM plugin_v2_grants WHERE instance_id=$1 AND scope=$2 AND fingerprint=$3)`, l.ID, scope, l.Fingerprint).Scan(&granted); e != nil {
		return nil, e
	}
	if !granted {
		return nil, contracts.NewError(contracts.PermissionDenied)
	}
	for _, extra := range b.RequiredScopes(call.Method) {
		found := false
		for _, p := range manifest.Permissions {
			if p.Scope == extra {
				found = true
				break
			}
		}
		if !found {
			return nil, contracts.NewError(contracts.PermissionDenied)
		}
		var granted bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM plugin_v2_grants WHERE instance_id=$1 AND scope=$2 AND fingerprint=$3)`, l.ID, extra, l.Fingerprint).Scan(&granted); e != nil {
			return nil, e
		}
		if !granted {
			return nil, contracts.NewError(contracts.PermissionDenied)
		}
	}
	if _, ok := b.Lookup(call.Method); ok {
		if !validBusinessResources(permit) {
			return nil, contracts.NewError(contracts.Unsupported)
		}
		if e = tx.Commit(ctx); e != nil {
			return nil, e
		}
		return s.business(ctx, l, call, permit)
	}
	return s.authorizedCall(ctx, tx, l, call, state)
}

// authorizedCall is internal; production callers must first pass hostCall's live
// generation, declared-scope and grant checks. Tests can isolate this DB dispatcher.
func (s Store) authorizedCall(ctx context.Context, tx pgx.Tx, l Lease, call contracts.HostCall, state string) (any, error) {
	var e error
	if hostWrites(call.Method) {
		// Serialize quotas/CAS per instance, independently of supervisor leases.
		if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,62))`, l.ID); e != nil {
			return nil, e
		}
	}
	var result any
	switch call.Method {
	case "runtime.capabilities":
		result = struct {
			ContractVersion    int        `json:"contractVersion"`
			Methods            []b.Method `json:"methods"`
			UnsupportedOptions []string   `json:"unsupportedOptions"`
		}{contracts.ContractVersion, b.Methods(), []string{"sites.request.raw", "subscriptions.completion", "downloads.retry", "downloads.manualRetry.acceptDuplicateRisk"}}
	case "plugin.state.get":
		result = struct {
			State string `json:"state"`
		}{state}
	case "system.summary":
		result = struct {
			Version         string `json:"version"`
			ContractVersion int    `json:"contractVersion"`
		}{buildinfo.Version, contracts.ContractVersion}
	case "plugin.data.path":
		path, e := s.DataPath(l.ID)
		if e != nil {
			return nil, e
		}
		result = struct {
			Path string `json:"path"`
		}{path}
	case "plugin.config.get":
		var cipher []byte
		var v contracts.Config
		if e = tx.QueryRow(ctx, `SELECT ciphertext,revision FROM plugin_v2_configs WHERE id=$1`, l.ID).Scan(&cipher, &v.Revision); e != nil {
			return nil, e
		}
		v.Value, e = s.open("config", l.ID, cipher)
		if e != nil {
			return nil, e
		}
		result = v
	case "plugin.config.update":
		var a struct {
			Value json.RawMessage `json:"value"`
		}
		if contracts.Decode(call.Args, &a) != nil || contracts.ValidateJSON(a.Value) != nil || call.ExpectedRevision == nil {
			return nil, contracts.NewError(contracts.InvalidParams)
		}
		cipher, e := s.seal("config", l.ID, a.Value)
		if e != nil {
			return nil, e
		}
		var rev contracts.Revision
		e = tx.QueryRow(ctx, `UPDATE plugin_v2_configs SET ciphertext=$2,revision=revision+1 WHERE id=$1 AND revision=$3 RETURNING revision`, l.ID, cipher, *call.ExpectedRevision).Scan(&rev.Revision)
		if errors.Is(e, pgx.ErrNoRows) {
			return nil, contracts.NewError(contracts.Conflict)
		}
		if e != nil {
			return nil, e
		}
		if _, e = tx.Exec(ctx, `UPDATE plugin_v2_instances SET revision=revision+1,updated_at=now() WHERE id=$1`, l.ID); e != nil {
			return nil, e
		}
		if e = emitPluginConfig(ctx, tx, l.ID, rev.Revision); e != nil {
			return nil, e
		}
		result = rev
	case "plugin.data.list":
		var a struct {
			After string `json:"after"`
			Limit int    `json:"limit"`
		}
		if contracts.Decode(call.Args, &a) != nil || len(a.After) > 128 || a.Limit < 1 || a.Limit > 200 {
			return nil, contracts.NewError(contracts.InvalidParams)
		}
		rows, e := tx.Query(ctx, `SELECT key,revision FROM plugin_v2_kv WHERE instance_id=$1 AND key>$2 AND key NOT LIKE '.host/%' ORDER BY key LIMIT $3`, l.ID, a.After, a.Limit)
		if e != nil {
			return nil, e
		}
		type item struct {
			Key      string `json:"key"`
			Revision int64  `json:"revision"`
		}
		items := []item{}
		for rows.Next() {
			var v item
			if e = rows.Scan(&v.Key, &v.Revision); e != nil {
				break
			}
			items = append(items, v)
		}
		other := rows.Err()
		rows.Close()
		if e != nil {
			return nil, e
		}
		if other != nil {
			return nil, other
		}
		result = struct {
			Items []item `json:"items"`
		}{items}
	case "plugin.data.get", "plugin.data.put", "plugin.data.delete":
		var a struct {
			Key   string          `json:"key"`
			Value json.RawMessage `json:"value,omitempty"`
		}
		if contracts.Decode(call.Args, &a) != nil || a.Key == "" || strings.HasPrefix(a.Key, ".host/") || len(a.Key) > 128 || !utf8.ValidString(a.Key) || strings.ContainsAny(a.Key, "\x00\r\n") {
			return nil, contracts.NewError(contracts.InvalidParams)
		}
		var id string
		var cipher []byte
		var v contracts.Value
		e = tx.QueryRow(ctx, `SELECT id::text,ciphertext,revision FROM plugin_v2_kv WHERE instance_id=$1 AND key=$2`, l.ID, a.Key).Scan(&id, &cipher, &v.Revision)
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return nil, e
		}
		exists := e == nil
		if call.Method == "plugin.data.get" {
			if !exists {
				return nil, contracts.NewError(contracts.NotFound)
			}
			v.Value, e = s.open("kv", id, cipher)
			if e != nil {
				return nil, e
			}
			result = v
			break
		}
		if call.ExpectedRevision == nil || *call.ExpectedRevision != v.Revision {
			return nil, contracts.NewError(contracts.Conflict)
		}
		if call.Method == "plugin.data.delete" {
			if !exists {
				return nil, contracts.NewError(contracts.NotFound)
			}
			if _, e = tx.Exec(ctx, `DELETE FROM plugin_v2_kv WHERE id=$1`, id); e != nil {
				return nil, e
			}
			result = struct{}{}
			break
		}
		if contracts.ValidateJSON(a.Value) != nil {
			return nil, contracts.NewError(contracts.InvalidParams)
		}
		if !exists {
			id = jobs.UUID()
		}
		cipher, e = s.seal("kv", id, a.Value)
		if e != nil {
			return nil, e
		}
		var used int64
		var count int
		if e = tx.QueryRow(ctx, `SELECT coalesce(sum(octet_length(ciphertext)),0),count(*) FROM plugin_v2_kv WHERE instance_id=$1 AND id<>$2`, l.ID, id).Scan(&used, &count); e != nil {
			return nil, e
		}
		if (!exists && count >= 1024) || used+int64(len(cipher)) > 10<<20 {
			return nil, contracts.NewError(contracts.PayloadTooLarge)
		}
		v.Revision++
		if _, e = tx.Exec(ctx, `INSERT INTO plugin_v2_kv(id,instance_id,key,ciphertext,revision)VALUES($1,$2,$3,$4,$5) ON CONFLICT(instance_id,key)DO UPDATE SET ciphertext=excluded.ciphertext,revision=excluded.revision`, id, l.ID, a.Key, cipher, v.Revision); e != nil {
			return nil, e
		}
		result = contracts.Revision{Revision: v.Revision}
	case "notifications.send":
		var m contracts.Message
		if contracts.Decode(call.Args, &m) != nil || len(m.Title) > 256 || len(m.Text) > 8192 || m.ChannelID != "" {
			return nil, contracts.NewError(contracts.InvalidParams)
		}
		payload, _ := json.Marshal(map[string]string{"title": m.Title, "message": m.Text})
		event := jobs.UUID()
		if _, e = tx.Exec(ctx, `INSERT INTO outbox_events(id,event_type,aggregate_id,payload)VALUES($1,'plugin.notice',$2,$3)`, event, l.ID, payload); e != nil {
			return nil, e
		}
		result = struct{}{}
	default:
		return nil, contracts.NewError(contracts.MethodNotFound)
	}
	if hostWrites(call.Method) {
		if e = fenceHost(ctx, tx, l, hostScope(call.Method)); e != nil {
			return nil, e
		}
		details, _ := json.Marshal(map[string]string{"instanceId": l.ID, "method": call.Method})
		if _, e = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('plugin','plugins.v2.host',$1)`, details); e != nil {
			return nil, e
		}
	}
	return result, tx.Commit(ctx)
}

func hostWrites(method string) bool {
	if spec, ok := b.Lookup(method); ok {
		return spec.Level != "R"
	}
	return method == "plugin.data.put" || method == "plugin.data.delete" || method == "plugin.config.update" || method == "notifications.send"
}

// The conditional UPDATE waits for any committed revocation/definition refresh,
// then rechecks the current row. Zero rows rolls back *all* staged side effects.
func fenceHost(ctx context.Context, tx pgx.Tx, l Lease, scope string) error {
	tag, e := tx.Exec(ctx, `UPDATE plugin_v2_instances i SET updated_at=clock_timestamp(),revision=i.revision+CASE WHEN $6='plugin.config.write' THEN 1 ELSE 0 END,grant_revision=i.grant_revision+CASE WHEN $6='plugin.config.write' THEN 1 ELSE 0 END,runtime_state=CASE WHEN $6='plugin.config.write' THEN 'stopping' ELSE i.runtime_state END FROM plugin_v2_definitions d WHERE i.id=$1 AND i.generation=$2 AND i.owner_id=$3 AND i.grant_revision=$4 AND i.desired_state='enabled' AND i.lease_until>clock_timestamp() AND d.id=i.plugin_id AND d.availability='available' AND d.fingerprint=i.approved_fingerprint AND i.approved_fingerprint=$5 AND EXISTS(SELECT 1 FROM plugin_v2_grants g WHERE g.instance_id=i.id AND g.scope=$6 AND g.fingerprint=$5)`, l.ID, l.Token, l.Owner, l.GrantRevision, l.Fingerprint, scope)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return contracts.NewError(contracts.PermissionDenied)
	}
	return nil
}
