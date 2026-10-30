package cartridge

import (
	"slices"
	"testing"
)

func TestSavedReflectionFeatureRequiresReadyFixedWorld(t *testing.T) {
	t.Parallel()
	features := []string{
		FeatureWorldChunkReadyV1,
		FeatureWorldChunksV1,
		FeatureWorldFixedLightingV1,
		FeatureWorldLightmapsV1,
		FeatureWorldReflectionCubemapsV1,
		FeatureWorldSectorsV1,
	}
	slices.Sort(features)
	if !validManifestFeatures(features) {
		t.Fatal("complete saved reflection contract rejected")
	}
	for _, required := range []string{FeatureWorldChunkReadyV1, FeatureWorldFixedLightingV1} {
		without := slices.DeleteFunc(slices.Clone(features), func(feature string) bool { return feature == required })
		if validManifestFeatures(without) {
			t.Fatal("missing dependency accepted", required)
		}
	}
}
