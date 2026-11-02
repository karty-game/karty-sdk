package worldchunk

import (
	"bytes"
	"testing"

	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

func TestCompileSelectedMeshHasNoAtlasAndPreservesClippedGeometry(t *testing.T) {
	t.Parallel()
	document := liquidFixture(t)
	tile := [4]float64{0, 0, 2, 4}
	selection := worldlightmap.SurfaceSelection{
		Bindings: []worldlightmap.Binding{{Kind: "sector-floor", Index: 0, Edge: -1, Chart: -1}},
		Clip:     &tile,
	}
	mesh, err := CompileSelectedMesh(document, []int{0}, nil, selection)
	if err != nil {
		t.Fatal(err)
	}
	if mesh.Schema != SelectedMeshSchema || mesh.Selection == nil {
		t.Fatal("geometry selection lost")
	}
	for _, triangle := range mesh.Triangles {
		if triangle.Chart != -1 {
			t.Fatal("geometry-only compile invented lightmap chart")
		}
	}
	for _, vertex := range mesh.Vertices {
		if vertex.Position[0] < 0 || vertex.Position[0] > 2 || vertex.LightmapUV != [2]float32{} {
			t.Fatal("clipping or geometry-only UV changed")
		}
	}
	encoded, err := EncodeMesh(mesh, &document)
	if err != nil {
		t.Fatal(err)
	}
	ready, err := CompileReady(&mesh, encoded, &document, nil, ReadyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(ready.Batches) == 0 {
		t.Fatal("no final geometry")
	}
	selection.Bindings[0].Index = 1
	tile[2] = 1
	if mesh.Selection.Bindings[0].Index != 0 || mesh.Selection.Clip[2] != 2 {
		t.Fatal("caller selection aliases compiled geometry")
	}
	decoded, err := DecodeMesh(encoded, &document)
	if err != nil {
		t.Fatal(err)
	}
	again, err := EncodeMesh(decoded, &document)
	if err != nil || !bytes.Equal(encoded, again) {
		t.Fatal("noncanonical selected geometry", err)
	}
}
func TestCompileSelectedMeshRejectsMalformedClipWithoutLighting(t *testing.T) {
	t.Parallel()
	document := liquidFixture(t)
	tile := [4]float64{2, 0, 1, 4}
	if _, err := CompileSelectedMesh(document, []int{0}, nil, worldlightmap.SurfaceSelection{Clip: &tile}); err == nil {
		t.Fatal("invalid geometry-only selection admitted")
	}
}
