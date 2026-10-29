package api_test

import (
	"media-tool/internal/modules/maintenance"
	"reflect"
	"testing"
)

func annotateAlertContract(schemas object) {
	schema := schemas["modules_maintenance_Alert"].(object)
	props := schema["properties"].(object)
	props["unit"].(object)["enum"] = []string{"GiB", "percent", "load_per_cpu", "jobs_per_10m", "queues", "entries"}
	props["comparison"].(object)["enum"] = []string{"<=", ">="}
	for _, k := range []string{"threshold", "unit", "comparison"} {
		props[k].(object)["readOnly"] = true
	}
	props["threshold"].(object)["description"] = "当前策略与实际告警判定共用的阈值；未知规则不提供展示元数据。"
}

func TestI23AlertMetadataSchema(t *testing.T) {
	schema := readDocument(t)["components"].(object)["schemas"].(object)["modules_maintenance_Alert"].(object)
	props := schema["properties"].(object)
	if !reflect.DeepEqual(jsonFields(reflect.TypeOf(maintenance.Alert{})), keys(props)) {
		t.Fatal("alert DTO differs from schema")
	}
	for _, k := range []string{"threshold", "unit", "comparison"} {
		if props[k].(object)["readOnly"] != true {
			t.Fatal(k)
		}
	}
	units := props["unit"].(object)["enum"].([]any)
	if len(units) != 6 {
		t.Fatal(units)
	}
}
