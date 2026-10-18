package cartridge_test

import (
	"testing"

	"github.com/karty-game/karty-sdk/format/asset"
	"github.com/karty-game/karty-sdk/format/cartridge"
)

func TestVegetationCapabilityAndFeatureDependencies(t *testing.T) {
	t.Parallel()
	if err := (asset.Capabilities{Runtime: []asset.Capability{asset.CapabilityWorldVegetationV1, asset.CapabilityWorldVegetationV2, asset.CapabilityWorldVegetationV3, asset.CapabilityWorldVegetationV4, asset.CapabilityWorldVegetationV5}}).Validate(); err != nil {
		t.Fatal(err)
	}
	manifest := cartridge.Manifest{
		ProjectName: "vegetation",
		Compiler:    "go",
		Width:       640,
		Height:      360,
		Features:    []string{cartridge.FeatureWorldSectorsV1, cartridge.FeatureWorldVegetationV1},
	}
	data, err := cartridge.EncodeManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cartridge.DecodeManifest(data); err != nil {
		t.Fatal(err)
	}
	manifest.Features = []string{cartridge.FeatureWorldSectorsV1, cartridge.FeatureWorldVegetationV2}
	if _, err := cartridge.EncodeManifest(manifest); err != nil {
		t.Fatal(err)
	}
	manifest.Features = []string{cartridge.FeatureWorldSectorsV1, cartridge.FeatureWorldVegetationV3}
	if _, err := cartridge.EncodeManifest(manifest); err != nil {
		t.Fatal(err)
	}
	manifest.Features = []string{cartridge.FeatureWorldSectorsV1, cartridge.FeatureWorldVegetationV4}
	if _, err := cartridge.EncodeManifest(manifest); err != nil {
		t.Fatal(err)
	}
	manifest.Features = []string{cartridge.FeatureWorldSectorsV1, cartridge.FeatureWorldVegetationV5}
	if _, err := cartridge.EncodeManifest(manifest); err != nil {
		t.Fatal(err)
	}
	for _, features := range [][]string{{cartridge.FeatureWorldVegetationV5}, {cartridge.FeatureWorldVegetationV4}, {cartridge.FeatureWorldVegetationV3}, {cartridge.FeatureWorldVegetationV2}, {cartridge.FeatureWorldVegetationV1}, {cartridge.FeatureWorldVegetationV1, cartridge.FeatureWorldSectorsV1}, {cartridge.FeatureWorldSectorsV1, "world/vegetation@6"}} {
		manifest.Features = features
		if _, err := cartridge.EncodeManifest(manifest); err == nil {
			t.Fatalf("invalid feature set accepted: %v", features)
		}
	}
}
