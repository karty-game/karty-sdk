package cartridge_test

import (
	"reflect"
	"testing"

	"github.com/karty-game/karty-sdk/format/cartridge"
)

func TestAnimationFeatureRequiresWorldAndRoundTrips(t *testing.T) {
	m := cartridge.Manifest{
		ProjectName: "animations",
		Compiler:    "tinygo",
		Width:       640,
		Height:      360,
		Features:    []string{cartridge.FeatureWorldAnimationsV1, cartridge.FeatureWorldSectorsV1},
	}
	encoded, err := cartridge.EncodeManifest(m)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := cartridge.DecodeManifest(encoded)
	if err != nil || !reflect.DeepEqual(decoded, m) {
		t.Fatalf("animation feature round trip: %+v %v", decoded, err)
	}
	m.Features = m.Features[:1]
	if _, err := cartridge.EncodeManifest(m); err == nil {
		t.Fatal("animation feature accepted without world")
	}
	m.Features = []string{"world/animations@2", cartridge.FeatureWorldSectorsV1}
	if _, err := cartridge.EncodeManifest(m); err == nil {
		t.Fatal("unknown animation feature accepted")
	}
}
