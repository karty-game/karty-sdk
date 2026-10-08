package asset_test

import (
	"testing"

	"github.com/karty-game/karty-sdk/format/asset"
)

func TestAnimationCapabilityCanonical(t *testing.T) {
	if err := (asset.Capabilities{Runtime: []asset.Capability{asset.CapabilityWorldAnimationsV1}}).Validate(); err != nil {
		t.Fatal(err)
	}
	for _, caps := range []asset.Capabilities{{Runtime: []asset.Capability{"world/animations@2"}}, {Runtime: []asset.Capability{asset.CapabilityWorldAnimationsV1, asset.CapabilityWorldAnimationsV1}}} {
		if err := caps.Validate(); err == nil {
			t.Fatal("unsupported animation capability accepted")
		}
	}
}
