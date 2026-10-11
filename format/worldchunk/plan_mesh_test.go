package worldchunk

import (
	"bytes"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

func assertMeshOnlyCompilerParity(t *testing.T, document world.Document, options Options) PlanResult {
	t.Helper()
	options.NoLightmap = true
	plan, err := Plan(document, options)
	if err != nil {
		t.Fatal(err)
	}
	for _, chunk := range plan.Chunks {
		want, err := CompileMesh(document, chunk.Sectors, chunk.Solids, worldlightmap.Layout{})
		if err != nil {
			t.Fatal(err)
		}
		before, err := EncodeMesh(want, &document)
		if err != nil {
			t.Fatal(err)
		}
		after, err := EncodeMesh(chunk.Mesh, &document)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, after) || chunk.Bounds != want.Bounds || chunk.MeshBytes != DecodedMeshBytes(want) ||
			chunk.LightmapBytes != 0 || chunk.Layout.Schema != "" {
			t.Fatal("prepared geometry changed standalone compiler bytes, bounds or accounting")
		}
	}
	return plan
}

func TestMeshOnlyPreparedGeometryMatchesStandaloneCompiler(t *testing.T) {
	for name, makeDocument := range map[string]func(testing.TB) world.Document{
		"ordinary": fixture,
		"liquid":   liquidFixture,
		"solids and liquid": func(t testing.TB) world.Document {
			document := liquidFixture(t)
			document.StaticSolids = &world.StaticSolids{Version: 1, Items: []world.Solid{
				{ID: "second-room", Footprint: []world.Vec2{{X: 5, Y: 1}, {X: 6, Y: 1}, {X: 6, Y: 2}, {X: 5, Y: 2}},
					Bottom: world.Plane{C: .5}, Top: world.Plane{C: 1}, SideMaterial: 1, TopMaterial: 1, BottomMaterial: 1},
				{ID: "first-room", Footprint: []world.Vec2{{X: 3, Y: 1}, {X: 4, Y: 1}, {X: 4, Y: 2}, {X: 3, Y: 2}},
					Bottom: world.Plane{C: .5}, Top: world.Plane{C: 1}, SideMaterial: 1, TopMaterial: 1, BottomMaterial: 1},
			}}
			return document
		},
		"sky": func(t testing.TB) world.Document {
			document := fixture(t)
			document.Environment = &world.Environment{Version: 1, Preset: world.EnvironmentDaylight}
			document.Sectors[0].Sky = true
			document.Sectors[0].Walls[0].Sky = true
			return document
		},
		"quantized junctions": func(t testing.TB) world.Document {
			document := fixture(t)
			document.Sectors[1].Floor = document.Sectors[0].Floor
			document.Sectors[1].Floor.C += 1e-9
			document.Sectors[1].Ceiling = document.Sectors[0].Ceiling
			return document
		},
		"transformed": func(t testing.TB) world.Document {
			document := fixture(t)
			for i := range document.Sectors[1].Walls {
				document.Sectors[1].Walls[i].Start.X += 100
				document.Sectors[1].Walls[i].End.X += 100
			}
			document.Sectors[1].Floor.C -= document.Sectors[1].Floor.A * 100
			document.Sectors[1].Ceiling.C -= document.Sectors[1].Ceiling.A * 100
			document.Contents[0].Position.X += 100
			return document
		},
	} {
		t.Run(name, func(t *testing.T) {
			document := makeDocument(t)
			for _, options := range []Options{{}, {MaxSectors: 1}, {SectorGroups: []int{5, 8}}} {
				assertMeshOnlyCompilerParity(t, document, options)
			}
		})
	}
}

func TestMeshOnlyExactEncodedBudgetAndFinalValidation(t *testing.T) {
	document := fixture(t)
	whole := assertMeshOnlyCompilerParity(t, document, Options{})
	encoded, err := EncodeMesh(whole.Chunks[0].Mesh, &document)
	if err != nil {
		t.Fatal(err)
	}
	for _, budget := range []uint64{uint64(len(encoded)), uint64(len(encoded) - 1)} {
		plan := assertMeshOnlyCompilerParity(t, document, Options{MaxEncodedBytes: budget})
		want := 1
		if budget < uint64(len(encoded)) {
			want = 2
		}
		if len(plan.Chunks) != want {
			t.Fatal("binary header or ownership bytes omitted from exact growth budget")
		}
	}
	// Mutating the last emitted triangle still fails the unchanged complete
	// validator, even though growth trials do not revalidate prepared arrays.
	mesh := whole.Chunks[0].Mesh
	mesh.Triangles = slices.Clone(mesh.Triangles)
	mesh.Triangles[len(mesh.Triangles)-1].Material = 999
	if _, err := EncodeMesh(mesh, &document); err == nil {
		t.Fatal("prepared final mesh bypassed semantic validation")
	}
	document.Sectors[len(document.Sectors)-1].Ceiling.C = -100
	if _, err := Plan(document, Options{NoLightmap: true}); err == nil {
		t.Fatal("mesh-only preparation bypassed full public input validation")
	}
}

func TestMeshOnlySolidScopeCachePreservesAmbiguityAndOrphanRejection(t *testing.T) {
	for _, orphan := range []bool{false, true} {
		document := fixture(t)
		for edge := range document.Sectors[1].Walls {
			document.Sectors[1].Walls[edge].Start.X += 100
			document.Sectors[1].Walls[edge].End.X += 100
		}
		document.Sectors[1].Floor.C -= document.Sectors[1].Floor.A * 100
		document.Sectors[1].Ceiling.C -= document.Sectors[1].Ceiling.A * 100
		document.Contents[0].Position.X += 100
		document.StaticSolids = &world.StaticSolids{Version: 1, Items: []world.Solid{
			{ID: "ordinary", Footprint: []world.Vec2{{X: 1, Y: 1}, {X: 2, Y: 1}, {X: 2, Y: 2}, {X: 1, Y: 2}},
				Bottom: world.Plane{C: .5}, Top: world.Plane{C: 1}, SideMaterial: 3, TopMaterial: 3, BottomMaterial: 3},
			{ID: "invalid", Footprint: []world.Vec2{{X: 1, Y: 1}, {X: 106, Y: 1}, {X: 106, Y: 2}, {X: 1, Y: 2}},
				Bottom: world.Plane{C: .5}, Top: world.Plane{C: 1}, SideMaterial: 3, TopMaterial: 3, BottomMaterial: 3},
		}}
		want := "touches multiple physical spaces"
		if orphan {
			want = "no receiver space"
			for index := range document.StaticSolids.Items[1].Footprint {
				document.StaticSolids.Items[1].Footprint[index].Y += 100
			}
		}
		if _, err := Plan(document, Options{NoLightmap: true}); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("invalid solid scope accepted after component caching: %v", err)
		}
	}
}

func TestMeshOnlyWorldMayExceedOneMeshEntry(t *testing.T) {
	document := fixture(t)
	document.StaticSolids = &world.StaticSolids{Version: 1}
	for sector := range document.Sectors {
		for item := range 64 {
			points := make([]world.Vec2, 24)
			for edge := range points {
				angle := float64(edge) * 2 * math.Pi / float64(len(points))
				points[edge] = world.Vec2{X: float64(sector)*4 + 2 + math.Cos(angle), Y: 2 + math.Sin(angle)}
			}
			document.StaticSolids.Items = append(document.StaticSolids.Items, world.Solid{
				ID: fmt.Sprintf("volume-%d-%d", sector, item), Footprint: points,
				Bottom: world.Plane{C: .5}, Top: world.Plane{C: 1}, SideMaterial: 3, TopMaterial: 3, BottomMaterial: 3,
			})
		}
	}
	plan, err := Plan(document, Options{NoLightmap: true, MaxMeshBytes: 64 * 1024 * 1024})
	if err != nil || len(plan.Chunks) != 2 {
		t.Fatalf("world geometry incorrectly required to fit one encoded entry: %v", err)
	}
	var total uint64
	for _, chunk := range plan.Chunks {
		total += chunk.MeshBytes
		if chunk.MeshBytes >= MaxEncodedSize {
			t.Fatal("final child exceeds the existing entry bound")
		}
	}
	if total <= MaxEncodedSize {
		t.Fatal("fixture did not exceed an entire mesh entry")
	}
}
