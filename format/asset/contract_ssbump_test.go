package asset_test

import (
	"testing"

	"github.com/karty-game/karty-sdk/format/asset"
)

func TestSSBumpOptionalCapability(t *testing.T) {
	c := asset.Capabilities{Runtime: []asset.Capability{asset.CapabilityWorldMaterialSSBumpV1}}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	c.Runtime = append(c.Runtime, asset.CapabilityWorldMaterialSSBumpV1)
	if c.Validate() == nil {
		t.Fatal("duplicate SSBump capability accepted")
	}
}
