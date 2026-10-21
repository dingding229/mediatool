package api_test

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestB4WebRestoreContract(t *testing.T) {
	_, doc := generatedContract(t)
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Paths map[string]map[string]struct {
			Responses map[string]any
			Security  []any
		}
		Components struct {
			Schemas map[string]struct{ Properties map[string]any }
		}
	}
	if err = json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	for path, method := range map[string]string{"/api/v1/ops/restores": "get", "/api/v1/ops/restores/preview": "post", "/api/v1/ops/restores/{id}/execute": "post", "/api/v1/ops/restores/{id}/reconcile": "post"} {
		op, ok := spec.Paths[path][method]
		if !ok || len(op.Security) == 0 {
			t.Fatal("admin boundary absent", path)
		}
		status := "200"
		if method == "post" && path == "/api/v1/ops/restores/{id}/execute" {
			status = "202"
		}
		if _, ok = op.Responses[status]; !ok {
			t.Fatal("response contract absent", path, status)
		}
	}
	model := spec.Components.Schemas["modules_maintenance_WebRestore"]
	for _, field := range []string{"state", "snapshotId", "appliedBackup", "expiresAt", "message", "tableChanges"} {
		if _, ok := model.Properties[field]; !ok {
			t.Fatal(field)
		}
	}
	if _, ok := model.Properties["protected"]; ok {
		t.Fatal("internal protection digests leaked")
	}
}

func TestB4bRestorePreviewExplicitRiskAndTableCounts(t *testing.T) {
	_, doc := generatedContract(t)
	raw, _ := json.Marshal(doc)
	for _, name := range []string{"tableChanges", "currentRows", "backupRows", "currentMissing", "warnings"} {
		if !strings.Contains(string(raw), `"`+name+`"`) {
			t.Fatal(name)
		}
	}
}
