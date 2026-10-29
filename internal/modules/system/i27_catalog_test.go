package system

import "testing"

func TestCatalogKeepsNativeAndCloudWithoutExternalGateway(t *testing.T) {
	native, cloud := false, false
	for _, module := range Modules() {
		if module.ID == "emby" {
			t.Fatal("retired external gateway advertised")
		}
		native = native || module.ID == "media-server"
		cloud = cloud || module.ID == "cloudStorage"
	}
	if !native || !cloud {
		t.Fatal("native media or cloud boundary removed")
	}
}
