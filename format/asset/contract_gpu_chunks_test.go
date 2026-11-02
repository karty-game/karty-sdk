package asset_test

import (
	"testing"

	"github.com/karty-game/karty-sdk/format/asset"
)

func TestGPUCellCapabilityIsExplicitAndUnique(t *testing.T) {
	t.Parallel()
	for _, capability := range []asset.Capability{asset.CapabilityWorldGPUChunksV1, asset.CapabilityWorldGPUChunksV2} {
		c := asset.Capabilities{Runtime: []asset.Capability{capability}}
		if err := c.Validate(); err != nil {
			t.Fatal(err)
		}
		c.Runtime = append(c.Runtime, capability)
		if c.Validate() == nil {
			t.Fatal("duplicate GPU cell capability admitted")
		}
	}
}
