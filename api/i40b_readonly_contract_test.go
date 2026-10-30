package api_test

import "testing"

func annotateI40bReadOnly(schemas object) {
	for _, entry := range []struct{ schema, key, description string }{
		{"modules_library_Scan", "startedAt", "本轮覆盖开始时间，仅展示；未开始时省略。"},
		{"modules_library_Scan", "endedAt", "终态最后落定时间，仅展示；进行中时省略。"},
		{"modules_catalog_Item", "versionCount", "当前集包含的物理版本数；容量为这些版本各累计一次。"},
	} {
		schemas[entry.schema].(object)["properties"].(object)[entry.key].(object)["readOnly"] = true
		schemas[entry.schema].(object)["properties"].(object)[entry.key].(object)["description"] = entry.description
	}
}
func TestI40bReadOnlyMetadata(t *testing.T) {
	schemas := readDocument(t)["components"].(object)["schemas"].(object)
	for _, entry := range [][2]string{{"modules_library_Scan", "startedAt"}, {"modules_library_Scan", "endedAt"}, {"modules_catalog_Item", "versionCount"}} {
		if schemas[entry[0]].(object)["properties"].(object)[entry[1]].(object)["readOnly"] != true {
			t.Fatal(entry)
		}
	}
}
