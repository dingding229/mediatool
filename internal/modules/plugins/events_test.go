package plugins

import "testing"

func TestExtensionDeliveryStatesMatchPersistedContract(t *testing.T) {
	states := []string{extensionDeliveryPending, extensionDeliverySucceeded, extensionDeliveryUnknown}
	if extensionDeliveryPending != "pending" || extensionDeliverySucceeded != "succeeded" || extensionDeliveryUnknown != "unknown" {
		t.Fatalf("extension delivery state contract changed: %v", states)
	}
}
