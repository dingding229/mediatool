package migrations_test

import (
	"media-tool/migrations"
	"reflect"
	"regexp"
	"slices"
	"testing"
)

func TestPluginV2ChecksMatchGoEnums(t *testing.T) {
	raw, e := migrations.FS.ReadFile("00062_plugin_v2_runtime.sql")
	if e != nil {
		t.Fatal(e)
	}
	for _, tt := range []struct{ column, typ string }{{"desired_state", "Desired"}, {"runtime_state", "State"}, {"availability", "Availability"}, {"reason", "Reason"}, {"kind", "Operation"}} {
		expression := regexp.MustCompile(tt.column + ` IN\(([^)]*)\)`)
		match := expression.FindStringSubmatch(string(raw))
		if len(match) != 2 {
			t.Fatal("missing CHECK", tt.column)
		}
		literals := regexp.MustCompile(`'([^']*)'`).FindAllStringSubmatch(match[1], -1)
		actual := []string{}
		for _, l := range literals {
			actual = append(actual, l[1])
		}
		slices.Sort(actual)
		expected := declaredEnum(t, "internal/modules/plugins/v2/types.go", tt.typ)
		if !reflect.DeepEqual(actual, expected) {
			t.Fatalf("%s SQL=%v Go=%v", tt.column, actual, expected)
		}
	}
}
