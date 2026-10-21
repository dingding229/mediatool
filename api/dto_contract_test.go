package api_test

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"media-tool/internal/modules/acquisition"
	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/externalapi"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/maintenance"
	pluginv2 "media-tool/internal/modules/plugins/v2"
	"media-tool/internal/platform/flow"
	"media-tool/internal/platform/httpapi"
)

func readDocument(t *testing.T) object {
	t.Helper()
	raw, e := os.ReadFile("openapi.yaml")
	if e != nil {
		t.Fatal(e)
	}
	var d object
	if e = json.Unmarshal(raw, &d); e != nil {
		t.Fatal(e)
	}
	return d
}
func jsonFields(ty reflect.Type) []string {
	var out []string
	for i := 0; i < ty.NumField(); i++ {
		f := ty.Field(i)
		if !f.IsExported() {
			continue
		}
		tag := strings.Split(f.Tag.Get("json"), ",")[0]
		if tag == "-" {
			continue
		}
		if f.Anonymous && tag == "" {
			out = append(out, jsonFields(f.Type)...)
			continue
		}
		if tag == "" {
			tag = f.Name
		}
		out = append(out, tag)
	}
	sort.Strings(out)
	return out
}
func keys(m map[string]any) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Independent runtime reflection (not the AST schema emitter). It catches wire
// projection/custom MarshalJSON drift and tests the actual serialized field set.
func TestKeyDTOFieldsAndSerializedProjectionMatchSchemas(t *testing.T) {
	schemas := readDocument(t)["components"].(map[string]any)["schemas"].(map[string]any)
	for _, value := range []any{externalapi.Key{}, externalapi.KeySummary{}, acquisition.Task{}, integrations.Record{}, integrations.Input{}, integrations.Readiness{}, pluginv2.Instance{}, pluginv2.Capability{}, pluginv2.Run{}, pluginv2.Request{}, pluginv2.StateRequest{}, maintenance.Request{}, flow.Status{}, httpapi.APIError{}, dto.AuthenticationResult{}, dto.PlaybackInfoResponse{}} {
		ty := reflect.TypeOf(value)
		name := ty.PkgPath() + "." + ty.Name()
		var schema map[string]any
		for _, v := range schemas {
			m := v.(map[string]any)
			if m["x-go-type"] == name {
				schema = m
				break
			}
		}
		if schema == nil { // APIError is the explicit error wire projection.
			if ty == reflect.TypeOf(httpapi.APIError{}) {
				schema = schemas["ErrorEnvelope"].(map[string]any)["properties"].(map[string]any)["error"].(map[string]any)
			} else {
				t.Errorf("no typed schema for %s", name)
				continue
			}
		}
		props := schema["properties"].(map[string]any)
		fields := jsonFields(ty)
		if ty == reflect.TypeOf(maintenance.Request{}) {
			fields = append(fields, "flow")
			sort.Strings(fields)
		}
		if !reflect.DeepEqual(fields, keys(props)) {
			t.Errorf("%s fields: Go=%v schema=%v", name, fields, keys(props))
		}
		raw, e := json.Marshal(value)
		if e != nil {
			t.Fatal(e)
		}
		var actual map[string]any
		if e = json.Unmarshal(raw, &actual); e != nil {
			t.Fatal(e)
		}
		for k := range actual {
			if props[k] == nil {
				t.Errorf("%s serialized undeclared field %s", name, k)
			}
		}
		if required, ok := schema["required"].([]any); ok {
			for _, k := range required {
				if _, ok := actual[k.(string)]; !ok {
					t.Errorf("%s missing declared required wire field %s", name, k)
				}
			}
		}
	}
	// Non-struct custom JSON scalars/containers have their actual wire types.
	stamp := dto.NewUTCDateTime(time.Date(2026, 10, 20, 0, 0, 0, 100, time.UTC))
	raw, e := json.Marshal(stamp)
	if e != nil || string(raw) != `"2026-10-20T00:00:00.0000001Z"` {
		t.Fatal(string(raw), e)
	}
	raw, e = json.Marshal(dto.Array[string](nil))
	if e != nil || string(raw) != "[]" {
		t.Fatal(string(raw), e)
	}
}

func TestSchemaReferencesErrorsAndProtocolBoundaries(t *testing.T) {
	doc := readDocument(t)
	schemas := doc["components"].(map[string]any)["schemas"].(map[string]any)
	var walk func(any)
	walk = func(v any) {
		switch v := v.(type) {
		case map[string]any:
			if ref, ok := v["$ref"].(string); ok {
				prefix := "#/components/schemas/"
				responses := doc["components"].(map[string]any)["responses"].(map[string]any)
				if (!strings.HasPrefix(ref, prefix) || schemas[strings.TrimPrefix(ref, prefix)] == nil) && (!strings.HasPrefix(ref, "#/components/responses/") || responses[strings.TrimPrefix(ref, "#/components/responses/")] == nil) {
					t.Error("dangling schema ref", ref)
				}
			}
			for _, x := range v {
				walk(x)
			}
		case []any:
			for _, x := range v {
				walk(x)
			}
		}
	}
	walk(doc)
	schemes := doc["components"].(map[string]any)["securitySchemes"].(map[string]any)
	for path, methods := range doc["paths"].(map[string]any) {
		for method, value := range methods.(map[string]any) {
			op := value.(map[string]any)
			for _, item := range op["security"].([]any) {
				for k := range item.(map[string]any) {
					if schemes[k] == nil {
						t.Error("unknown auth", path, k)
					}
				}
			}
			for _, m := range parameterPattern.FindAllStringSubmatch(path, -1) {
				found := false
				if params, ok := op["parameters"].([]any); ok {
					for _, p := range params {
						p := p.(map[string]any)
						if p["in"] == "path" && p["name"] == m[1] && p["required"] == true {
							found = true
						}
					}
				}
				if method != "options" && !found {
					t.Error("missing path parameter", method, path, m[1])
				}
			}
			if path == "/socket" {
				if op["responses"].(map[string]any)["101"] != nil {
					t.Error("fictitious websocket support")
				}
			}
			if strings.HasPrefix(path, "/Videos/") && (strings.Contains(path, "/stream") || strings.Contains(path, "/original")) && op["x-listener"] == "native-media" && method != "options" {
				if op["responses"].(map[string]any)["200"] != nil || op["responses"].(map[string]any)["206"] != nil {
					t.Error("native media byte relay advertised", path)
				}
			}
		}
	}
}
