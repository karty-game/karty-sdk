package asset_test

import (
	"testing"

	"github.com/karty-game/karty-sdk/format/asset"
)

func TestStaticSolidsCapabilityCanonical(t *testing.T) {
	if err := (asset.Capabilities{Runtime: []asset.Capability{asset.CapabilityWorldStaticSolidsV1}}).Validate(); err != nil {
		t.Fatal(err)
	}
	for _, caps := range []asset.Capabilities{
		{Runtime: []asset.Capability{"world/static-solids@2"}},
		{Runtime: []asset.Capability{asset.CapabilityWorldStaticSolidsV1, asset.CapabilityWorldStaticSolidsV1}},
		{Processors: []asset.Processor{asset.Processor(asset.CapabilityWorldStaticSolidsV1)}},
	} {
		if err := caps.Validate(); err == nil {
			t.Fatalf("accepted unsupported declaration: %+v", caps)
		}
	}
}
