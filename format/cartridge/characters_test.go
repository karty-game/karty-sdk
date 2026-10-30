package cartridge

import (
	"testing"

	"github.com/karty-game/karty-sdk/format/asset"
)

func TestCharacterFeatureRequiresWorld(t *testing.T) {
	t.Parallel()
	if err := (asset.Capabilities{Runtime: []asset.Capability{asset.CapabilityWorldCharactersV1}}).Validate(); err != nil {
		t.Fatal(err)
	}
	if validManifestFeatures([]string{FeatureWorldCharactersV1}) {
		t.Fatal("accepted characters without world")
	}
	if !validManifestFeatures([]string{FeatureWorldCharactersV1, FeatureWorldSectorsV1}) {
		t.Fatal("rejected world characters")
	}
	if validManifestFeatures([]string{"world/characters@2", FeatureWorldSectorsV1}) {
		t.Fatal("accepted unknown characters version")
	}
}
