package migrations_test

import (
	"media-tool/migrations"
	"reflect"
	"regexp"
	"slices"
	"testing"
)

func TestPluginExtensionRunChecksMatchGo(t *testing.T) {
	raw, e := migrations.FS.ReadFile("00063_plugin_v2_extensions.sql")
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range []struct{ col, typ string }{{"kind", "RunKind"}, {"state", "RunState"}, {"reason", "RunReason"}} {
		checkRaw := raw
		if v.col == "kind" {
			var e error
			checkRaw, e = migrations.FS.ReadFile("00065_plugin_tools.sql")
			if e != nil {
				t.Fatal(e)
			}
		}
		m := regexp.MustCompile(v.col + ` IN\(([^)]*)\)`).FindStringSubmatch(string(checkRaw))
		if len(m) != 2 {
			t.Fatal(v)
		}
		vals := []string{}
		for _, x := range regexp.MustCompile(`'([^']*)'`).FindAllStringSubmatch(m[1], -1) {
			vals = append(vals, x[1])
		}
		slices.Sort(vals)
		if !reflect.DeepEqual(vals, declaredEnum(t, "internal/modules/plugins/v2/extensions.go", v.typ)) {
			t.Fatal(v, vals)
		}
	}
}
func TestPluginExtensionMigrationUpDownUp(t *testing.T) {
	f := newNativeFixture(t)
	if _, e := f.provider.UpTo(f.ctx, 62); e != nil {
		t.Fatal(e)
	}
	before := pluginUnrelatedSnapshot(t, f)
	for range 2 {
		if _, e := f.provider.UpTo(f.ctx, 63); e != nil {
			t.Fatal(e)
		}
		if !reflect.DeepEqual(before, pluginUnrelatedSnapshot(t, f)) {
			t.Fatal("non-plugin table mutated")
		}
		if _, e := f.provider.DownTo(f.ctx, 62); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := f.provider.UpTo(f.ctx, 63); e != nil {
		t.Fatal(e)
	}
}
