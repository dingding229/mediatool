package library

import "testing"

func TestRefreshKeyIsStablePerAssetRevision(t *testing.T) {
	first := refreshKey("asset-1", 7)
	if first != "library.native.refresh:asset-1:7" {
		t.Fatalf("unexpected refresh key: %s", first)
	}
	if first != refreshKey("asset-1", 7) {
		t.Fatal("same asset revision must produce the same idempotency key")
	}
	if first == refreshKey("asset-1", 8) || first == refreshKey("asset-2", 7) {
		t.Fatal("different asset or revision reused the refresh key")
	}
}
