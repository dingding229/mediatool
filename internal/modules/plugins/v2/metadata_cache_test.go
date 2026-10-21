package v2

import (
	"encoding/json"
	"sync"
	"testing"
)

func TestCompiledMetadataDetachedAcrossConcurrentCallers(t *testing.T) {
	list, err := Catalog()
	if err != nil || len(list) == 0 {
		t.Fatal(err)
	}
	id := list[0].Manifest.ID
	before, err := definition(id)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(before)
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range 10 {
				items, err := Catalog()
				if err != nil {
					t.Error(err)
					return
				}
				for i := range items {
					items[i].Manifest.Description = "caller-only"
					for j := range items[i].Manifest.Permissions {
						items[i].Manifest.Permissions[j].Scope = "caller-only"
					}
				}
				d, err := definition(id)
				if err != nil {
					t.Error(err)
					return
				}
				d.Manifest.Description = "mutated"
				for k := range d.Manifest.History {
					d.Manifest.History[k] = "caller-only"
				}
				for i := range d.Manifest.Extensions.Events {
					d.Manifest.Extensions.Events[i] = "caller-only"
				}
			}
		})
	}
	wg.Wait()
	after, err := definition(id)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := json.Marshal(after)
	if string(got) != string(raw) {
		t.Fatal("compiled metadata/fingerprint polluted by a caller")
	}
}
