package v2

import (
	"context"
	"encoding/json"
	"testing"
)

func TestT16ExternalConfigDisabledWithoutChangingCiphertextOrBuiltin(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	defs, e := Catalog()
	if e != nil {
		t.Fatal(e)
	}
	var d Definition
	for _, v := range defs {
		if v.Manifest.ID == "library-refresh-notify" {
			d = v
		}
	}
	// Install normally with a valid builtin config, then model a persisted pre-T16 config.
	req := Request{Kind: Install, OperationKey: "t16-install", PluginID: d.Manifest.ID, Fingerprint: d.Fingerprint, Confirm: true, Scopes: scopes(d), Config: json.RawMessage(`{"active":false,"provider":"builtin","intervalSeconds":"60"}`)}
	first, e := s.Apply(ctx, "test-admin", req)
	if e != nil {
		t.Fatal(e)
	}
	cipher, e := s.seal("config", first.ID, json.RawMessage(`{"active":true,"provider":"emby","intervalSeconds":"60"}`))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.DB.Exec(ctx, `UPDATE plugin_v2_configs SET ciphertext=$2 WHERE id=$1`, first.ID, cipher); e != nil {
		t.Fatal(e)
	}
	for range 2 {
		if e = s.retireExternalMediaConfigs(ctx); e != nil {
			t.Fatal(e)
		}
	}
	var desired, reason string
	var same bool
	if e = s.DB.QueryRow(ctx, `SELECT i.desired_state,i.reason,c.ciphertext=$2 FROM plugin_v2_instances i JOIN plugin_v2_configs c ON c.id=i.id WHERE i.id=$1`, first.ID, cipher).Scan(&desired, &reason, &same); e != nil || desired != "disabled" || reason != "external_media_removed" || !same {
		t.Fatal(desired, reason, same, e)
	}
}
