package worldchunk

import (
	"slices"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
)

func TestPhysicalCatalogPreservesBakedSourcesWithoutOwningRNMImages(t *testing.T) {
	t.Parallel()
	document, catalog, layouts := bakedCatalogFixture(t)
	catalog.Schema = PhysicalSchema
	for index := range catalog.Chunks {
		chunk := &catalog.Chunks[index]
		chunk.CollisionBytes = chunk.MeshBytes
		chunk.MeshBytes, chunk.LightmapBytes = 0, 0
		chunk.LightingBytes = 4096
	}
	encoded, err := EncodeCatalog(catalog, &document)
	if err != nil {
		t.Fatal("physical probes lost baked-source ownership", err)
	}
	decoded, err := DecodeCatalog(encoded, &document)
	if err != nil || !slices.Equal(decoded.BakedLights, catalog.BakedLights) {
		t.Fatal("baked sources dropped", err)
	}
	for index := range layouts {
		if err := ValidateBakedSelection(&decoded, index, &document, &layouts[index]); err != nil {
			t.Fatal("physical metadata recipe rejected", err)
		}
	}
	document.Lighting.Lights[1].Motion = &world.LightMotion{Version: 1, PeriodSeconds: 1}
	if ValidateCatalog(&decoded, &document) == nil {
		t.Fatal("dynamic light accepted as a physical baked source")
	}
}
