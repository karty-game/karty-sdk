package cartridge_test

import (
	"testing"

	"github.com/karty-game/karty-sdk/format/asset"
	"github.com/karty-game/karty-sdk/format/cartridge"
)

func TestExplicitEnvironmentCapabilityDependencies(t *testing.T) {
	t.Parallel()
	if err := (asset.Capabilities{Runtime: []asset.Capability{asset.CapabilityWorldEnvironmentV2}}).Validate(); err != nil {
		t.Fatal(err)
	}
	manifest := cartridge.Manifest{
		ProjectName: "test",
		Compiler:    "go",
		Width:       640,
		Height:      360,
		Features:    []string{cartridge.FeatureWorldEnvironmentV2},
	}
	if _, err := cartridge.EncodeManifest(manifest); err == nil {
		t.Fatal("appearance accepted without dependencies")
	}
	manifest.Features = []string{cartridge.FeatureWorldEnvironmentV1, cartridge.FeatureWorldEnvironmentV2, cartridge.FeatureWorldSectorsV1}
	if _, err := cartridge.EncodeManifest(manifest); err != nil {
		t.Fatal(err)
	}
}
