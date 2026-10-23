package asset_test

import (
	"testing"

	"github.com/karty-game/karty-sdk/format/asset"
)

func TestChunkReadyRuntimeCapabilityIsVersionedAndNotAProcessor(t *testing.T) {
	t.Parallel()
	if err := (asset.Capabilities{Runtime: []asset.Capability{asset.CapabilityWorldChunkReadyV1}}).Validate(); err != nil {
		t.Fatal(err)
	}
	for _, capabilities := range []asset.Capabilities{
		{Runtime: []asset.Capability{"world/chunk-ready@2"}},
		{Runtime: []asset.Capability{asset.CapabilityWorldChunkReadyV1, asset.CapabilityWorldChunkReadyV1}},
		{Processors: []asset.Processor{asset.Processor(asset.CapabilityWorldChunkReadyV1)}},
	} {
		if err := capabilities.Validate(); err == nil {
			t.Fatal("invalid ready capability accepted", capabilities)
		}
	}
}
