package cartridge

import (
	"slices"
	"testing"
)

func TestCompactFixedLightingRequiresCurrentSpatialReadyContract(t *testing.T) {
	features := []string{FeatureWorldSectorsV1, FeatureWorldChunksV1, FeatureWorldChunkReadyV1,
		FeatureWorldLightmapsV1, FeatureWorldFixedLightingV1, FeatureWorldSpatialChunksV2, FeatureWorldFixedCompactLightingV1}
	slices.Sort(features)
	if !validManifestFeatures(features) {
		t.Fatal("complete compact lighting feature set rejected")
	}
	for _, required := range []string{FeatureWorldChunkReadyV1, FeatureWorldFixedLightingV1, FeatureWorldSpatialChunksV2} {
		without := slices.DeleteFunc(slices.Clone(features), func(feature string) bool { return feature == required })
		if validManifestFeatures(without) {
			t.Fatal("missing compact dependency accepted", required)
		}
	}
	oldSpatial := slices.Clone(features)
	for index, feature := range oldSpatial {
		if feature == FeatureWorldSpatialChunksV2 {
			oldSpatial[index] = FeatureWorldSpatialChunksV1
		}
	}
	slices.Sort(oldSpatial)
	if validManifestFeatures(oldSpatial) {
		t.Fatal("compact lighting admitted with obsolete spatial feature")
	}
}
