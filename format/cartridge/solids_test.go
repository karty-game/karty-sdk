package cartridge_test

import (
	"reflect"
	"testing"

	"github.com/karty-game/karty-sdk/format/cartridge"
)

func TestStaticSolidsCartridgeFeatureRoundTrip(t *testing.T) {
	manifest := cartridge.Manifest{
		ProjectName: "solids",
		Compiler:    "tinygo",
		Width:       640,
		Height:      360,
		Features:    []string{cartridge.FeatureWorldSectorsV1, cartridge.FeatureWorldStaticSolidsV1},
	}
	encoded, err := cartridge.EncodeManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := cartridge.DecodeManifest(encoded)
	if err != nil || !reflect.DeepEqual(decoded, manifest) {
		t.Fatalf("feature lost: %v", err)
	}
	manifest.Features[1] = "world/static-solids@2"
	if _, err := cartridge.EncodeManifest(manifest); err == nil {
		t.Fatal("unsupported version accepted")
	}
}
