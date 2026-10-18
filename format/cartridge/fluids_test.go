package cartridge_test

import (
	"slices"
	"testing"

	"github.com/karty-game/karty-sdk/format/asset"
	"github.com/karty-game/karty-sdk/format/cartridge"
)

func TestFluidFeatureDependencies(t *testing.T) {
	if err := (asset.Capabilities{Runtime: []asset.Capability{asset.CapabilityWorldFluidsV1}}).Validate(); err != nil {
		t.Fatal(err)
	}
	m := cartridge.Manifest{ProjectName: "fluids", Compiler: "go", Width: 640, Height: 360}
	valid := []string{
		cartridge.FeatureWorldSectorsV1,
		cartridge.FeatureWorldStaticSolidsV1,
		cartridge.FeatureWorldAnimationsV1,
		cartridge.FeatureWorldFluidsV1,
	}
	slices.Sort(valid)
	m.Features = valid
	if _, err := cartridge.EncodeManifest(m); err != nil {
		t.Fatal(err)
	}
	for index := range valid {
		m.Features = append(append([]string(nil), valid[:index]...), valid[index+1:]...)
		if valid[index] == cartridge.FeatureWorldFluidsV1 {
			continue
		}
		if _, err := cartridge.EncodeManifest(m); err == nil {
			t.Fatal("incomplete fluid dependencies accepted", m.Features)
		}
	}
}
