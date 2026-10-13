package worldchunk

import (
	"slices"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

func skyPortalFixture(t testing.TB, indoor, framed bool) world.Document {
	t.Helper()
	document := fixture(t)
	document.Contents = nil
	document.Environment = &world.Environment{Version: 1, Preset: world.EnvironmentDaylight}
	document.Sectors[0].Floor, document.Sectors[0].Ceiling = world.Plane{}, world.Plane{C: 8}
	document.Sectors[1].Floor, document.Sectors[1].Ceiling = world.Plane{C: 2}, world.Plane{B: .125, C: 4}
	document.Sectors[0].Sky, document.Sectors[1].Sky = true, !indoor
	if framed {
		document.MaterialMapping = &world.MaterialMapping{Version: 1}
		document.MaterialLayers = &world.MaterialLayers{Version: 1}
		for index := range document.Sectors {
			sector := &document.Sectors[index]
			sector.FloorUV = &world.SurfaceUV{Projections: []world.UVProjection{{U: world.UVPlane{X: 1}, V: world.UVPlane{Y: 1}}},
				Weights: []float64{1}}
			sector.CeilingUV = sector.FloorUV
			for edge := range sector.Walls {
				sector.Walls[edge].UV = legacyMapping(&document, Triangle{Sector: index, Surface: "sector-wall", SurfaceEdge: edge})
			}
		}
		profile, err := world.ProfileForWall(&document, 0, 1)
		if err != nil {
			t.Fatal(err)
		}
		regions, err := world.CompileWallFrame(document.Sectors[0].Walls[1], profile,
			world.WallFrameConfig{Top: world.FrameHorizontal{Height: .25, Repeat: 1,
				Piece: world.FramePiece{Material: 8, Coverage: world.FrameCoverageOpaque}}})
		if err != nil {
			t.Fatal(err)
		}
		document.Sectors[0].Walls[1].FrameRegions = regions
		document.Sectors[0].Walls[1].FrameCompiled = true
	}
	return document
}

func TestChunkSkyPortalUpperMasksRetainDepthWithoutReceiversOrCasters(t *testing.T) {
	for _, framed := range []bool{false, true} {
		for _, indoor := range []bool{false, true} {
			document := skyPortalFixture(t, indoor, framed)
			plan, err := Plan(document, Options{Lightmap: worldlightmap.Options{PageSize: 512, TexelsPerUnit: 2}})
			if err != nil {
				t.Fatal(err)
			}
			mesh, layout := plan.Chunks[0].Mesh, plan.Chunks[0].Layout
			upper, lower, firstUpper, firstLower := 0, 0, -1, -1
			for index, triangle := range mesh.Triangles {
				if triangle.Sector != 0 || triangle.Surface != "sector-wall" || triangle.SurfaceEdge != 1 {
					continue
				}
				if mesh.Vertices[index*3].Position[2] > 3 {
					upper++
					firstUpper = index
					if triangle.Sky == indoor || !triangle.Outdoor || !indoor && triangle.Chart != -1 {
						t.Fatal("outdoor upper mask or indoor lintel metadata is incorrect")
					}
				} else {
					lower++
					firstLower = index
					if triangle.Sky || triangle.Chart < 0 {
						t.Fatal("physical lower span lost its opaque receiver")
					}
				}
			}
			if upper == 0 || lower == 0 {
				t.Fatal("finite upper mask or lower wall geometry was omitted")
			}
			encoded, err := EncodeMesh(mesh, &document)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := DecodeMesh(encoded, &document)
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateMeshBinding(&decoded, &layout); err != nil {
				t.Fatal(err)
			}
			proxy, err := CompileShadowProxy(document)
			if err != nil {
				t.Fatal(err)
			}
			for _, caster := range proxy.Triangles {
				if !indoor && caster.Sector == 0 && caster.Surface == "sector-wall" && caster.SurfaceEdge == 1 &&
					caster.Positions[0][2] > 3 {
					t.Fatal("sky mask entered the opaque shadow proxy")
				}
			}
			if !indoor {
				rejectMeshMutation(t, document, mesh, func(changed *Mesh) { changed.Triangles[firstUpper].Sky = false })
				rejectMeshMutation(t, document, mesh, func(changed *Mesh) { changed.Triangles[firstLower].Sky = true })
				bad := mesh
				bad.Triangles = slices.Clone(mesh.Triangles)
				bad.Triangles[firstUpper].Chart = 0
				if ValidateMeshBinding(&bad, &layout) == nil {
					t.Fatal("sky mask accepted an opaque RNM chart")
				}
			}
			assertMeshOnlyCompilerParity(t, document, Options{})
		}
	}
}

func TestChunkMeshRejectsTruncatedSkyUpperMask(t *testing.T) {
	document := skyPortalFixture(t, false, false)
	plan, err := Plan(document, Options{NoLightmap: true})
	if err != nil {
		t.Fatal(err)
	}
	mesh := plan.Chunks[0].Mesh
	last := len(mesh.Triangles) - 1
	for index, triangle := range mesh.Triangles {
		if triangle.Sector == 0 && triangle.Surface == "sector-wall" && triangle.SurfaceEdge == 1 && triangle.Sky {
			mesh.Triangles[index], mesh.Triangles[last] = mesh.Triangles[last], mesh.Triangles[index]
			copy(mesh.Vertices[index*3:index*3+3], mesh.Vertices[last*3:last*3+3])
			break
		}
	}
	mesh.Triangles = mesh.Triangles[:last]
	mesh.Vertices, mesh.Indices = mesh.Vertices[:last*3], mesh.Indices[:last*3]
	if _, err := EncodeMesh(mesh, &document); err == nil {
		t.Fatal("incomplete sky-mask geometry accepted")
	}
}
