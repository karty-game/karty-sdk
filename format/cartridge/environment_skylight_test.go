package cartridge_test

import (
	"slices"
	"testing"

	"github.com/karty-game/karty-sdk/format/cartridge"
)

func TestSkylightRequiresEnvironmentAndSectors(t *testing.T) {
	t.Parallel()
	for _, dependencies := range [][]string{nil, {cartridge.FeatureWorldEnvironmentV1}, {cartridge.FeatureWorldSectorsV1}} {
		features := append(slices.Clone(dependencies), cartridge.FeatureWorldEnvironmentSkylightV1)
		slices.Sort(features)
		if _, err := cartridge.EncodeManifest(
			cartridge.Manifest{ProjectName: "sky", Compiler: "tinygo", Width: 64, Height: 64, Features: features},
		); err == nil {
			t.Fatal("skylight accepted incomplete dependencies")
		}
	}
	features := []string{cartridge.FeatureWorldEnvironmentSkylightV1, cartridge.FeatureWorldEnvironmentV1, cartridge.FeatureWorldSectorsV1}
	slices.Sort(features)
	encoded, err := cartridge.EncodeManifest(
		cartridge.Manifest{ProjectName: "sky", Compiler: "tinygo", Width: 64, Height: 64, Features: features},
	)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := cartridge.DecodeManifest(encoded)
	if err != nil || !slices.Equal(decoded.Features, features) {
		t.Fatal("skylight capability round trip", err)
	}
}
