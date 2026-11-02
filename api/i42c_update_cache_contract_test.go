package api_test

import "testing"

func annotateI42cUpdateState(schemas object) {
	schemas["modules_updatecheck_State"].(object)["properties"].(object)["currentAtLeastLatest"].(object)["readOnly"] = true
}
func TestI42cUpdateCacheProjectionContract(t *testing.T) {
	d := readDocument(t)
	props := d["components"].(object)["schemas"].(object)["modules_updatecheck_State"].(object)["properties"].(object)
	if props["currentAtLeastLatest"].(object)["readOnly"] != true {
		t.Fatal("cache comparison must be read-only")
	}
}
