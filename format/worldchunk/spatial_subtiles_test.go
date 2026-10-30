package worldchunk

import (
	"bytes"
	"testing"

	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

type spatialTriangleIdentity struct {
	Vertices [3]Vertex
	Triangle Triangle
}

// Atlas coordinates can move when loading units share a page. Every other
// vertex/triangle attribute, including wave phase and tessellation, must match.
func spatialGeometryIdentity(mesh Mesh) map[spatialTriangleIdentity]int {
	result := make(map[spatialTriangleIdentity]int)
	for index, triangle := range mesh.Triangles {
		triangle.Chart = -1
		identity := spatialTriangleIdentity{Triangle: triangle}
		for corner := range 3 {
			vertex := mesh.Vertices[mesh.Indices[index*3+corner]]
			vertex.LightmapUV = [2]float32{}
			identity.Vertices[corner] = vertex
		}
		result[identity]++
	}
	return result
}

func TestSpatialLoadingUnitPreservesOriginalGeometryCells(t *testing.T) {
	t.Parallel()
	document := liquidFixture(t)
	binding := worldlightmap.Binding{Kind: "sector-floor", Index: 0, Edge: -1, Chart: -1}
	whole := [4]float64{0, 0, 4, 4}
	cells := []worldlightmap.LiquidTile{
		{Clip: [4]float64{0, 0, 2, 4}},
		{Clip: [4]float64{2, 0, 4, 4}, LiquidMaxSubdivisions: 1},
	}
	compile := func(selection worldlightmap.SurfaceSelection) (worldlightmap.Layout, Mesh) {
		t.Helper()
		layout, err := worldlightmap.CompileSelectedReceivers(document,
			worldlightmap.Options{TexelsPerUnit: 4, PageSize: 32}, []int{0}, nil, selection)
		if err != nil {
			t.Fatal(err)
		}
		mesh, err := CompileMesh(document, []int{0}, nil, layout)
		if err != nil {
			t.Fatal(err)
		}
		return layout, mesh
	}
	selection := worldlightmap.SurfaceSelection{Bindings: []worldlightmap.Binding{binding}, Clip: &whole,
		LiquidEdgeStep: 1, LiquidTiles: cells}
	layout, grouped := compile(selection)
	want := make(map[spatialTriangleIdentity]int)
	for _, cell := range cells {
		original := selection
		original.LiquidTiles, original.Clip, original.LiquidMaxSubdivisions = nil, &cell.Clip, cell.LiquidMaxSubdivisions
		_, mesh := compile(original)
		for identity, count := range spatialGeometryIdentity(mesh) {
			want[identity] += count
		}
	}
	got := spatialGeometryIdentity(grouped)
	if len(want) != len(got) {
		t.Fatalf("grouping changed geometry: identities %d/%d", len(got), len(want))
	}
	for identity, count := range want {
		if got[identity] != count {
			t.Fatal("grouping changed a triangle, material coordinate or wave phase", identity)
		}
	}
	encoded, err := EncodeMesh(grouped, &document)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeMesh(encoded, &document)
	if err != nil {
		t.Fatal(err)
	}
	again, err := EncodeMesh(decoded, &document)
	if err != nil || !bytes.Equal(encoded, again) {
		t.Fatal("grouped geometry wire did not round-trip", err)
	}
	if err := ValidateMeshBinding(&decoded, &layout); err != nil {
		t.Fatal(err)
	}
	if _, err := CompileReady(&decoded, encoded, &document, &layout, ReadyOptions{}); err != nil {
		t.Fatal("grouped geometry lost ready-stream coverage", err)
	}
	// A larger rectangle alone must fail this oracle by changing ring geometry.
	selection.LiquidTiles = nil
	_, unpartitioned := compile(selection)
	if len(spatialGeometryIdentity(unpartitioned)) == len(got) {
		t.Fatal("geometry oracle cannot distinguish a coarsened loading unit")
	}
}
