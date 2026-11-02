package asset_test

import (
	"testing"

	"github.com/karty-game/karty-sdk/format/asset"
)

func TestPreparedGPUCapability(t *testing.T) {
	c := asset.Capabilities{Runtime: []asset.Capability{asset.CapabilityWorldMaterialGPUV1}}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	c.Runtime = append(c.Runtime, asset.CapabilityWorldMaterialGPUV1)
	if c.Validate() == nil {
		t.Fatal("duplicate capability accepted")
	}
}
