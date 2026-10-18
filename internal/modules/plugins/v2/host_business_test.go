package v2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	b "github.com/dingding229/mediatool-plugin/sdk/host/business"
	"media-tool/internal/modules/jobs"
	dbutil "media-tool/internal/platform/database"
	"strings"
	"testing"
	"time"
)

type businessFunc func(context.Context, contracts.HostCall, contracts.Permission, func() error) (any, error)

func (f businessFunc) Call(c context.Context, v contracts.HostCall, p contracts.Permission, a func() error) (any, error) {
	return f(c, v, p, a)
}
func businessCall(method, key string) contracts.HostCall {
	return contracts.HostCall{Method: method, OperationKey: key, Args: json.RawMessage(`{}`)}
}
func requireCode(t *testing.T, e error, code contracts.Code) {
	t.Helper()
	var rpc *contracts.Error
	if !errors.As(e, &rpc) || rpc.Code != code {
		t.Fatalf("expected code %d, got %v", code, e)
	}
}
func TestEveryBusinessMethodLiveScopeAndRevocation(t *testing.T) {
	s, l := activeFixture(t)
	ctx := context.Background()
	defs, _ := Catalog()
	base := defs[0].Manifest
	var calls int
	s.Business = businessFunc(func(c context.Context, _ contracts.HostCall, _ contracts.Permission, admit func() error) (any, error) {
		calls++
		if e := admit(); e != nil {
			return nil, e
		}
		return b.Receipt{ID: "fixture", State: "observed"}, nil
	})
	for i, m := range b.Methods() {
		t.Run(m.Name, func(t *testing.T) {
			if hostScope(m.Name) != m.Scope {
				t.Fatal("scope drift")
			}
			call := businessCall(m.Name, "f3a-operation-"+strings.ReplaceAll(m.Name, ".", "-"))
			before := calls
			_, e := s.hostDeclared(ctx, l, call, base)
			requireCode(t, e, contracts.PermissionDenied)
			manifest := base
			manifest.Permissions = append(append([]contracts.Permission{}, base.Permissions...), contracts.Permission{Scope: m.Scope})
			_, e = s.hostDeclared(ctx, l, call, manifest)
			requireCode(t, e, contracts.PermissionDenied)
			if _, e = s.DB.Exec(ctx, `INSERT INTO plugin_v2_grants(instance_id,scope,fingerprint)VALUES($1,$2,$3)ON CONFLICT DO NOTHING`, l.ID, m.Scope, l.Fingerprint); e != nil {
				t.Fatal(e)
			}
			_, e = s.hostDeclared(ctx, l, call, manifest)
			if m.Supported {
				if e != nil || calls != before+1 {
					t.Fatal("authorized path", e, calls)
				}
			} else {
				requireCode(t, e, contracts.Unsupported)
				if calls != before {
					t.Fatal("unsupported dispatched")
				}
			}

			prior := s.Business
			s.Business = businessFunc(func(context.Context, contracts.HostCall, contracts.Permission, func() error) (any, error) {
				return nil, contracts.NewError(contracts.Unsupported)
			})
			unavailable := call
			unavailable.OperationKey += "-unsupported"
			_, e = s.hostDeclared(ctx, l, unavailable, manifest)
			requireCode(t, e, contracts.Unsupported)
			s.Business = prior
			if _, e = s.DB.Exec(ctx, `DELETE FROM plugin_v2_grants WHERE instance_id=$1 AND scope=$2`, l.ID, m.Scope); e != nil {
				t.Fatal(e)
			}
			_, e = s.hostDeclared(ctx, l, call, manifest)
			requireCode(t, e, contracts.PermissionDenied)
		})
		if i%10 == 0 {
			if e := s.Renew(ctx, l, true); e != nil {
				t.Fatal(e)
			}
		}
	}
}
func TestBusinessReceiptsUnknownNeverReplayReservedKVAndClone(t *testing.T) {
	s, l := activeFixture(t)
	ctx := context.Background()
	p := contracts.Permission{Scope: "downloads.add"}
	_, e := s.DB.Exec(ctx, `INSERT INTO plugin_v2_grants(instance_id,scope,fingerprint)VALUES($1,$2,$3)`, l.ID, p.Scope, l.Fingerprint)
	if e != nil {
		t.Fatal(e)
	}
	n := 0
	s.Business = businessFunc(func(context.Context, contracts.HostCall, contracts.Permission, func() error) (any, error) {
		n++
		return b.Receipt{ID: "safe-result", State: "pending"}, nil
	})
	call := businessCall("downloads.manualCreate", "durable-operation-key")
	if _, e = s.business(ctx, l, call, p); e != nil {
		t.Fatal(e)
	}
	if _, e = s.business(ctx, l, call, p); e != nil || n != 1 {
		t.Fatal("replay", e, n)
	}
	call.Args = json.RawMessage(`{"different":true}`)
	_, e = s.business(ctx, l, call, p)
	requireCode(t, e, contracts.Conflict)
	var key string
	if e = s.DB.QueryRow(ctx, `SELECT key FROM plugin_v2_kv WHERE instance_id=$1 AND key LIKE '.host/%'`, l.ID).Scan(&key); e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(contracts.HostCall{Method: "plugin.data.delete", Args: json.RawMessage(`{"key":"` + key + `"}`)})
	_, e = s.hostCall(ctx, l, raw)
	requireCode(t, e, contracts.InvalidParams)
	target := jobs.UUID()
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer rollback(ctx, tx)
	if e = s.cloneKV(ctx, tx, l.ID, target); e != nil {
		t.Fatal(e)
	}
	var count int
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM plugin_v2_kv WHERE instance_id=$1`, target).Scan(&count); e != nil || count != 0 {
		t.Fatal("receipt cloned", e, count)
	}
}
func TestBusinessCommitFenceDoesNotHoldAuthLockAcrossWork(t *testing.T) {
	s, l := activeFixture(t)
	ctx := context.Background()
	p := contracts.Permission{Scope: "downloads.add"}
	if _, e := s.DB.Exec(ctx, `INSERT INTO plugin_v2_grants(instance_id,scope,fingerprint)VALUES($1,$2,$3)`, l.ID, p.Scope, l.Fingerprint); e != nil {
		t.Fatal(e)
	}
	staged, release := make(chan struct{}), make(chan struct{})
	s.Business = businessFunc(func(c context.Context, _ contracts.HostCall, _ contracts.Permission, _ func() error) (any, error) {
		tx, e := s.DB.Begin(c)
		if e != nil {
			return nil, e
		}
		defer rollback(c, tx)
		if _, e = tx.Exec(c, `INSERT INTO outbox_events(id,event_type,aggregate_id,payload)VALUES($1,'f3a.test',$2,'{}')`, jobs.UUID(), l.ID); e != nil {
			return nil, e
		}
		close(staged)
		<-release
		return b.Receipt{}, dbutil.Commit(c, tx)
	})
	done := make(chan error, 1)
	go func() {
		_, e := s.business(ctx, l, businessCall("downloads.manualCreate", "fenced-operation"), p)
		done <- e
	}()
	<-staged
	bounded, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	_, e := s.DB.Exec(bounded, `UPDATE plugin_v2_instances SET desired_state='disabled' WHERE id=$1`, l.ID)
	close(release)
	workerErr := <-done
	if e != nil {
		t.Fatal("revoke blocked by business work", e)
	}
	requireCode(t, workerErr, contracts.PermissionDenied)
	var count int
	if e = s.DB.QueryRow(ctx, `SELECT count(*)FROM outbox_events WHERE event_type='f3a.test'`).Scan(&count); e != nil || count != 0 {
		t.Fatal("unfenced business commit", e, count)
	}
}
func TestBusinessResourceSelectorsFailClosed(t *testing.T) {
	for _, p := range []contracts.Permission{{Scope: "sites.request", Resources: map[string][]string{"hosts": {"https://*.invalid"}}}, {Scope: "search.read", Resources: map[string][]string{"siteIds": {"a"}}}, {Scope: "downloads.add", Resources: map[string][]string{"libraryIds": {"a"}}}} {
		if validBusinessResources(p) {
			t.Fatal(p)
		}
	}
	if !validBusinessResources(contracts.Permission{Scope: "sites.request", Resources: map[string][]string{"hosts": {"https://owned.invalid"}, "siteIds": {"owned"}}}) {
		t.Fatal("valid exact selector")
	}
}

func TestBusinessUnknownReceiptDoesNotExecuteAndGCIsBounded(t *testing.T) {
	s, l := activeFixture(t)
	ctx := context.Background()
	p := contracts.Permission{Scope: "downloads.add"}
	if _, e := s.DB.Exec(ctx, `INSERT INTO plugin_v2_grants(instance_id,scope,fingerprint)VALUES($1,$2,$3)`, l.ID, p.Scope, l.Fingerprint); e != nil {
		t.Fatal(e)
	}
	call := businessCall("downloads.manualCreate", "unknown-operation")
	raw, _ := json.Marshal(struct {
		Fingerprint string
		Call        contracts.HostCall
	}{l.Fingerprint, call})
	record, _ := json.Marshal(businessRecord{Request: digest(raw)})
	id := jobs.UUID()
	cipher, e := s.seal("kv", id, record)
	if e != nil {
		t.Fatal(e)
	}
	key := fmt.Sprintf(".host/%010d/%s", time.Now().Unix(), digest([]byte(call.Method+":"+call.OperationKey)))
	if _, e = s.DB.Exec(ctx, `INSERT INTO plugin_v2_kv(id,instance_id,key,ciphertext,revision)VALUES($1,$2,$3,$4,1)`, id, l.ID, key, cipher); e != nil {
		t.Fatal(e)
	}
	invoked := 0
	s.Business = businessFunc(func(context.Context, contracts.HostCall, contracts.Permission, func() error) (any, error) {
		invoked++
		return b.Receipt{State: "pending"}, nil
	})
	_, e = s.business(ctx, l, call, p)
	requireCode(t, e, contracts.ResultUnknown)
	if invoked != 0 {
		t.Fatal("unknown receipt replayed")
	}
	for i := 0; i < 40; i++ {
		id := jobs.UUID()
		cipher, e := s.seal("kv", id, json.RawMessage(`{}`))
		if e != nil {
			t.Fatal(e)
		}
		key := fmt.Sprintf(".host/%010d/%s", time.Now().Add(-8*24*time.Hour).Unix(), digest([]byte(fmt.Sprint(i))))
		if _, e = s.DB.Exec(ctx, `INSERT INTO plugin_v2_kv(id,instance_id,key,ciphertext,revision)VALUES($1,$2,$3,$4,1)`, id, l.ID, key, cipher); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = s.business(ctx, l, businessCall("downloads.manualCreate", "next-operation"), p); e != nil {
		t.Fatal(e)
	}
	var count int
	if e = s.DB.QueryRow(ctx, `SELECT count(*) FROM plugin_v2_kv WHERE instance_id=$1`, l.ID).Scan(&count); e != nil || count != 10 {
		t.Fatal("GC must remove only 32", count, e)
	}
}

func TestUnsupportedSelectorOnReviewedDefinitionNeverBroadensScope(t *testing.T) {
	s, l := activeFixture(t)
	ctx := context.Background()
	if _, e := s.DB.Exec(ctx, `INSERT INTO plugin_v2_grants(instance_id,scope,fingerprint)VALUES($1,'downloads.add',$2)`, l.ID, l.Fingerprint); e != nil {
		t.Fatal(e)
	}
	defs, _ := Catalog()
	m := defs[0].Manifest
	m.Permissions = append(append([]contracts.Permission{}, m.Permissions...), contracts.Permission{Scope: "downloads.add", Resources: map[string][]string{"libraryIds": {"not-a-downloader-selector"}}})
	invoked := false
	s.Business = businessFunc(func(context.Context, contracts.HostCall, contracts.Permission, func() error) (any, error) {
		invoked = true
		return b.Receipt{}, nil
	})
	_, e := s.hostDeclared(ctx, l, businessCall("downloads.manualCreate", "invalid-selector"), m)
	requireCode(t, e, contracts.Unsupported)
	if invoked {
		t.Fatal("unsupported selector silently broadened")
	}
}

func TestMediaBusinessResourceSelectorsNeverBroaden(t *testing.T) {
	for _, scope := range []string{"fs.media:read", "fs.media:write", "storage.read", "storage.write"} {
		if !validBusinessResources(contracts.Permission{Scope: scope, Resources: map[string][]string{"rootIds": {"exact-id"}, "storageIds": {"other-exact-id"}}}) {
			t.Fatal(scope)
		}
		for _, v := range []map[string][]string{{"hosts": {"https://example.invalid"}}, {"rootIds": {"*"}}, {"rootIds": {}}, {"libraryIds": {"other"}}} {
			if validBusinessResources(contracts.Permission{Scope: scope, Resources: v}) {
				t.Fatal(scope, v)
			}
		}
	}
	if validBusinessResources(contracts.Permission{Scope: "library.read", Resources: map[string][]string{"libraryIds": {"g:movie"}}}) {
		t.Fatal("unimplemented library selector broadened to global")
	}
}
