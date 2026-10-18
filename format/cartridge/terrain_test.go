package cartridge

import "testing"

func TestTerrainFeatureRequiresSectorWorld(t *testing.T) {
	if validManifestFeatures([]string{FeatureWorldTerrainV1}) {
		t.Fatal("terrain without sector capability accepted")
	}
	if !validManifestFeatures([]string{FeatureWorldSectorsV1, FeatureWorldTerrainV1}) {
		t.Fatal("terrain with sector capability rejected")
	}
}
