package api_test

import "testing"

func TestI29IdentitySnapshotAndBrowseQueryContract(t *testing.T) {
	doc := readDocument(t)
	props := doc["components"].(object)["schemas"].(object)["modules_metadata_Item"].(object)["properties"].(object)
	for _, key := range []string{"candidateTitles", "imdbId"} {
		if props[key] == nil {
			t.Fatal("missing identity evidence", key)
		}
	}
	paths := doc["paths"].(object)
	for _, p := range []string{"/api/v1/search/{id}", "/api/external/v1/search/{id}"} {
		route := paths[p].(object)["get"].(object)
		found := false
		for _, v := range route["parameters"].([]any) {
			if v.(object)["name"] == "ignoreMediaIdentity" {
				found = true
			}
		}
		if !found {
			t.Fatal("missing browse-only query", p)
		}
	}
}
