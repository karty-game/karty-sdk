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

func TestSavedReflectionFeatureAllowsFixedGPUCellsWithoutLegacyReady(t *testing.T) {
	t.Parallel()
	features := []string{
		FeatureWorldChunksV1,
		FeatureWorldFixedLightingV1,
		FeatureWorldLightmapsV1,
		FeatureWorldReflectionCubemapsV1,
		FeatureWorldSectorsV1,
		FeatureWorldGPUChunksV1,
		FeatureWorldMaterialGPUV1,
		FeatureWorldMaterialAtlasV2,
	}
	slices.Sort(features)
	manifest := Manifest{ProjectName: "fixed-cell-reflections", Compiler: "test", Width: 64, Height: 64, Features: features}
	encoded, err := EncodeManifest(manifest)
	if err != nil {
		t.Fatal("fixed GPU-cell reflections require no legacy renderer feature", err)
	}
	decoded, err := DecodeManifest(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(decoded.Features, features) || slices.Contains(decoded.Features, FeatureWorldChunkReadyV1) {
		t.Fatal("round trip changed the declared renderer profile")
	}
	for _, required := range []string{
		FeatureWorldFixedLightingV1,
		FeatureWorldGPUChunksV1,
		FeatureWorldMaterialGPUV1,
		FeatureWorldSectorsV1,
	} {
		manifest.Features = slices.DeleteFunc(slices.Clone(features), func(feature string) bool { return feature == required })
		if _, err := EncodeManifest(manifest); err == nil {
			t.Fatal("missing fixed GPU reflection dependency accepted", required)
		}
	}
}

func TestPreparedReflectionRequiresSavedFixedGPUProfile(t *testing.T) {
	t.Parallel()
	features := []string{FeatureWorldChunksV1, FeatureWorldFixedLightingV1, FeatureWorldLightmapsV1,
		FeatureWorldReflectionCubemapsV1, FeatureWorldReflectionGPUV1, FeatureWorldSectorsV1,
		FeatureWorldGPUChunksV1, FeatureWorldMaterialGPUV1, FeatureWorldMaterialAtlasV2}
	slices.Sort(features)
	if !validManifestFeatures(features) {
		t.Fatal("complete prepared reflection profile rejected")
	}
	for _, required := range []string{FeatureWorldReflectionCubemapsV1, FeatureWorldGPUChunksV1, FeatureWorldFixedLightingV1} {
		without := slices.DeleteFunc(slices.Clone(features), func(feature string) bool { return feature == required })
		if validManifestFeatures(without) {
			t.Fatal("prepared reflection dependency omitted", required)
		}
	}
}
