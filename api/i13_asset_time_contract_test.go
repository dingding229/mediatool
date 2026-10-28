package api_test

import (
	"strings"
	"testing"
)

func TestI13AssetListDocumentsCreationTime(t *testing.T) {
	doc := readDocument(t)
	obj := func(v any) map[string]any {
		t.Helper()
		m, ok := v.(map[string]any)
		if !ok {
			t.Fatalf("expected schema object, got %T", v)
		}
		return m
	}
	responses := obj(obj(doc["components"])["responses"])
	// HEAD intentionally has no response body; the list payload is GET-only.
	op := obj(obj(obj(doc["paths"])["/api/v1/media/assets"])["get"])
	response := obj(obj(op["responses"])["200"])
	response = obj(responses[strings.TrimPrefix(response["$ref"].(string), "#/components/responses/")])
	schema := obj(obj(obj(response["content"])["application/json"])["schema"])
	variants := obj(obj(schema["properties"])["data"])["anyOf"].([]any)
	for _, variant := range variants {
		row := obj(variant)
		if row["type"] == "object" {
			row = obj(obj(row["properties"])["items"])
		}
		row = obj(row["items"])
		created := obj(obj(row["properties"])["createdAt"])
		if created["type"] != "string" || created["format"] != "date-time" {
			t.Fatal("createdAt must be a serialized timestamp", created)
		}
		found := false
		for _, field := range row["required"].([]any) {
			found = found || field == "createdAt"
		}
		if !found {
			t.Fatal("createdAt must exist on every listed asset")
		}
	}
}
