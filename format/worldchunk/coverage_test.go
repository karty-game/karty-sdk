package worldchunk

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
)

func TestChunkBoundaryRetainsUnselectedSolidJunction(t *testing.T) {
	document := fixture(t)
	for i := range document.Sectors {
		document.Sectors[i].Floor = world.Plane{}
		document.Sectors[i].Ceiling = world.Plane{C: 4}
	}
	document.StaticSolids = &world.StaticSolids{
		Version: 1,
		Items: []world.Solid{
			{
				ID:             "junction",
				Footprint:      []world.Vec2{{X: 3, Y: 1}, {X: 4, Y: 1}, {X: 4, Y: 2}, {X: 3, Y: 2}},
				Top:            world.Plane{C: 1},
				SideMaterial:   1,
				TopMaterial:    1,
				BottomMaterial: 1,
			},
		},
	}
	plan, err := Plan(document, Options{NoLightmap: true, MaxSectors: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, chunk := range plan.Chunks {
		found := false
		for i, triangle := range chunk.Mesh.Triangles {
			if triangle.Surface != "sector-floor" {
				continue
			}
			for _, vertex := range chunk.Mesh.Vertices[i*3 : i*3+3] {
				found = found || vertex.Position == [3]float32{4, 2, 0}
			}
		}
		if !found {
			t.Fatal("boundary omitted neighboring chunk's collinear junction")
		}
	}
}

func TestChunkMappingFramesAndSky(t *testing.T) {
	document := fixture(t)
	document.MaterialMapping = &world.MaterialMapping{Version: 1}
	for si := range document.Sectors {
		s := &document.Sectors[si]
		s.FloorUV = &world.SurfaceUV{
			Projections: []world.UVProjection{{U: world.UVPlane{Y: 1}, V: world.UVPlane{X: -1}}},
			Weights:     []float64{1},
		}
		s.CeilingUV = s.FloorUV
		for edge := range s.Walls {
			s.Walls[edge].UV = legacyMapping(&document, Triangle{Sector: si, Surface: "sector-wall", SurfaceEdge: edge})
		}
	}
	document.MaterialLayers = &world.MaterialLayers{Version: 1}
	document.Sectors[0].FloorSecondary = &world.SurfaceSecondary{Material: 7, Strength: .5, UV: document.Sectors[0].FloorUV}
	profile, err := world.ProfileForWall(&document, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	regions, err := world.CompileWallFrame(
		document.Sectors[0].Walls[0],
		profile,
		world.WallFrameConfig{
			Top: world.FrameHorizontal{Height: 1, Repeat: 1, Piece: world.FramePiece{Material: 8, Coverage: world.FrameCoverageOpaque}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	document.Sectors[0].Walls[0].FrameRegions = regions
	document.Sectors[0].Walls[0].FrameCompiled = true
	assertMeshOnlyCompilerParity(t, document, Options{})
	plan, err := Plan(document, Options{NoLightmap: true, MaxSectors: 1})
	if err != nil {
		t.Fatal(err)
	}
	frame, floor := false, false
	for _, triangle := range plan.Chunks[0].Mesh.Triangles {
		if triangle.Surface == "sector-floor" {
			floor = true
			if triangle.Tangent[1] < .99 || triangle.Bitangent[0] > -.99 || triangle.Secondary.Material != 7 ||
				triangle.Secondary.Strength != .5 {
				t.Fatal("authored tangent/secondary changed")
			}
		}
		frame = frame || triangle.Band.Material == 8 && triangle.Band.Coverage == 2
	}
	if !frame || !floor {
		t.Fatal("compiled frame or floor missing")
	}
	stale := plan.Chunks[0].Mesh
	document.Sectors[0].FloorUV.Projections[0].U.Offset = .25
	if err := ValidateMesh(&stale, &document); err == nil {
		t.Fatal("stale projection digest accepted")
	}
	sky := fixture(t)
	sky.Environment = &world.Environment{Version: 1, Preset: world.EnvironmentDaylight}
	sky.Sectors[0].Sky = true
	sky.Sectors[0].Walls[0].Sky = true
	plan, err = Plan(sky, Options{NoLightmap: true, MaxSectors: 1})
	if err != nil {
		t.Fatal(err)
	}
	ceiling, horizon := false, false
	for _, triangle := range plan.Chunks[0].Mesh.Triangles {
		ceiling = ceiling || triangle.Sky && triangle.Surface == "sector-ceiling"
		horizon = horizon || triangle.Sky && triangle.Surface == "sector-wall"
	}
	if !ceiling || !horizon {
		t.Fatal("sky masks omitted from immutable chunk")
	}
}

func TestDecodeRejectsRecordExpansionBeforeOwnedAllocation(t *testing.T) {
	w, err := newWire(meshHeaderBytes, "KCM1", meshHeaderBytes)
	if err != nil {
		t.Fatal(err)
	}
	encoded := w.raw
	binary.LittleEndian.PutUint32(encoded[8:12], 1)
	binary.LittleEndian.PutUint32(encoded[16:20], MaxVertices+3)
	binary.LittleEndian.PutUint32(encoded[20:24], MaxVertices+3)
	binary.LittleEndian.PutUint32(encoded[24:28], MaxTriangles+1)
	if _, err := DecodeMesh(encoded, nil); err == nil || !strings.Contains(err.Error(), "array record bound") {
		t.Fatal("oversized tiny vertex records reached owned decoder")
	}
}

func TestOfflineTrianglesUseStoredFloat32Geometry(t *testing.T) {
	document := fixture(t)
	// A valid small difference at an ordinary portal produces distinct float64
	// junctions and a lower wall strip whose endpoints coincide in float32.
	document.Sectors[1].Floor = document.Sectors[0].Floor
	document.Sectors[1].Floor.C += 1e-9
	document.Sectors[1].Ceiling = document.Sectors[0].Ceiling
	plan, err := Plan(document, Options{NoLightmap: true, MaxSectors: 1})
	if err != nil || len(plan.Chunks) != 2 {
		t.Fatalf("quantized junctions produced invalid offline triangles: %v", err)
	}
	for _, chunk := range plan.Chunks {
		for i, triangle := range chunk.Mesh.Triangles {
			vertices := chunk.Mesh.Vertices[i*3 : i*3+3]
			a, b, c := unpacked(vertices[0].Position), unpacked(vertices[1].Position), unpacked(vertices[2].Position)
			if dot(cross(sub(b, a), sub(c, a)), unpacked(triangle.Normal)) <= 0 {
				t.Fatal("stored float32 triangle is degenerate or reversed")
			}
		}
		if _, err := EncodeMesh(chunk.Mesh, &document); err != nil {
			t.Fatalf("complete receiver coverage lost at quantized junction: %v", err)
		}
	}
}
