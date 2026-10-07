package asset_test

import (
	"testing"

	"github.com/karty-game/karty-sdk/format/asset"
)

func TestAdvancedMaterialCapabilities(t *testing.T) {
	t.Parallel()
	if err := (asset.Capabilities{Runtime: []asset.Capability{asset.CapabilityWorldMaterialAtlasV2, asset.CapabilityWorldMaterialLayersV1}}).Validate(); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []asset.Capabilities{
		{Runtime: []asset.Capability{asset.CapabilityWorldMaterialLayersV1, asset.CapabilityWorldMaterialAtlasV2}},
		{Runtime: []asset.Capability{asset.CapabilityWorldMaterialLayersV1, asset.CapabilityWorldMaterialLayersV1}},
		{Runtime: []asset.Capability{"world/material-layers@2"}},
		{Processors: []asset.Processor{asset.Processor(asset.CapabilityWorldMaterialAtlasV2)}},
	} {
		if bad.Validate() == nil {
			t.Fatal("accepted invalid advanced capability list")
		}
	}
}
