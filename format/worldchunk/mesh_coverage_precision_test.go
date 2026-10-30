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

func TestMeshCompilerSubdividesStoredSlopedCorner(t *testing.T) {
	// Nearby clipped solids can contribute several junctions around a corner
	// that straddles a float32 Z rounding boundary. Interpolating the original
	// endpoints makes the packed boundary travel down/up/down at identical XY,
	// producing overlapping fan wedges when their winding is corrected.
	polygon := []world.Vec3{
		{X: -42.118506047, Y: 14.57350549, Z: 5.8136},
		{X: -41.921583661, Y: 14.394601678, Z: 5.8136},
		{X: -41.921583661, Y: 14.394601678, Z: 6.338600016533135},
		{X: -42.118506047, Y: 14.57350549, Z: 6.886070017124226},
	}
	points := append([]world.Vec3(nil), polygon...)
	points = append(points,
		world.Vec3{X: -42.11850604042712, Y: 14.57350548402855, Z: 6.886069998850776},
		world.Vec3{X: -42.11850604179666, Y: 14.573505485272774, Z: 6.886070002658269},
		world.Vec3{X: -42.118506047, Y: 14.57350549, Z: 6.886069999241877},
		world.Vec3{X: -42.118506047, Y: 14.57350549, Z: 6.886069974156364},
	)
	normal := world.Vec3{X: -0.6724329342312133, Y: -0.7401580567427477}
	mapping := &world.SurfaceUV{Projections: []world.UVProjection{{U: world.UVPlane{X: 1}, V: world.UVPlane{Z: 1}}}, Weights: []float64{1}}
	mesh := Mesh{}
	template := Triangle{Surface: "solid-side", Normal: packed(normal), Chart: -1}
	if err := emitPolygon(
		&mesh,
		&world.Document{},
		worldlightmap.Layout{},
		newPointIndex(points),
		polygon,
		template,
		mapping,
		nil,
		nil,
	); err != nil {
		t.Fatal(err)
	}
	if len(mesh.Triangles) != 4 {
		t.Fatalf("stored quad must retain four boundary wedges, got %d", len(mesh.Triangles))
	}
	for _, point := range polygon {
		found := false
		for _, vertex := range mesh.Vertices {
			found = found || vertex.Position == packed(point)
		}
		if !found {
			t.Fatalf("authored corner was lost: %v", point)
		}
	}
	for _, missingWedge := range []bool{false, true} {
		coverage := meshCoverage{surface: worldlightmap.Surface{Normal: normal}, polygons: [][]world.Vec3{polygon}}
		for index := range mesh.Triangles {
			if missingWedge && index == 0 {
				continue
			}
			if err := coverage.addTriangle(mesh.Vertices[index*3 : index*3+3]); err != nil {
				t.Fatal(err)
			}
		}
		var work cooperativeWork
		if err := coverage.validateBoundary(&work); (err == nil) == missingWedge {
			t.Fatalf("sloped receiver coverage accepted/rejected incorrectly: missing=%v: %v", missingWedge, err)
		}
	}
}
