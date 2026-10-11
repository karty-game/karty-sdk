package worldchunk

import (
	"bytes"
	"encoding/json"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

func liquidFixture(t testing.TB) world.Document {
	t.Helper()
	document := fixture(t)
	document.MaterialMapping = &world.MaterialMapping{Version: 1}
	for sector := range document.Sectors {
		item := &document.Sectors[sector]
		item.Floor, item.Ceiling = world.Plane{}, world.Plane{C: 4}
		item.FloorUV = &world.SurfaceUV{
			Projections: []world.UVProjection{{U: world.UVPlane{X: .25, Offset: .13}, V: world.UVPlane{Y: .25, Offset: -.17}}},
			Weights:     []float64{1},
		}
		item.CeilingUV = legacyMapping(&document, Triangle{Sector: sector, Surface: "sector-ceiling"})
		for edge := range item.Walls {
			item.Walls[edge].UV = legacyMapping(&document, Triangle{Sector: sector, Surface: "sector-wall", SurfaceEdge: edge})
		}
	}
	document.Animations = &world.Animations{
		Version:   1,
		Presets:   []world.AnimationPreset{{Name: "acid", Kind: "liquid", Frequency: 1, SurfaceAmplitude: .045}},
		Materials: []world.MaterialAnimation{{Material: 1, Preset: "acid"}},
	}
	return document
}

func liquidSurfaceVertices(mesh *Mesh, sector int) map[[3]float32][2]float32 {
	vertices := make(map[[3]float32][2]float32)
	for index, triangle := range mesh.Triangles {
		if triangle.Sector != sector || triangle.Surface != "sector-floor" {
			continue
		}
		for _, vertex := range mesh.Vertices[index*3 : index*3+3] {
			vertices[vertex.Position] = vertex.WaveUV
		}
	}
	return vertices
}

func TestChunkLiquidOfflineSeamsAndChunkIndependentSampling(t *testing.T) {
	document := liquidFixture(t)
	whole, err := Plan(document, Options{NoLightmap: true})
	if err != nil || len(whole.Chunks) != 1 {
		t.Fatalf("whole liquid patch: %v", err)
	}
	split, err := Plan(document, Options{NoLightmap: true, MaxSectors: 1})
	if err != nil || len(split.Chunks) != 2 {
		t.Fatalf("split liquid patch: %v", err)
	}
	floorCount := 0
	for _, triangle := range whole.Chunks[0].Mesh.Triangles {
		if triangle.Surface == "sector-floor" {
			floorCount++
		}
	}
	if floorCount <= 8 || floorCount > liquidPatchTriangleTarget {
		t.Fatalf("subtle connected patch uses %d triangles", floorCount)
	}
	seams := [2]map[[3]float32][2]float32{{}, {}}
	for sector, chunk := range split.Chunks {
		want := liquidSurfaceVertices(&whole.Chunks[0].Mesh, sector)
		got := liquidSurfaceVertices(&chunk.Mesh, sector)
		if len(want) != len(got) {
			t.Fatal("chunk budget changed full physical patch sampling")
		}
		for position, wave := range want {
			if actual, found := got[position]; !found || actual != wave {
				t.Fatalf("partition changed liquid rest position or phase: %v", position)
			}
			if position[0] == 4 {
				seams[sector][position] = wave
			}
		}
		if _, err := EncodeMesh(chunk.Mesh, &document); err != nil {
			t.Fatalf("subdivision changed semantic area coverage: %v", err)
		}
	}
	if len(seams[0]) <= 2 || len(seams[0]) != len(seams[1]) {
		t.Fatalf("shared edge subdivisions differ: %d/%d", len(seams[0]), len(seams[1]))
	}
	for position, wave := range seams[0] {
		if actual, found := seams[1][position]; !found || actual != wave {
			t.Fatalf("shared liquid edge has a rest-position or wave-phase crack at %v", position)
		}
	}
	again, err := Plan(document, Options{NoLightmap: true, MaxSectors: 1})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(split)
	after, _ := json.Marshal(again)
	if !bytes.Equal(before, after) {
		t.Fatal("offline liquid arrays are nondeterministic")
	}
}

func TestChunkLiquidPreservesChartsAttributesAndRestBounds(t *testing.T) {
	document := liquidFixture(t)
	document.MaterialLayers = &world.MaterialLayers{Version: 1}
	document.Sectors[0].FloorSecondary = &world.SurfaceSecondary{Material: 7, Strength: .5, UV: document.Sectors[0].FloorUV}
	document.Lighting = &world.Lighting{Version: 1, Lights: []world.PointLight{
		{ID: "light", Position: world.Vec3{X: 2, Y: 2, Z: 2}, Color: world.Vec3{X: 1, Y: 1, Z: 1}, Radius: 10},
	}}
	plan, err := Plan(document, Options{MaxSectors: 1, Lightmap: worldlightmap.Options{
		PageSize: 512, TexelsPerUnit: 4, Padding: 4, Lights: []string{"light"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, chunk := range plan.Chunks {
		if chunk.MeshBytes != uint64(len(chunk.Mesh.Triangles))*484 {
			t.Fatal("subdivided decoded byte accounting changed")
		}
		if err := ValidateMeshBinding(&chunk.Mesh, &chunk.Layout); err != nil {
			t.Fatalf("affine bake coordinates changed during subdivision: %v", err)
		}
		for index, triangle := range chunk.Mesh.Triangles {
			if triangle.Surface != "sector-floor" {
				continue
			}
			if triangle.Tangent != [3]float32{1, 0, 0} || triangle.Bitangent != [3]float32{0, 1, 0} ||
				triangle.Sector == 0 && triangle.Secondary.Strength != .5 {
				t.Fatal("liquid subdivision changed authored differential bases or secondary identity")
			}
			for _, vertex := range chunk.Mesh.Vertices[index*3 : index*3+3] {
				position := unpacked(vertex.Position)
				if position.Z != 0 || math.Abs(float64(vertex.WaveUV[0])-(position.X*.25+.13)) > 1e-6 ||
					math.Abs(float64(vertex.WaveUV[1])-(position.Y*.25-.17)) > 1e-6 ||
					math.Abs(float64(vertex.UV[0][0])-(position.X*.25+.13-math.Floor(float64(triangle.Sector)+.13))) > 1e-6 ||
					math.Abs(float64(vertex.UV[0][1])-(position.Y*.25+.83)) > 1e-6 {
					t.Fatal("liquid UV or rest position lost its affine authored semantics")
				}
				if triangle.Sector == 0 && vertex.SecondaryUV[0] != vertex.UV[0] {
					t.Fatal("secondary affine UVs changed during subdivision")
				}
			}
		}
	}
}

func TestChunkLiquidBudgetsIncludeFinalOfflineGeometry(t *testing.T) {
	document := liquidFixture(t)
	split, err := Plan(document, Options{NoLightmap: true, MaxSectors: 1})
	if err != nil {
		t.Fatal(err)
	}
	budget := max(split.Chunks[0].MeshBytes, split.Chunks[1].MeshBytes)
	plan, err := Plan(document, Options{NoLightmap: true, MaxMeshBytes: budget})
	if err != nil || len(plan.Chunks) != 2 {
		t.Fatalf("final liquid mesh bypassed chunk budget: %v", err)
	}
	if _, err := Plan(
		document,
		Options{NoLightmap: true, MaxMeshBytes: min(split.Chunks[0].MeshBytes, split.Chunks[1].MeshBytes) - 1},
	); err == nil ||
		!strings.Contains(err.Error(), "cannot fit one chunk") {
		t.Fatal("oversized single-sector liquid mesh did not fail")
	}
	for _, chunk := range plan.Chunks {
		if chunk.MeshBytes > budget {
			t.Fatal("mesh scalar budget excludes offline liquid vertices")
		}
	}
}

func TestChunkLiquidDigestBindsSubdivisionInputs(t *testing.T) {
	document := liquidFixture(t)
	plan, err := Plan(document, Options{NoLightmap: true, MaxSectors: 1})
	if err != nil {
		t.Fatal(err)
	}
	mesh := plan.Chunks[0].Mesh
	for _, field := range []string{"frequency", "amplitude", "material"} {
		copyDocument := document
		copyAnimations := *document.Animations
		copyAnimations.Presets = slices.Clone(document.Animations.Presets)
		copyAnimations.Materials = slices.Clone(document.Animations.Materials)
		copyDocument.Animations = &copyAnimations
		switch field {
		case "frequency":
			copyAnimations.Presets[0].Frequency *= 2
		case "amplitude":
			copyAnimations.Presets[0].SurfaceAmplitude *= 2
		case "material":
			copyAnimations.Materials = nil
		}
		if err := ValidateMesh(&mesh, &copyDocument); err == nil {
			t.Fatalf("stale liquid %s accepted", field)
		}
	}
	document.Animations.Presets[0].Speed = .7
	document.Animations.Presets[0].Opacity = new(.5)
	if err := ValidateMesh(&mesh, &document); err != nil {
		t.Fatalf("visual timing/opacity invalidated rest geometry: %v", err)
	}
}

func TestChunkLiquidDisconnectedPoolsKeepIndependentFactors(t *testing.T) {
	source := Mesh{}
	for pool, size := range []float32{1, 4} {
		vertices := [3]Vertex{{Position: [3]float32{float32(pool) * 10, 0, 0}},
			{Position: [3]float32{float32(pool)*10 + size, 0, 0}}, {Position: [3]float32{float32(pool) * 10, size, 0}}}
		for corner := range vertices {
			vertices[corner].WaveUV = [2]float32{vertices[corner].Position[0] * .25, vertices[corner].Position[1] * .25}
		}
		appendLiquidTriangle(&source, Triangle{Normal: [3]float32{0, 0, 1}, Material: 1}, vertices)
	}
	liquids := map[uint32]liquidMaterial{1: {Material: 1, Frequency: 1, Amplitude: .04}}
	factors := liquidFactors(&source, liquids)
	if factors[0] >= factors[1] || factors[0] <= 1 {
		t.Fatal("disconnected larger pool changed small-pool subdivision")
	}
	for _, material := range []liquidMaterial{{Material: 1, Frequency: 2, Amplitude: .04}, {Material: 1, Frequency: 1, Amplitude: .16}} {
		liquids[1] = material
		refined := liquidFactors(&source, liquids)
		if refined[1] <= factors[1] || refined[1] > liquidMaxSubdivisions {
			t.Fatal("wave frequency/amplitude did not refine the offline mesh within its cap")
		}
	}
}

func TestChunkLiquidFinalSamplesBoundWaveInterpolation(t *testing.T) {
	source := Mesh{Sectors: []int{0}}
	vertices := [3]Vertex{{}, {Position: [3]float32{4, 0, 0}}, {Position: [3]float32{0, 4, 0}}}
	for corner := range vertices {
		vertices[corner].WaveUV = [2]float32{vertices[corner].Position[0] * .25, vertices[corner].Position[1] * .25}
	}
	appendLiquidTriangle(&source, Triangle{Sector: 0, Solid: -1, Normal: [3]float32{0, 0, 1}, Material: 1}, vertices)
	mesh, err := tessellateLiquids(source, map[uint32]liquidMaterial{1: {Material: 1, Frequency: 1, Amplitude: .04}})
	if err != nil {
		t.Fatal(err)
	}
	for _, phase := range []float64{0, .73, 2.1} {
		wave := func(uv [2]float64) float64 {
			return .04 * (math.Sin((uv[0]+.37*uv[1])*2*math.Pi+phase) + .5*math.Sin((-.53*uv[0]+uv[1])*2*math.Pi-phase*.7)) / 1.5
		}
		for index := range mesh.Triangles {
			for _, weights := range [][3]float64{{1.0 / 3, 1.0 / 3, 1.0 / 3}, {.5, .5, 0}, {0, .5, .5}, {.5, 0, .5}} {
				uv, interpolated := [2]float64{}, 0.0
				for corner, weight := range weights {
					point := mesh.Vertices[index*3+corner].WaveUV
					uv[0] += float64(point[0]) * weight
					uv[1] += float64(point[1]) * weight
					interpolated += wave([2]float64{float64(point[0]), float64(point[1])}) * weight
				}
				if math.Abs(interpolated-wave(uv)) > liquidInterpolationError+1e-7 {
					t.Fatal("affordable offline patch exceeded two-wave interpolation target")
				}
			}
		}
	}
}

func TestChunkLiquidExpansionChecksTriangleLimitBeforeAllocation(t *testing.T) {
	source := Mesh{Sectors: []int{0}}
	for pool := range 65 {
		position := float32(pool) * 200
		vertices := [3]Vertex{{Position: [3]float32{position, 0, 0}},
			{Position: [3]float32{position + 100, 0, 0}}, {Position: [3]float32{position, 100, 0}}}
		for corner := range vertices {
			vertices[corner].WaveUV = [2]float32{vertices[corner].Position[0], vertices[corner].Position[1]}
		}
		appendLiquidTriangle(&source, Triangle{Sector: 0, Solid: -1, Normal: [3]float32{0, 0, 1}, Material: 1}, vertices)
	}
	liquids := map[uint32]liquidMaterial{1: {Material: 1, Frequency: 100, Amplitude: .25}}
	if _, err := tessellateLiquids(source, liquids); err == nil || !strings.Contains(err.Error(), "triangle budget") {
		t.Fatal("expanded offline liquid mesh crossed the hard triangle bound")
	}
	source.Triangles = source.Triangles[:64]
	source.Vertices = source.Vertices[:64*3]
	source.Indices = source.Indices[:64*3]
	mesh, err := tessellateLiquids(source, liquids)
	if err != nil || len(mesh.Triangles) != MaxTriangles || len(mesh.Vertices) != MaxVertices {
		t.Fatalf("exact final triangle bound rejected: %v", err)
	}
}

func TestChunkLiquidCompletePayloadValidation(t *testing.T) {
	document := liquidFixture(t)
	plan, err := Plan(document, Options{NoLightmap: true, MaxSectors: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []string{"wave", "rest", "coverage"} {
		mesh := plan.Chunks[0].Mesh
		mesh.Vertices = slices.Clone(mesh.Vertices)
		mesh.Indices = slices.Clone(mesh.Indices)
		mesh.Triangles = slices.Clone(mesh.Triangles)
		switch mutation {
		case "wave":
			mesh.Vertices[0].WaveUV[0] += .1
		case "rest":
			mesh.Vertices[0].Position[2] += .1
		case "coverage":
			mesh.Vertices = mesh.Vertices[3:]
			mesh.Triangles = mesh.Triangles[1:]
			mesh.Indices = mesh.Indices[:len(mesh.Indices)-3]
		}
		if err := ValidateMesh(&mesh, &document); err == nil {
			t.Fatalf("corrupt final liquid %s accepted", mutation)
		}
	}
}

func TestChunkLiquidSolidTopsRefineWithoutChangingOtherFaces(t *testing.T) {
	document := liquidFixture(t)
	document.StaticSolids = &world.StaticSolids{Version: 1, Items: []world.Solid{{
		ID: "pool", Footprint: []world.Vec2{{X: 1, Y: 1}, {X: 3, Y: 1}, {X: 3, Y: 3}, {X: 1, Y: 3}},
		Bottom: world.Plane{C: .2}, Top: world.Plane{C: .5}, SideMaterial: 7, TopMaterial: 7, BottomMaterial: 7,
	}}}
	document.Animations.Materials = append(document.Animations.Materials, world.MaterialAnimation{Material: 7, Preset: "acid"})
	plan, err := Plan(document, Options{NoLightmap: true, MaxSectors: 1})
	if err != nil {
		t.Fatal(err)
	}
	without := document
	without.Animations = nil
	baseline, err := Plan(without, Options{NoLightmap: true, MaxSectors: 1})
	if err != nil {
		t.Fatal(err)
	}
	counts := [2]map[string]int{{}, {}}
	for variant, mesh := range []*Mesh{&baseline.Chunks[0].Mesh, &plan.Chunks[0].Mesh} {
		for index, triangle := range mesh.Triangles {
			if triangle.Solid < 0 {
				continue
			}
			counts[variant][triangle.Surface]++
			if triangle.Surface == "solid-top" {
				for _, vertex := range mesh.Vertices[index*3 : index*3+3] {
					if vertex.Position[2] != .5 {
						t.Fatal("offline solid liquid displaced the rest receiver")
					}
				}
			}
		}
	}
	if counts[1]["solid-top"] <= counts[0]["solid-top"] || counts[1]["solid-top"] > liquidPatchTriangleTarget ||
		counts[1]["solid-side"] != counts[0]["solid-side"] || counts[1]["solid-bottom"] != counts[0]["solid-bottom"] {
		t.Fatal("liquid material must refine upward tops while retaining sides and bottoms")
	}
	if _, err := EncodeMesh(plan.Chunks[0].Mesh, &document); err != nil {
		t.Fatalf("liquid solid semantic coverage failed: %v", err)
	}
}
