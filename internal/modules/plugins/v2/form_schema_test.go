package v2

import (
	"context"
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"os"
	"strings"
	"testing"
)

func TestStaticFormInventoryUsesSamePinWithoutNewPermissions(t *testing.T) {
	var data struct {
		Pin   string                    `json:"pin"`
		Forms map[string]contracts.Form `json:"forms"`
	}
	if json.Unmarshal(libraryInventory, &data) != nil || data.Pin != BuildID() {
		t.Fatal("static forms do not match executable pin")
	}
	defs, e := Catalog()
	if e != nil || len(data.Forms) != len(defs) {
		t.Fatal(e, "missing builtin forms")
	}
	raw, e := os.ReadFile("testdata/form-permissions-before.json")
	if e != nil {
		t.Fatal(e)
	}
	var before []contracts.Manifest
	if json.Unmarshal(raw, &before) != nil {
		t.Fatal("invalid permission baseline")
	}
	old := map[string]contracts.Manifest{}
	for _, m := range before {
		old[m.ID] = m
	}
	for _, d := range defs {
		form, ok := data.Forms[d.Manifest.ID]
		b, _ := json.Marshal(form)
		if !ok || validateUI(d.Manifest, "form", b) != nil {
			t.Fatal("invalid static form", d.Manifest.ID)
		}
		previous, ok := old[d.Manifest.ID]
		if !ok || requiresReview(previous, d.Manifest) {
			t.Fatal("form pin changed execution permissions; stop release", d.Manifest.ID)
		}
	}
}
func TestSchemaOnlyRefreshDoesNotDisableApprovedInstance(t *testing.T) {
	s, _ := activeFixture(t)
	defs, _ := Catalog()
	d := defs[0]
	d.Fingerprint = digest([]byte("schema-only-test-build"))
	if e := s.refresh(context.Background(), []Definition{d}); e != nil {
		t.Fatal(e)
	}
	var desired, reason, approved string
	if e := s.DB.QueryRow(context.Background(), `SELECT desired_state,reason,approved_fingerprint FROM plugin_v2_instances WHERE plugin_id=$1`, d.Manifest.ID).Scan(&desired, &reason, &approved); e != nil {
		t.Fatal(e)
	}
	if desired != "enabled" || reason != "" || approved != d.Fingerprint {
		t.Fatal("schema update disabled plugin or requested permission consent", desired, reason, approved)
	}
}

func TestSettingsInspectionDoesNotReadOrRewriteInstanceKV(t *testing.T) {
	s, l := activeFixture(t)
	ctx := context.Background()
	if _, e := s.DB.Exec(ctx, `INSERT INTO plugin_v2_kv(id,instance_id,key,ciphertext,revision)VALUES(gen_random_uuid(),$1,'retained',decode(repeat('ff',64),'hex'),1)`, l.ID); e != nil {
		t.Fatal(e)
	}
	result, e := s.Configuration(ctx, l.ID)
	if e != nil || !json.Valid(result.Config.Value) || len(result.Entries) != 0 {
		t.Fatal("settings depended on KV data", e)
	}
	var value string
	if e := s.DB.QueryRow(ctx, `SELECT encode(ciphertext,'hex') FROM plugin_v2_kv WHERE instance_id=$1 AND key='retained'`, l.ID).Scan(&value); e != nil || value != strings.Repeat("ff", 64) {
		t.Fatal("KV changed", e, value)
	}
	if _, e := s.Inspect(ctx, l.ID, ""); e == nil {
		t.Fatal("fixture did not exercise a KV value that cannot be decrypted")
	}
}
