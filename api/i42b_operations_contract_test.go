package api_test

import "testing"

func TestI42bReadOnlyOperationsProjections(t *testing.T) {
	d := readDocument(t)
	paths := d["paths"].(object)
	op := paths["/api/v1/jobs/browse"].(object)["get"].(object)
	names := map[string]bool{}
	for _, raw := range op["parameters"].([]any) {
		names[raw.(object)["name"].(string)] = true
	}
	for _, name := range []string{"hideRoutineSuccess", "unresolvedOnly"} {
		if !names[name] {
			t.Fatal(name)
		}
	}
	schemas := d["components"].(object)["schemas"].(object)
	props := schemas["modules_cacheview_Entry"].(object)["properties"].(object)
	if props["conditions"] == nil {
		t.Fatal("missing query conditions")
	}
}
