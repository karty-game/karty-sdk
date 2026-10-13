package cartridge_test

import (
	"testing"

	"github.com/karty-game/karty-sdk/format/asset"
	"github.com/karty-game/karty-sdk/format/cartridge"
)

func TestEnvironmentCapabilityAndFeatureDependencies(t *testing.T) {
	t.Parallel()
	if err := (asset.Capabilities{Runtime: []asset.Capability{asset.CapabilityWorldEnvironmentV1}}).Validate(); err != nil {
		t.Fatal(err)
	}
	manifest := cartridge.Manifest{ProjectName: "environment", Compiler: "go", Width: 640, Height: 360,
		Features: []string{cartridge.FeatureWorldEnvironmentV1, cartridge.FeatureWorldSectorsV1}}
	encoded, err := cartridge.EncodeManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cartridge.DecodeManifest(encoded); err != nil {
		t.Fatal(err)
	}
	for _, features := range [][]string{
		{cartridge.FeatureWorldEnvironmentV1},
		{cartridge.FeatureWorldSectorsV1, cartridge.FeatureWorldEnvironmentV1},
		{cartridge.FeatureWorldEnvironmentV1, cartridge.FeatureWorldEnvironmentV1, cartridge.FeatureWorldSectorsV1},
		{"world/environment@2", cartridge.FeatureWorldSectorsV1},
	} {
		manifest.Features = features
		if _, err := cartridge.EncodeManifest(manifest); err == nil {
			t.Fatalf("accepted invalid features %+v", features)
		}
	}
}

func TestBackdropCapabilityRequiresEnvironmentAndSectors(t *testing.T) {
	t.Parallel()
	if err := (asset.Capabilities{Runtime: []asset.Capability{asset.CapabilityWorldEnvironmentBackdropV1}}).Validate(); err != nil {
		t.Fatal(err)
	}
	manifest := cartridge.Manifest{ProjectName: "backdrop", Compiler: "go", Width: 640, Height: 360}
	for _, features := range [][]string{
		{cartridge.FeatureWorldEnvironmentBackdropV1},
		{cartridge.FeatureWorldEnvironmentBackdropV1, cartridge.FeatureWorldSectorsV1},
		{cartridge.FeatureWorldEnvironmentBackdropV1, cartridge.FeatureWorldEnvironmentV1},
	} {
		manifest.Features = features
		if _, err := cartridge.EncodeManifest(manifest); err == nil {
			t.Fatal("incomplete backdrop dependencies accepted", features)
		}
	}
	manifest.Features = []string{
		cartridge.FeatureWorldEnvironmentBackdropV1,
		cartridge.FeatureWorldEnvironmentV1,
		cartridge.FeatureWorldSectorsV1,
	}
	encoded, err := cartridge.EncodeManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cartridge.DecodeManifest(encoded); err != nil {
		t.Fatal(err)
	}
}

func TestSkyCapabilityRequiresEnvironmentAndSectors(t *testing.T) {
	t.Parallel()
	if err := (asset.Capabilities{Runtime: []asset.Capability{asset.CapabilityWorldSkyV1}}).Validate(); err != nil {
		t.Fatal(err)
	}
	manifest := cartridge.Manifest{ProjectName: "sky", Compiler: "go", Width: 640, Height: 360}
	for _, features := range [][]string{
		{cartridge.FeatureWorldSkyV1},
		{cartridge.FeatureWorldSectorsV1, cartridge.FeatureWorldSkyV1},
		{cartridge.FeatureWorldEnvironmentV1, cartridge.FeatureWorldSkyV1},
	} {
		manifest.Features = features
		if _, err := cartridge.EncodeManifest(manifest); err == nil {
			t.Fatal("incomplete sky dependencies accepted", features)
		}
	}
	manifest.Features = []string{cartridge.FeatureWorldEnvironmentV1, cartridge.FeatureWorldSectorsV1, cartridge.FeatureWorldSkyV1}
	encoded, err := cartridge.EncodeManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cartridge.DecodeManifest(encoded); err != nil {
		t.Fatal(err)
	}
	if err := (asset.Capabilities{Runtime: []asset.Capability{"world/sky@2"}}).Validate(); err == nil {
		t.Fatal("unknown sky capability accepted")
	}
}
