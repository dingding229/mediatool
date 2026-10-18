// Package testworker is used only by integration tests. It never changes a
// production timeout. One common readiness barrier for ALL real-worker suites.
package testworker

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/jackc/pgx/v5/pgxpool"
	"testing"
	"time"
)

// Ready verifies every compiled manifest provider/handler, then performs a pure
// form round trip through the real worker lane. Timed policy assertions start
// only AFTER this barrier. A partial registry can never satisfy it.
func Ready(t testing.TB, db *pgxpool.Pool, id string, probe func(context.Context) error) {
	t.Helper()
	ctx, stop := context.WithTimeout(context.Background(), 20*time.Second)
	defer stop()
	var last error
	for ctx.Err() == nil {
		var raw, manifest []byte
		last = db.QueryRow(ctx, `SELECT s.descriptors,d.manifest FROM plugin_v2_instances i JOIN plugin_v2_extensions x ON x.instance_id=i.id JOIN plugin_v2_snapshots s ON s.id=x.snapshot_id JOIN plugin_v2_definitions d ON d.id=i.plugin_id WHERE i.id=$1 AND i.runtime_state='running' AND i.desired_state='enabled' AND i.generation=s.generation AND i.grant_revision=s.grant_revision AND i.lease_until>clock_timestamp()`, id).Scan(&raw, &manifest)
		if last == nil {
			var snap contracts.Snapshot
			var m contracts.Manifest
			if json.Unmarshal(raw, &snap) == nil && json.Unmarshal(manifest, &m) == nil {
				found := map[string]bool{}
				for _, d := range snap.Descriptors {
					found[d.Kind+":"+d.ID] = true
				}
				expected := map[string][]string{"event": m.Extensions.Events, "chain": m.Extensions.Chains, "module": m.Extensions.Modules, "service": m.Extensions.Services, "command": m.Extensions.Commands, "action": m.Extensions.Actions}
				complete := true
				for kind, list := range expected {
					for _, key := range list {
						complete = complete && found[kind+":"+key]
					}
				}
				for _, a := range m.Extensions.API {
					complete = complete && found["api:"+a.ID]
				}
				ui := []string{}
				if m.Extensions.Forms {
					ui = append(ui, "form")
				}
				if len(m.Extensions.Dashboards) > 0 {
					ui = append(ui, "meta")
				}
				for _, id := range m.Extensions.Pages {
					ui = append(ui, "page."+id)
				}
				for _, id := range m.Extensions.Dashboards {
					ui = append(ui, "dashboard."+id)
				}
				for _, id := range ui {
					complete = complete && found["ui:"+id]
				}
				if len(m.Extensions.AuthProviders) > 0 {
					complete = complete && found["chain:auth.verification"]
				}
				if complete {
					bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
					last = probe(bounded)
					cancel()
					if last == nil {
						return
					}
				} else {
					last = fmt.Errorf("incomplete provider/handler registry")
				}
			}
		}
		select {
		case <-ctx.Done():
		case <-time.After(30 * time.Millisecond):
		}
	}
	t.Fatalf("real worker readiness barrier failed: %v", last)
}
