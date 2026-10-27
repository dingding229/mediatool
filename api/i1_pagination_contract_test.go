package api_test

import (
	"encoding/json"
	"os"
	"testing"
)

func TestI1OptionalPaginationContracts(t *testing.T) {
	raw, e := os.ReadFile("openapi.yaml")
	if e != nil {
		t.Fatal(e)
	}
	var d struct {
		Paths map[string]map[string]struct {
			Parameters []struct {
				Name   string `json:"name"`
				Schema struct {
					Default any   `json:"default"`
					Enum    []any `json:"enum"`
				} `json:"schema"`
			} `json:"parameters"`
		} `json:"paths"`
	}
	if e = json.Unmarshal(raw, &d); e != nil {
		t.Fatal(e)
	}
	for _, c := range []struct {
		path, key string
		initial   int
	}{{"/api/v1/media/library/managed/{id}/metadata", "unmatchedPageSize", 50}, {"/api/v1/cache/{kind}", "pageSize", 30}, {"/api/v1/plugins/workflows", "pageSize", 20}} {
		for _, method := range []string{"get", "head"} {
			found := false
			for _, p := range d.Paths[c.path][method].Parameters {
				if p.Name == c.key {
					found = true
					if p.Schema.Default != float64(c.initial) || len(p.Schema.Enum) < 4 {
						t.Fatal(c, method, p)
					}
				}
			}
			if !found {
				t.Fatal("missing", c, method)
			}
		}
	}
}
