package asset_test

import (
	"github.com/karty-game/karty-sdk/format/asset"
	"testing"
)

func TestLightmapsCapabilityVersion(t *testing.T) {
	if err := (asset.Capabilities{Runtime: []asset.Capability{asset.CapabilityWorldLightmapsV1}}).Validate(); err != nil {
		t.Fatal(err)
	}
	for _, caps := range []asset.Capabilities{{Runtime: []asset.Capability{"world/lightmaps@2"}}, {Runtime: []asset.Capability{asset.CapabilityWorldLightmapsV1, asset.CapabilityWorldLightmapsV1}}, {Processors: []asset.Processor{asset.Processor(asset.CapabilityWorldLightmapsV1)}}} {
		if err := caps.Validate(); err == nil {
			t.Fatal("accepted unsupported declaration")
		}
	}
}

func TestPrebakedLightmapsCapabilityVersion(t *testing.T) {
	if err := (asset.Capabilities{Runtime: []asset.Capability{asset.CapabilityWorldLightmapsPrebakedV1}}).Validate(); err != nil {
		t.Fatal(err)
	}
	for _, capabilities := range []asset.Capabilities{
		{Runtime: []asset.Capability{"world/lightmaps-prebaked@2"}},
		{Runtime: []asset.Capability{asset.CapabilityWorldLightmapsPrebakedV1, asset.CapabilityWorldLightmapsPrebakedV1}},
	} {
		if err := capabilities.Validate(); err == nil {
			t.Fatal("unsupported prebake capability accepted")
		}
	}
}
