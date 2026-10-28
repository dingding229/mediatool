package api_test

import (
	"encoding/json"
	"os"
	"testing"
)

func TestI12ArchiveRoutesAbsentRecoveryContractsPreserved(t *testing.T) {
	raw, e := os.ReadFile("openapi.yaml")
	if e != nil {
		t.Fatal(e)
	}
	var d struct {
		Paths map[string]json.RawMessage `json:"paths"`
	}
	if e = json.Unmarshal(raw, &d); e != nil {
		t.Fatal(e)
	}
	for _, path := range []string{"/api/v1/library/archives", "/api/v1/library/archive-policy", "/api/v1/library/archives/cleanup-preview", "/api/v1/library/archives/cleanup-confirm", "/api/v1/library/archive-cleanups/{id}", "/api/v1/library/archive-cleanups/{id}/retry", "/api/v1/library/artifact-repairs"} {
		if _, ok := d.Paths[path]; ok {
			t.Fatal("retired path remains", path)
		}
	}
	for _, path := range []string{"/api/v1/library/activity", "/api/v1/library/artifact-repairs/{id}/retry", "/api/v1/transfers/{id}/output-preview", "/api/v1/transfers/{id}/resume-import"} {
		if _, ok := d.Paths[path]; !ok {
			t.Fatal("recovery path lost", path)
		}
	}
}
