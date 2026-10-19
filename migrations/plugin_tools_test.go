package migrations_test

import (
	"reflect"
	"testing"
)

func TestPluginToolKindMigrationUpDownUp(t *testing.T) {
	f := newNativeFixture(t)
	if _, e := f.provider.UpTo(f.ctx, 64); e != nil {
		t.Fatal(e)
	}
	before := pluginUnrelatedSnapshot(t, f)
	for range 2 {
		if _, e := f.provider.UpTo(f.ctx, 65); e != nil {
			t.Fatal(e)
		}
		if !reflect.DeepEqual(before, pluginUnrelatedSnapshot(t, f)) {
			t.Fatal("non-plugin changed")
		}
		if _, e := f.provider.DownTo(f.ctx, 64); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := f.provider.UpTo(f.ctx, 65); e != nil {
		t.Fatal(e)
	}
}
