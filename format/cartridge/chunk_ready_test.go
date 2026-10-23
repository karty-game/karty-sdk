package cartridge_test

import (
	"slices"
	"testing"

	"github.com/karty-game/karty-sdk/format/cartridge"
)

func TestChunkReadyFeatureRequiresChunksAndSectors(t *testing.T) {
	t.Parallel()
	features := []string{cartridge.FeatureWorldChunkReadyV1, cartridge.FeatureWorldChunksV1, cartridge.FeatureWorldSectorsV1}
	slices.Sort(features)
	manifest := cartridge.Manifest{ProjectName: "ready", Compiler: "tinygo", Width: 640, Height: 360, Features: features}
	encoded, err := cartridge.EncodeManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := cartridge.DecodeManifest(encoded)
	if err != nil || !slices.Equal(decoded.Features, features) {
		t.Fatal("ready feature did not roundtrip", err)
	}
	for _, required := range []string{cartridge.FeatureWorldChunksV1, cartridge.FeatureWorldSectorsV1} {
		manifest.Features = slices.DeleteFunc(slices.Clone(features), func(feature string) bool { return feature == required })
		if _, err := cartridge.EncodeManifest(manifest); err == nil {
			t.Fatal("ready feature accepted without dependency", required)
		}
	}
	manifest.Features = slices.Clone(features)
	manifest.Features[0] = "world/chunk-ready@2"
	slices.Sort(manifest.Features)
	if _, err := cartridge.EncodeManifest(manifest); err == nil {
		t.Fatal("unsupported ready wire version accepted")
	}
}
