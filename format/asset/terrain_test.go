package asset

import "testing"

func TestTerrainCapability(t *testing.T) {
	if err := (Capabilities{Runtime: []Capability{CapabilityWorldTerrainV1}}).Validate(); err != nil {
		t.Fatal(err)
	}
}
