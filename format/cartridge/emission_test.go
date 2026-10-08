package cartridge_test

import (
	"reflect"
	"testing"

	"github.com/karty-game/karty-sdk/format/cartridge"
)

func TestEmissionFeatureRequiresWorldAndRoundTrips(t *testing.T) {
	t.Parallel()
	m := cartridge.Manifest{
		ProjectName: "emission",
		Compiler:    "tinygo",
		Width:       640,
		Height:      360,
		Features:    []string{cartridge.FeatureWorldEmissionV1, cartridge.FeatureWorldSectorsV1},
	}
	data, err := cartridge.EncodeManifest(m)
	if err != nil {
		t.Fatal(err)
	}
	got, err := cartridge.DecodeManifest(data)
	if err != nil || !reflect.DeepEqual(m, got) {
		t.Fatal("emission manifest drift", err)
	}
	m.Features = m.Features[:1]
	if _, err := cartridge.EncodeManifest(m); err == nil {
		t.Fatal("emission accepted without world")
	}
	m.Features = []string{"world/emission@2", cartridge.FeatureWorldSectorsV1}
	if _, err := cartridge.EncodeManifest(m); err == nil {
		t.Fatal("unknown emission version accepted")
	}
}
