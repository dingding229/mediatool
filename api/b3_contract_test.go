package api_test

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestB3HierarchyAndExternalSubtitleContract(t *testing.T) {
	_, contract := generatedContract(t)
	raw, e := json.Marshal(contract)
	if e != nil {
		t.Fatal(e)
	}
	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]any `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
		Paths map[string]map[string]struct {
			Parameters []struct{ Name, In string }
			Responses  map[string]any
		} `json:"paths"`
	}
	if e = json.Unmarshal(raw, &doc); e != nil {
		t.Fatal(e)
	}
	for schema, field := range map[string]string{"modules_catalog_Item": "type", "modules_embyapi_dto_MediaStream": "DeliveryUrl"} {
		if _, ok := doc.Components.Schemas[schema].Properties[field]; !ok {
			t.Fatal("missing typed contract", schema, field)
		}
	}
	path := "/Videos/{id}/{mediaSourceId}/Subtitles/{index}/Stream.{format}"
	op := doc.Paths[path]["get"]
	if _, ok := op.Responses["302"]; !ok {
		t.Fatal("missing strict subtitle redirect")
	}
	if _, ok := op.Responses["200"]; !ok {
		t.Fatal("missing bounded uploaded-text subtitle exception")
	}
	for path, methods := range doc.Paths {
		if strings.Contains(path, "/Videos/") && !strings.Contains(strings.ToLower(path), "/subtitles/") {
			if _, ok := methods["get"].Responses["200"]; ok {
				t.Fatal("video must remain redirect-only", path)
			}
		}
	}
	for path, methods := range doc.Paths {
		for method, op := range methods {
			seen := map[string]bool{}
			for _, p := range op.Parameters {
				key := p.In + ":" + p.Name
				if seen[key] {
					t.Fatal("duplicate parameter", path, method, key)
				}
				seen[key] = true
			}
		}
	}
}
