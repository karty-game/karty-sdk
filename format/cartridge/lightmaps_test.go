package cartridge_test

import (
	"slices"
	"testing"

	"github.com/karty-game/karty-sdk/format/cartridge"
)

func TestLightmapsFeatureVersion(t *testing.T) {
	manifest := cartridge.Manifest{
		ProjectName: "lightmaps",
		Compiler:    "tinygo",
		Width:       640,
		Height:      360,
		Features:    []string{cartridge.FeatureWorldLightmapsV1, cartridge.FeatureWorldSectorsV1},
	}
	encoded, err := cartridge.EncodeManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cartridge.DecodeManifest(encoded); err != nil {
		t.Fatal(err)
	}
	manifest.Features[0] = "world/lightmaps@2"
	if _, err := cartridge.EncodeManifest(manifest); err == nil {
		t.Fatal("accepted unsupported lightmap version")
	}
}

func TestPrebakedLightmapsRequiredFeatures(t *testing.T) {
	features := []string{cartridge.FeatureWorldSectorsV1, cartridge.FeatureWorldLightingV1,
		cartridge.FeatureWorldLightmapsV1, cartridge.FeatureWorldLightmapsPrebakedV1}
	slices.Sort(features)
	manifest := cartridge.Manifest{ProjectName: "prebake", Compiler: "tinygo", Width: 640, Height: 360, Features: features}
	encoded, err := cartridge.EncodeManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cartridge.DecodeManifest(encoded); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{cartridge.FeatureWorldSectorsV1, cartridge.FeatureWorldLightingV1, cartridge.FeatureWorldLightmapsV1} {
		manifest.Features = slices.DeleteFunc(slices.Clone(features), func(feature string) bool { return feature == required })
		if _, err := cartridge.EncodeManifest(manifest); err == nil {
			t.Fatalf("missing dependency %s accepted", required)
		}
	}
}
