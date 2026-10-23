package worldchunk

import (
	"math"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

func TestMeshCoverageKeepsOneULPParallelBoundariesDistinct(t *testing.T) {
	low := float32(.2)
	high := math.Nextafter32(low, float32(math.Inf(1)))
	polygon := []world.Vec3{{X: 46, Y: 24, Z: float64(low)}, {X: 46, Y: 20, Z: float64(low)},
		{X: 46, Y: 20, Z: float64(high)}, {X: 46, Y: 24, Z: float64(high)}}
	vertices := [][3]Vertex{
		{{Position: [3]float32{46, 22, high}}, {Position: [3]float32{46, 20, low}}, {Position: [3]float32{46, 24, low}}},
		{{Position: [3]float32{46, 22, high}}, {Position: [3]float32{46, 20, high}}, {Position: [3]float32{46, 20, low}}},
		{{Position: [3]float32{46, 22, high}}, {Position: [3]float32{46, 24, low}}, {Position: [3]float32{46, 24, high}}},
	}
	for _, mutation := range []string{"none", "missing wedge", "duplicate wedge", "gap and equal overlap"} {
		t.Run(mutation, func(t *testing.T) {
			coverage := meshCoverage{surface: worldlightmap.Surface{Normal: world.Vec3{X: 1}}, polygons: [][]world.Vec3{polygon}}
			for index, triangle := range vertices {
				if mutation == "missing wedge" && index == 1 {
					continue
				}
				if mutation == "gap and equal overlap" && index == 1 {
					triangle = vertices[2]
				}
				if err := coverage.addTriangle(triangle[:]); err != nil {
					t.Fatal(err)
				}
			}
			if mutation == "duplicate wedge" {
				if err := coverage.addTriangle(vertices[1][:]); err != nil {
					t.Fatal(err)
				}
			}
			var work cooperativeWork
			if err := coverage.validateBoundary(&work); (err == nil) != (mutation == "none") {
				t.Fatal("packed strip coverage accepted/rejected incorrectly", mutation, err)
			}
		})
	}
}

func TestMeshCompilerRetainsRoundedPortalSlivers(t *testing.T) {
	document := fixture(t)
	document.Sectors[0].Floor, document.Sectors[1].Floor = world.Plane{C: .2}, world.Plane{C: .20000001499999698}
	document.Sectors[0].Ceiling, document.Sectors[1].Ceiling = world.Plane{C: 3.2}, world.Plane{C: 3.1999999850000016}
	assertMeshOnlyCompilerParity(t, document, Options{MaxSectors: 1})
}
