package worldchunk

import (
	"bytes"
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

func cleanupTestDocument(boxes ...[6]float64) world.Document {
	points := []world.Vec2{{}, {X: 8}, {X: 8, Y: 8}, {Y: 8}}
	sector := world.Sector{ID: "room", SourceRoom: "room", Ceiling: world.Plane{C: 4}, FloorMaterial: 1, CeilingMaterial: 1}
	for i, p := range points {
		sector.Walls = append(sector.Walls, world.Wall{Start: p, End: points[(i+1)%4], Material: 1, Portal: -1})
	}
	doc := world.Document{Version: world.Version, Sectors: []world.Sector{sector}, StaticSolids: &world.StaticSolids{Version: 1}}
	for i, b := range boxes {
		doc.StaticSolids.Items = append(
			doc.StaticSolids.Items,
			world.Solid{
				ID:             fmt.Sprintf("box-%d", i),
				Collision:      true,
				Footprint:      []world.Vec2{{X: b[0], Y: b[1]}, {X: b[3], Y: b[1]}, {X: b[3], Y: b[4]}, {X: b[0], Y: b[4]}},
				Bottom:         world.Plane{C: b[2]},
				Top:            world.Plane{C: b[5]},
				TopMaterial:    2,
				BottomMaterial: 2,
				SideMaterial:   2,
			},
		)
	}
	return doc
}

func cleanupTestMeshes(t testing.TB, doc world.Document, opts ReadyOptions) (Mesh, Mesh) {
	t.Helper()
	solids := make([]int, len(doc.StaticSolids.Items))
	for i := range solids {
		solids[i] = i
	}
	original, err := CompileMesh(doc, []int{0}, solids, worldlightmap.Layout{})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := EncodeMesh(original, &doc)
	if err != nil {
		t.Fatal(err)
	}
	materials, _, err := readyMaterials(&doc, opts)
	if err != nil {
		t.Fatal(err)
	}
	derived, err := cleanupReadyMesh(&original, &doc, materials)
	if err != nil {
		t.Fatal(err)
	}
	opts.CleanupStatic = true
	ready, err := CompileReady(&original, encoded, &doc, nil, opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateReady(&ready, &original, encoded, &doc, nil, opts); err != nil {
		t.Fatal(err)
	}
	after, err := EncodeMesh(original, &doc)
	if err != nil || !bytes.Equal(encoded, after) {
		t.Fatal("cleanup mutated canonical source", err)
	}
	var count int
	for _, b := range ready.Batches {
		count += len(b.Sources)
	}
	if count != len(derived.Triangles) {
		t.Fatal("derived count differs from emitted sources")
	}
	return original, derived
}

func cleanupTestArea(mesh Mesh, selected func(Triangle) bool) float64 {
	area := 0.0
	for i, tri := range mesh.Triangles {
		if !selected(tri) {
			continue
		}
		a, b, c := unpacked(mesh.Vertices[i*3].Position), unpacked(mesh.Vertices[i*3+1].Position), unpacked(mesh.Vertices[i*3+2].Position)
		v := cross(sub(b, a), sub(c, a))
		area += math.Sqrt(dot(v, v)) / 2
	}
	return area
}

func TestReadyCleanupStaticContactsAndOverlapArea(t *testing.T) {
	for _, tc := range []struct {
		name         string
		boxes        [][6]float64
		floor, solid float64
	}{
		{"grounded", [][6]float64{{1, 1, 0, 2, 2, 1}}, 63, 6},
		{"touching", [][6]float64{{1, 1, 0, 2, 2, 1}, {2, 1, 0, 3, 2, 1}}, 62, 10},
		{"overlapping", [][6]float64{{1, 1, 0, 3, 3, 1}, {2, 1, 0, 4, 3, 1}}, 58, 22},
		{"duplicate", [][6]float64{{1, 1, 0, 2, 2, 1}, {1, 1, 0, 2, 2, 1}}, 63, 6},
		{"wall-contact", [][6]float64{{0, 1, 0, 1, 2, 1}}, 63, 6},
		{"positive-floor-gap", [][6]float64{{1, 1, .000001, 2, 2, 1}}, 64, 5.999996},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before, after := cleanupTestMeshes(t, cleanupTestDocument(tc.boxes...), ReadyOptions{})
			floor := cleanupTestArea(after, func(tri Triangle) bool { return tri.Surface == "sector-floor" })
			solid := cleanupTestArea(after, func(tri Triangle) bool { return tri.Solid >= 0 })
			if math.Abs(floor-tc.floor) > 1e-5 || math.Abs(solid-tc.solid) > 1e-5 {
				t.Fatalf("visible floor/solid area %.9f/%.9f want %.9f/%.9f", floor, solid, tc.floor, tc.solid)
			}
			t.Logf(
				"triangles %d -> %d; floor %.6f -> %.6f; solid %.6f -> %.6f",
				len(before.Triangles),
				len(after.Triangles),
				cleanupTestArea(before, func(v Triangle) bool { return v.Surface == "sector-floor" }),
				floor,
				cleanupTestArea(before, func(v Triangle) bool { return v.Solid >= 0 }),
				solid,
			)
		})
	}
}

func TestReadyCleanupKeepsFirstCoplanarMaterial(t *testing.T) {
	doc := cleanupTestDocument([6]float64{1, 1, -1, 2, 2, 0})
	_, after := cleanupTestMeshes(t, doc, ReadyOptions{})
	if got := cleanupTestArea(after, func(v Triangle) bool { return v.Surface == "sector-floor" }); got != 64 {
		t.Fatal("later solid replaced earlier differently materialed floor", got)
	}
	if got := cleanupTestArea(after, func(v Triangle) bool { return v.Surface == "solid-top" }); got != 0 {
		t.Fatal("same-facing later duplicate remained", got)
	}
}

func TestReadyCleanupExcludesMaskedAnimatedAndOtherChunkOwners(t *testing.T) {
	for _, mode := range []string{"masked", "animated", "noncolliding", "other-chunk"} {
		t.Run(mode, func(t *testing.T) {
			doc := cleanupTestDocument([6]float64{1, 1, 0, 2, 2, 1})
			opts := ReadyOptions{}
			if mode == "noncolliding" {
				doc.StaticSolids.Items[0].Collision = false
			}
			if mode == "masked" {
				opts.Masked = []bool{false, true}
			}
			if mode == "animated" {
				doc.Animations = &world.Animations{
					Version:   1,
					Presets:   []world.AnimationPreset{{Name: "flow", Kind: "liquid", Frequency: 1, SurfaceAmplitude: .1}},
					Materials: []world.MaterialAnimation{{Material: 2, Preset: "flow"}},
				}
			}
			if mode == "other-chunk" {
				mesh, err := CompileMesh(doc, []int{0}, nil, worldlightmap.Layout{})
				if err != nil {
					t.Fatal(err)
				}
				materials, _, err := readyMaterials(&doc, opts)
				if err != nil {
					t.Fatal(err)
				}
				derived, err := cleanupReadyMesh(&mesh, &doc, materials)
				if err != nil || !reflect.DeepEqual(mesh, derived) {
					t.Fatal("nonresident occluder cut resident floor", err)
				}
				return
			}
			before, after := cleanupTestMeshes(t, doc, opts)
			if !reflect.DeepEqual(before, after) {
				t.Fatal("unsafe material participated in static cuts")
			}
		})
	}
}
