package cartridge_test

import (
	"testing"

	"github.com/karty-game/karty-sdk/format/cartridge"
)

func TestSSBumpFeatureRequiresWorldAndAtlas(t *testing.T) {
	for _, features := range [][]string{
		{cartridge.FeatureWorldMaterialSSBumpV1},
		{cartridge.FeatureWorldMaterialAtlasV1, cartridge.FeatureWorldMaterialSSBumpV1},
		{cartridge.FeatureWorldMaterialSSBumpV1, cartridge.FeatureWorldSectorsV1},
	} {
		m := cartridge.Manifest{ProjectName: "test", Compiler: "go", Width: 320, Height: 240, Features: features}
		if _, err := cartridge.EncodeManifest(m); err == nil {
			t.Fatal("missing SSBump dependency accepted", features)
		}
	}
	for _, atlas := range []string{cartridge.FeatureWorldMaterialAtlasV1, cartridge.FeatureWorldMaterialAtlasV2} {
		m := cartridge.Manifest{
			ProjectName: "test",
			Compiler:    "go",
			Width:       320,
			Height:      240,
			Features:    []string{atlas, cartridge.FeatureWorldMaterialSSBumpV1, cartridge.FeatureWorldSectorsV1},
		}
		b, err := cartridge.EncodeManifest(m)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := cartridge.DecodeManifest(b); err != nil {
			t.Fatal(err)
		}
	}
}
