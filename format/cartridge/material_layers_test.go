package cartridge_test

import (
	"reflect"
	"testing"

	"github.com/karty-game/karty-sdk/format/cartridge"
)

func TestAdvancedMaterialsCartridgeCapabilityRoundTrip(t *testing.T) {
	t.Parallel()
	m := cartridge.Manifest{
		ProjectName: "advanced",
		Compiler:    "tinygo",
		Width:       640,
		Height:      360,
		Features: []string{
			cartridge.FeatureWorldMaterialAtlasV2,
			cartridge.FeatureWorldMaterialLayersV1,
			cartridge.FeatureWorldMaterialMappingV1,
			cartridge.FeatureWorldSectorsV1,
		},
	}
	b, err := cartridge.EncodeManifest(m)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := cartridge.DecodeManifest(b)
	if err != nil || !reflect.DeepEqual(decoded.Features, m.Features) {
		t.Fatalf("advanced features roundtrip: %v", err)
	}
}
