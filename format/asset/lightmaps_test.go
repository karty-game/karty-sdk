package asset_test

import (
	"testing"

	"github.com/karty-game/karty-sdk/format/asset"
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

func TestEnvironmentVariantsCapabilityVersion(t *testing.T) {
	if err := (asset.Capabilities{Runtime: []asset.Capability{asset.CapabilityWorldLightmapsEnvironmentVariantsV1}}).Validate(); err != nil {
		t.Fatal(err)
	}
	for _, capabilities := range []asset.Capabilities{
		{Runtime: []asset.Capability{"world/lightmaps-environment-variants@2"}},
		{Runtime: []asset.Capability{asset.CapabilityWorldLightmapsEnvironmentVariantsV1, asset.CapabilityWorldLightmapsEnvironmentVariantsV1}},
	} {
		if capabilities.Validate() == nil {
			t.Fatal("unsupported environment variants capability accepted")
		}
	}
}

func TestFixedLightingCapabilityVersion(t *testing.T) {
	t.Parallel()
	if err := (asset.Capabilities{Runtime: []asset.Capability{asset.CapabilityWorldFixedLightingV1}}).Validate(); err != nil {
		t.Fatal(err)
	}
	for _, capabilities := range []asset.Capabilities{
		{Runtime: []asset.Capability{"world/fixed-lighting@2"}},
		{Runtime: []asset.Capability{asset.CapabilityWorldFixedLightingV1, asset.CapabilityWorldFixedLightingV1}},
	} {
		if capabilities.Validate() == nil {
			t.Fatal("unsupported fixed lighting capability accepted")
		}
	}
}
