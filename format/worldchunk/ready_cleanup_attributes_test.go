package worldchunk

import (
	"math"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

func TestReadyCleanupInterpolatesEveryStreamWithinOriginalChart(t *testing.T) {
	source := []Vertex{{Position: [3]float32{0, 0, 1}}, {Position: [3]float32{4, 0, 3}}, {Position: [3]float32{0, 4, 2}}}
	for i := range source {
		x, y := source[i].Position[0], source[i].Position[1]
		for axis := range 3 {
			source[i].UV[axis] = [2]float32{x*2 + float32(axis), y*3 - float32(axis)}
			source[i].SecondaryUV[axis] = [2]float32{x/2 + float32(axis), y/4 - float32(axis)}
		}
		source[i].BandUV = [2]float32{x + 4, y + 8}
		source[i].WaveUV = [2]float32{x / 8, y / 16}
		source[i].LightmapUV = [2]float32{.25 + x/16, .5 + y/16}
	}
	actual := cleanupInterpolate(
		source,
		world.Vec3{X: 2, Y: 1, Z: 2.25},
		cleanupPlane{normal: world.Vec3{X: -.5, Y: -.25, Z: 1}, offset: -1},
	)
	expected := Vertex{
		Position:   [3]float32{2, 1, 2.25},
		BandUV:     [2]float32{6, 9},
		WaveUV:     [2]float32{.25, .0625},
		LightmapUV: [2]float32{.375, .5625},
	}
	for axis := range 3 {
		expected.UV[axis] = [2]float32{4 + float32(axis), 3 - float32(axis)}
		expected.SecondaryUV[axis] = [2]float32{1 + float32(axis), .25 - float32(axis)}
	}
	if actual != expected {
		t.Fatalf("new vertex changed inherited streams: %+v want %+v", actual, expected)
	}
	for _, v := range source {
		if got := cleanupInterpolate(source, unpacked(v.Position), cleanupPlane{normal: world.Vec3{Z: 1}}); got != v {
			t.Fatal("existing packed vertex was rewritten")
		}
	}
}

func TestReadyCleanupTriangulationPreservesCollinearBoundaryEdges(t *testing.T) {
	polygon := []world.Vec3{{}, {X: 1}, {X: 2}, {X: 2, Y: 2}, {X: 1, Y: 2}, {Y: 2}}
	triangles, ok := cleanupTriangulate(polygon, world.Vec3{Z: 1})
	if !ok {
		t.Fatal("valid convex boundary could not be triangulated")
	}
	edges := make(map[[2]world.Vec3]int)
	area := 0.0
	for _, tri := range triangles {
		v := cross(sub(tri[1], tri[0]), sub(tri[2], tri[0]))
		area += v.Z / 2
		for i, a := range tri {
			b := tri[(i+1)%3]
			reverse := [2]world.Vec3{b, a}
			if edges[reverse] > 0 {
				edges[reverse]--
			} else {
				edges[[2]world.Vec3{a, b}]++
			}
		}
	}
	if math.Abs(area-4) > 1e-12 {
		t.Fatal("triangulation left gap or overlap", area)
	}
	for i, a := range polygon {
		b := polygon[(i+1)%len(polygon)]
		if edges[[2]world.Vec3{a, b}] != 1 {
			t.Fatalf("lost authored/junction boundary edge %v -> %v", a, b)
		}
	}
}

func TestReadyCleanupRejectsMalformedCanonicalOwnerBeforeIndexing(t *testing.T) {
	doc, mesh, encoded, layout, ready := readyFixture(t, false)
	mesh.Triangles[0].Solid = len(doc.Sectors) + 100
	if err := ValidateReady(&ready, &mesh, encoded, &doc, layout, ReadyOptions{CleanupStatic: true}); err == nil {
		t.Fatal("malformed source owner accepted")
	}
}

func TestReadyCleanupPreservesExteriorViewsOfOpposingSectorContacts(t *testing.T) {
	document := cleanupTestDocument([6]float64{0, 1, 0, 1, 2, 1})
	mesh, err := CompileMesh(document, []int{0}, []int{0}, worldlightmap.Layout{})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := EncodeMesh(mesh, &document)
	if err != nil {
		t.Fatal(err)
	}
	ready, err := CompileReady(&mesh, encoded, &document, nil, ReadyOptions{CleanupStatic: true})
	if err != nil {
		t.Fatal(err)
	}
	before, after := cleanupRaySource(mesh), cleanupRayStreams(ready, world.MaterialIDs(&document))
	for _, ray := range [][2][3]float64{{{.5, 1.5, -1}, {0, 0, 1}}, {{-1, 1.5, .5}, {1, 0, 0}}} {
		want, got := cleanupRayNearest(before, ray[0], ray[1]), cleanupRayNearest(after, ray[0], ray[1])
		if want.distance != 1 || want.material != 2 || math.Abs(want.distance-got.distance) > 1e-7 || got.material != want.material ||
			got.normal != want.normal {
			t.Fatalf("opposing contact disappeared from exterior view: got %+v want %+v", got, want)
		}
	}
}

func TestReadyCleanupPreservesCameraInsideNoncollidingVolume(t *testing.T) {
	document := cleanupTestDocument([6]float64{1, 1, 0, 2, 2, 1})
	document.StaticSolids.Items[0].Collision = false
	mesh, err := CompileMesh(document, []int{0}, []int{0}, worldlightmap.Layout{})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := EncodeMesh(mesh, &document)
	if err != nil {
		t.Fatal(err)
	}
	ready, err := CompileReady(&mesh, encoded, &document, nil, ReadyOptions{CleanupStatic: true})
	if err != nil {
		t.Fatal(err)
	}
	eye, direction := [3]float64{1.5, 1.5, .5}, [3]float64{0, 0, -1}
	want := cleanupRayNearest(cleanupRaySource(mesh), eye, direction)
	got := cleanupRayNearest(cleanupRayStreams(ready, world.MaterialIDs(&document)), eye, direction)
	if want.distance != .5 || want.material != 1 || got.distance != want.distance || got.material != want.material {
		t.Fatalf("noncolliding occluder removed visible floor: got %+v want %+v", got, want)
	}
}
