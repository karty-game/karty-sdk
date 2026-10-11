package cartridge_test

import (
	"testing"

	"github.com/karty-game/karty-sdk/format/cartridge"
)

func TestReflectanceManifestRequiresWorldAndMaterialAtlas(t *testing.T) {
	t.Parallel()
	manifest := cartridge.Manifest{
		ProjectName: "reflectance",
		Compiler:    "fixture",
		Width:       64,
		Height:      48,
		Features: []string{
			cartridge.FeatureWorldMaterialAtlasV1,
			cartridge.FeatureWorldMaterialReflectanceV1,
			cartridge.FeatureWorldSectorsV1,
		},
	}
	if _, err := cartridge.EncodeManifest(manifest); err != nil {
		t.Fatal(err)
	}
	manifest.Features = manifest.Features[1:]
	if _, err := cartridge.EncodeManifest(manifest); err == nil {
		t.Fatal("reflectance without its atlas was accepted")
	}
}
