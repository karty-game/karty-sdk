package worldchunk

import (
	"math"
	"slices"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

func rejectMeshMutation(t *testing.T, document world.Document, source Mesh, mutate func(*Mesh)) {
	t.Helper()
	mesh := source
	mesh.Vertices = slices.Clone(source.Vertices)
	mesh.Triangles = slices.Clone(source.Triangles)
	mutate(&mesh)
	if err := ValidateMesh(&mesh, &document); err == nil {
		t.Fatal("malformed authored geometry or attributes accepted")
	}
	if _, err := EncodeMesh(mesh, &document); err == nil {
		t.Fatal("malformed mesh encoded")
	}
	encoded, err := encodeMeshBinary(mesh)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeMesh(encoded, &document); err == nil {
		t.Fatal("malformed binary mesh decoded")
	}
}

func TestMeshRejectsAreaPreservingGeometryAndFiniteAttributeCorruption(t *testing.T) {
	document := fixture(t)
	plan, err := Plan(document, Options{NoLightmap: true})
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Mesh){
		"off plane": func(mesh *Mesh) {
			for index := range 3 {
				mesh.Vertices[index].Position[2] += .1
			}
		},
		"equal-area duplicate wedge": func(mesh *Mesh) { copy(mesh.Vertices[3:6], mesh.Vertices[:3]) },
		"primary UV":                 func(mesh *Mesh) { mesh.Vertices[0].UV[0][0] += 100 },
		"unused UV slot":             func(mesh *Mesh) { mesh.Vertices[0].UV[2][0] = 1 },
		"wave UV":                    func(mesh *Mesh) { mesh.Vertices[0].WaveUV[0] += .1 },
		"unused lightmap UV":         func(mesh *Mesh) { mesh.Vertices[0].LightmapUV[0] = 1 },
		"projection basis":           func(mesh *Mesh) { mesh.Triangles[0].ProjectionBases[0].Tangent[0] = 123 },
		"primary tangent":            func(mesh *Mesh) { mesh.Triangles[0].Tangent[0] = 123 },
		"unused projection basis":    func(mesh *Mesh) { mesh.Triangles[0].ProjectionBases[2].Tangent[0] = 1 },
		"unused band basis":          func(mesh *Mesh) { mesh.Triangles[0].Band.Basis.Tangent[0] = 1 },
	} {
		t.Run(name, func(t *testing.T) { rejectMeshMutation(t, document, plan.Chunks[0].Mesh, mutate) })
	}
}

func TestMeshRequiresEveryDisconnectedWallStrip(t *testing.T) {
	document := fixture(t)
	document.Sectors[0].Floor, document.Sectors[0].Ceiling = world.Plane{}, world.Plane{C: 4}
	document.Sectors[1].Floor, document.Sectors[1].Ceiling = world.Plane{C: 1}, world.Plane{C: 3}
	document.Contents[0].Position.Z = 2
	plan, err := Plan(document, Options{NoLightmap: true})
	if err != nil {
		t.Fatal(err)
	}
	var lower, upper []int
	mesh := plan.Chunks[0].Mesh
	for index, triangle := range mesh.Triangles {
		if triangle.Surface == "sector-wall" && triangle.Sector == 0 && triangle.SurfaceEdge == 1 {
			if mesh.Vertices[index*3].Position[2] < 2 {
				lower = append(lower, index)
			} else {
				upper = append(upper, index)
			}
		}
	}
	if len(lower) != 4 || len(upper) != 4 {
		t.Fatal("fixture must contain equal-area separate wall strips")
	}
	rejectMeshMutation(t, document, mesh, func(copyMesh *Mesh) {
		for index, triangle := range upper {
			copy(copyMesh.Vertices[triangle*3:triangle*3+3], copyMesh.Vertices[lower[index]*3:lower[index]*3+3])
		}
	})
}

func TestMeshRejectsSecondaryWeightsAndUVDrift(t *testing.T) {
	document := liquidFixture(t)
	document.MaterialLayers = &world.MaterialLayers{Version: 1}
	document.Sectors[0].FloorSecondary = &world.SurfaceSecondary{Material: 7, Strength: .5, UV: document.Sectors[0].FloorUV}
	plan, err := Plan(document, Options{NoLightmap: true})
	if err != nil {
		t.Fatal(err)
	}
	rejectMeshMutation(t, document, plan.Chunks[0].Mesh, func(mesh *Mesh) { mesh.Triangles[0].Secondary.Weights[0] = .5 })
	rejectMeshMutation(t, document, plan.Chunks[0].Mesh, func(mesh *Mesh) { mesh.Vertices[0].SecondaryUV[0][0] += .1 })
}

func TestMeshBindingRejectsIncompleteArraysBeforeIndexing(t *testing.T) {
	document := fixture(t)
	plan, err := Plan(document, Options{NoLightmap: true})
	if err != nil {
		t.Fatal(err)
	}
	mesh := plan.Chunks[0].Mesh
	mesh.Vertices = mesh.Vertices[:len(mesh.Vertices)-1]
	if err := ValidateMeshBinding(&mesh, &plan.Chunks[0].Layout); err == nil {
		t.Fatal("incomplete binding arrays accepted")
	}
}

func TestMeshCoveragePreservesClippedEndpointsAtStoredPrecision(t *testing.T) {
	document := fixture(t)
	document.Sectors[0].Floor = world.Plane{B: .08}
	document.Sectors[1].Floor = world.Plane{B: -.12, C: .2}
	for index := range document.Sectors {
		document.Sectors[index].Ceiling = world.Plane{C: 4}
	}
	surfaces, err := worldlightmap.Surfaces(&document)
	if err != nil {
		t.Fatal(err)
	}
	repeated := false
	for _, surface := range surfaces {
		for _, polygon := range surface.Polygons {
			for index, point := range polygon {
				next := polygon[(index+1)%len(polygon)]
				repeated = repeated || point != next && packed(point) == packed(next)
			}
		}
	}
	if !repeated {
		t.Fatal("fixture must retain a clipping endpoint below stored precision")
	}
	assertMeshOnlyCompilerParity(t, document, Options{MaxSectors: 1})
}

func TestMeshBindingRejectsNonfiniteRNMCoordinates(t *testing.T) {
	document := fixture(t)
	plan, err := Plan(document, Options{Lightmap: worldlightmap.Options{PageSize: 512, TexelsPerUnit: 2}})
	if err != nil {
		t.Fatal(err)
	}
	mesh := plan.Chunks[0].Mesh
	mesh.Vertices[0].LightmapUV[0] = float32(math.NaN())
	if err := ValidateMeshBinding(&mesh, &plan.Chunks[0].Layout); err == nil {
		t.Fatal("nonfinite RNM coordinate accepted")
	}
}

func TestMeshRejectsAuthoredFrameAttributeDrift(t *testing.T) {
	document := fixture(t)
	document.MaterialMapping = &world.MaterialMapping{Version: 1}
	document.MaterialLayers = &world.MaterialLayers{Version: 1}
	for sector := range document.Sectors {
		s := &document.Sectors[sector]
		s.FloorUV = legacyMapping(&document, Triangle{Sector: sector, Surface: "sector-floor"})
		s.CeilingUV = legacyMapping(&document, Triangle{Sector: sector, Surface: "sector-ceiling"})
		for edge := range s.Walls {
			s.Walls[edge].UV = legacyMapping(&document, Triangle{Sector: sector, Surface: "sector-wall", SurfaceEdge: edge})
		}
	}
	profile, err := world.ProfileForWall(&document, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	regions, err := world.CompileWallFrame(document.Sectors[0].Walls[0], profile, world.WallFrameConfig{
		Top: world.FrameHorizontal{Height: 1, Repeat: 1, Piece: world.FramePiece{Material: 8, Coverage: world.FrameCoverageOpaque}},
	})
	if err != nil {
		t.Fatal(err)
	}
	document.Sectors[0].Walls[0].FrameRegions = regions
	document.Sectors[0].Walls[0].FrameCompiled = true
	plan, err := Plan(document, Options{NoLightmap: true})
	if err != nil {
		t.Fatal(err)
	}
	mesh := plan.Chunks[0].Mesh
	frame := slices.IndexFunc(mesh.Triangles, func(triangle Triangle) bool { return triangle.Band.Coverage > 0 })
	if frame < 0 {
		t.Fatal("fixture frame absent")
	}
	for name, mutate := range map[string]func(*Mesh){
		"coverage": func(mesh *Mesh) { mesh.Triangles[frame].Band.Coverage = 1 },
		"repeat":   func(mesh *Mesh) { mesh.Triangles[frame].Band.RepeatU = !mesh.Triangles[frame].Band.RepeatU },
		"basis":    func(mesh *Mesh) { mesh.Triangles[frame].Band.Basis.Tangent[0] = 123 },
		"UV":       func(mesh *Mesh) { mesh.Vertices[frame*3].BandUV[0] += 100 },
	} {
		t.Run(name, func(t *testing.T) { rejectMeshMutation(t, document, mesh, mutate) })
	}
}

func TestMeshLargeCoordinateDoesNotRelaxOtherAxes(t *testing.T) {
	document := fixture(t)
	for sector := range document.Sectors {
		s := &document.Sectors[sector]
		s.Floor, s.Ceiling = world.Plane{}, world.Plane{C: 4}
		for edge := range s.Walls {
			s.Walls[edge].Start.X += 900000
			s.Walls[edge].End.X += 900000
		}
	}
	document.Contents[0].Position.X += 900000
	plan, err := Plan(document, Options{NoLightmap: true})
	if err != nil {
		t.Fatal(err)
	}
	rejectMeshMutation(t, document, plan.Chunks[0].Mesh, func(mesh *Mesh) {
		for index, triangle := range mesh.Triangles {
			if triangle.Surface == "sector-floor" && triangle.Sector == 0 {
				for vertex := range 3 {
					mesh.Vertices[index*3+vertex].Position[2] += .1
				}
			}
		}
	})
}
