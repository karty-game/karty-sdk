package cartridge_test

import (
	"slices"
	"testing"

	"github.com/karty-game/karty-sdk/format/cartridge"
)

func TestPreparedMaterialFeatureRequiresWorldAndLayout(t *testing.T) {
	features := []string{cartridge.FeatureWorldMaterialAtlasV1, cartridge.FeatureWorldMaterialGPUV1, cartridge.FeatureWorldSectorsV1}
	slices.Sort(features)
	m := cartridge.Manifest{ProjectName: "test", Compiler: "test", Width: 64, Height: 64, Features: features}
	encoded, err := cartridge.EncodeManifest(m)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cartridge.DecodeManifest(encoded); err != nil {
		t.Fatal(err)
	}
	for _, remove := range []string{cartridge.FeatureWorldMaterialAtlasV1, cartridge.FeatureWorldSectorsV1} {
		m.Features = slices.DeleteFunc(slices.Clone(features), func(s string) bool { return s == remove })
		if _, err := cartridge.EncodeManifest(m); err == nil {
			t.Fatal("missing dependency accepted", remove)
		}
	}
}
