package cartridge_test

import (
	"slices"
	"testing"

	"github.com/karty-game/karty-sdk/format/cartridge"
)

func TestGPUCellsAllowLiveOnlyButRequirePreparedSharedMaterials(t *testing.T) {
	t.Parallel()
	for _, capability := range []string{cartridge.FeatureWorldGPUChunksV1, cartridge.FeatureWorldGPUChunksV2} {
		features := []string{
			capability,
			cartridge.FeatureWorldMaterialGPUV1,
			cartridge.FeatureWorldMaterialAtlasV1,
			cartridge.FeatureWorldSectorsV1,
		}
		slices.Sort(features)
		m := cartridge.Manifest{ProjectName: "test", Compiler: "test", Width: 64, Height: 64, Features: features}
		encoded, err := cartridge.EncodeManifest(m)
		if err != nil {
			t.Fatal("live-only cell profile rejected", err)
		}
		if _, err = cartridge.DecodeManifest(encoded); err != nil {
			t.Fatal(err)
		}
		for _, required := range []string{cartridge.FeatureWorldMaterialGPUV1, cartridge.FeatureWorldSectorsV1} {
			m.Features = slices.DeleteFunc(slices.Clone(features), func(value string) bool { return value == required })
			if _, err := cartridge.EncodeManifest(m); err == nil {
				t.Fatal("missing shared profile dependency admitted", required)
			}
		}
	}
}
