package worldchunk

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
	"github.com/karty-game/karty-sdk/format/worldterrain"
)

func TestSpatialTerrainReplacementBindsExactSourceAndOwners(t *testing.T) {
	document := fixture(t)
	document.Sectors[0].Sky = true
	document.Sectors[0].Floor = world.Plane{}
	document.Sectors[0].Ceiling = world.Plane{C: 10}
	image := func(size int, normal bool) worldterrain.Image {
		p := make([]byte, size*size*4)
		for i := 0; i < len(p); i += 4 {
			p[i], p[i+1], p[i+2], p[i+3] = 50, 80, 40, 255
			if normal {
				p[i], p[i+1], p[i+2] = 128, 128, 255
			}
		}
		return worldterrain.Image{Width: uint16(size), Height: uint16(size), Pixels: p}
	}
	weights := image(2, false)
	for i := 0; i < len(weights.Pixels); i += 4 {
		copy(weights.Pixels[i:i+4], []byte{255, 0, 0, 0})
	}
	terrain := worldterrain.Document{
		Version:         worldterrain.Version,
		CellSize:        1,
		Columns:         2,
		Rows:            2,
		Heights:         make([]float32, 4),
		Normals:         [][3]float32{{0, 0, 1}, {0, 0, 1}, {0, 0, 1}, {0, 0, 1}},
		ExteriorSectors: []uint16{0},
		Holes:           []byte{0},
		Materials:       []worldterrain.Material{{Albedo: image(256, false), Normal: image(256, true), Scale: 2, NormalStrength: 1}},
		Patches:         []worldterrain.Patch{{Columns: 1, Rows: 1, Weights: weights}},
		FarColour:       image(2, false),
	}
	data, err := worldterrain.Encode(terrain)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	catalog := Catalog{
		Schema:        SpatialSchema,
		TerrainSHA256: hex.EncodeToString(sum[:]),
		Surfaces: []SurfaceOwnership{
			{Binding: worldlightmap.Binding{Kind: "sector-floor", Index: 0, Edge: -1, Chart: -1}, Kind: "terrain", Owner: 0},
		},
	}
	if err := ValidateSpatialTerrain(&catalog, &terrain, &document); err != nil {
		t.Fatal(err)
	}
	changed := catalog
	changed.TerrainSHA256 = ""
	if err := ValidateSpatialTerrain(&changed, &terrain, &document); err == nil {
		t.Fatal("missing terrain fingerprint accepted")
	}
	changed = catalog
	changed.Surfaces = nil
	if err := ValidateSpatialTerrain(&changed, &terrain, &document); err == nil {
		t.Fatal("missing terrain owner accepted")
	}
	if err := ValidateSpatialTerrain(&catalog, nil, &document); err == nil {
		t.Fatal("missing terrain source accepted")
	}
	terrain.Heights[0] = .1
	if err := ValidateSpatialTerrain(&catalog, &terrain, &document); err == nil {
		t.Fatal("changed terrain source accepted")
	}
}
