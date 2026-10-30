package worldchunk

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

func TestSpatialSelectedMeshAndReadyPreserveClippedCoverage(t *testing.T) {
	document := liquidFixture(t)
	binding := worldlightmap.Binding{Kind: "sector-floor", Index: 0, Edge: -1, Chart: -1}
	var edgeWave map[[3]float32][2]float32
	for i, tile := range [][4]float64{{0, 0, 2, 4}, {2, 0, 4, 4}} {
		layout, err := worldlightmap.CompileSelectedReceivers(
			document,
			worldlightmap.Options{TexelsPerUnit: 4, PageSize: 32},
			[]int{0},
			nil,
			worldlightmap.SurfaceSelection{Bindings: []worldlightmap.Binding{binding}, Clip: &tile},
		)
		if err != nil {
			t.Fatal(err)
		}
		mesh, err := CompileMesh(document, []int{0}, nil, layout)
		if err != nil {
			t.Fatal(err)
		}
		data, err := EncodeMesh(mesh, &document)
		if err != nil {
			t.Fatal(err)
		}
		if string(data[:4]) != "KCM3" {
			t.Fatal("selected mesh missing gate")
		}
		decoded, err := DecodeMesh(data, &document)
		if err != nil {
			t.Fatal(err)
		}
		again, err := EncodeMesh(decoded, &document)
		if err != nil || !bytes.Equal(data, again) {
			t.Fatal("noncanonical mesh", err)
		}
		if _, err := CompileReady(&mesh, data, &document, &layout, ReadyOptions{}); err != nil {
			t.Fatal(err)
		}
		wave := liquidSurfaceVertices(&mesh, 0)
		edge := make(map[[3]float32][2]float32)
		for p, uv := range wave {
			if p[0] == 2 {
				edge[p] = uv
			}
			if p[0] < float32(tile[0]) || p[0] > float32(tile[2]) {
				t.Fatal("mesh escaped tile")
			}
		}
		if i == 0 {
			edgeWave = edge
		} else {
			if len(edge) != len(edgeWave) {
				t.Fatalf("liquid seam subdivisions differ: %d %d", len(edge), len(edgeWave))
			}
			for p, uv := range edge {
				if want, ok := edgeWave[p]; !ok || want != uv {
					t.Fatal("liquid wave seam", p, uv, want)
				}
			}
		}
		mesh.Schema = MeshSchema
		if _, err := EncodeMesh(mesh, &document); err == nil {
			t.Fatal("old mesh schema accepted delegated receiver")
		}
	}
}

func TestSpatialCatalogCoverageAndOldBounds(t *testing.T) {
	document, catalog := binaryCatalogFixture(t)
	catalog.Schema = SpatialSchema
	surfaces, err := worldlightmap.Surfaces(&document)
	if err != nil {
		t.Fatal(err)
	}
	delegated := worldlightmap.Binding{Kind: "sector-floor", Index: 0, Edge: -1, Chart: -1}
	for i := range catalog.Chunks {
		c := &catalog.Chunks[i]
		c.LightmapBytes = 512 * 512 * 12
		c.LightingBytes = 4096
		c.CollisionBytes = c.MeshBytes
	}
	for _, surface := range surfaces {
		binding := surface.Binding
		binding.Chart = -1
		owner := catalog.SectorChunks[binding.Index]
		kind := "primary"
		if binding == delegated {
			kind = "spatial"
		}
		catalog.Surfaces = append(catalog.Surfaces, SurfaceOwnership{Binding: binding, Kind: kind, Owner: owner})
	}
	for i, tile := range [][4]float64{{0, 0, 2, 4}, {2, 0, 4, 4}} {
		catalog.Spatial = append(
			catalog.Spatial,
			SpatialChunk{
				Chunk: Chunk{
					ID:            i,
					Sectors:       []int{0},
					Size:          256,
					ContentSHA256: strings.Repeat("b", 64),
					MeshBytes:     484 * 4,
					LightmapBytes: 32 * 32 * 12,
					LightingBytes: 4096,
					Bounds:        [6]float64{tile[0], tile[1], 0, tile[2], tile[3], 1},
				},
				Parent:   0,
				Tile:     tile,
				Bindings: []worldlightmap.Binding{delegated},
			},
		)
	}
	data, err := EncodeCatalog(catalog, &document)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeCatalog(data, &document)
	if err != nil {
		t.Fatal(err)
	}
	again, err := EncodeCatalog(decoded, &document)
	if err != nil || !bytes.Equal(data, again) {
		t.Fatal("noncanonical spatial catalog", err)
	}
	if decoded.StreamCount() != len(catalog.Chunks)+2 {
		t.Fatal("stream count")
	}
	last, ok := decoded.StreamChunk(decoded.StreamCount() - 1)
	if !ok || last.ID != decoded.StreamCount()-1 {
		t.Fatal("global stream id")
	}
	for name, change := range map[string]func(*Catalog){
		"missing source":    func(c *Catalog) { c.Surfaces = c.Surfaces[:len(c.Surfaces)-1] },
		"gap":               func(c *Catalog) { c.Spatial = slices.Clone(c.Spatial); c.Spatial[1].Tile[0] = 2.5 },
		"overlap":           func(c *Catalog) { c.Spatial = slices.Clone(c.Spatial); c.Spatial[1].Tile[0] = 1.5 },
		"missing collision": func(c *Catalog) { c.Chunks = slices.Clone(c.Chunks); c.Chunks[0].CollisionBytes = 0 },
		"old schema":        func(c *Catalog) { c.Schema = FixedSchema },
		"missing tiles":     func(c *Catalog) { c.Spatial = c.Spatial[:1] },
	} {
		t.Run(name, func(t *testing.T) {
			changed := catalog
			change(&changed)
			if _, err := EncodeCatalog(changed, &document); err == nil {
				t.Fatal("invalid spatial coverage accepted")
			}
		})
	}
	for _, broken := range [][]byte{data[:len(data)-1], append(bytes.Clone(data), 0)} {
		if _, err := DecodeCatalog(broken, &document); err == nil {
			t.Fatal("malformed wrapper accepted")
		}
	}
}

func TestSpatialLiquidDetailCapIsExplicitAndBoundToSelection(t *testing.T) {
	document := liquidFixture(t)
	binding := worldlightmap.Binding{Kind: "sector-floor", Index: 0, Edge: -1, Chart: -1}
	tile := [4]float64{0, 0, 4, 4}
	counts := [2]int{}
	for i, subdivisionCap := range []int{0, 1} {
		selection := worldlightmap.SurfaceSelection{
			Bindings:              []worldlightmap.Binding{binding},
			Clip:                  &tile,
			LiquidMaxSubdivisions: subdivisionCap,
		}
		layout, err := worldlightmap.CompileSelectedReceivers(
			document,
			worldlightmap.Options{TexelsPerUnit: 2, PageSize: 32},
			[]int{0},
			nil,
			selection,
		)
		if err != nil {
			t.Fatal(err)
		}
		mesh, err := CompileMesh(document, []int{0}, nil, layout)
		if err != nil {
			t.Fatal(err)
		}
		if err := ValidateMesh(&mesh, &document); err != nil {
			t.Fatal(err)
		}
		data, err := worldlightmap.Encode(layout, &document)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := worldlightmap.Decode(data, &document)
		if err != nil || decoded.Selection.LiquidMaxSubdivisions != subdivisionCap {
			t.Fatal("lost geometric detail policy", err)
		}
		counts[i] = len(mesh.Triangles)
	}
	if counts[0] <= counts[1] || counts[1] != 4 {
		t.Fatalf("explicit far detail subdivisionCap did not reduce geometry: %v", counts)
	}
}

func TestSpatialLiquidCommonGridKeepsCoarseFineAnimatedBoundaryConforming(t *testing.T) {
	document := liquidFixture(t)
	// Enlarge the existing reciprocal sector pair; only sector0's floor is
	// selected, so two independently compiled leaves share x=32 inside one face.
	for i := range document.Sectors {
		for j := range document.Sectors[i].Walls {
			wall := &document.Sectors[i].Walls[j]
			wall.Start.X *= 32
			wall.Start.Y *= 32
			wall.End.X *= 32
			wall.End.Y *= 32
		}
	}
	document.Contents[0].Position.X *= 32
	document.Contents[0].Position.Y *= 32
	binding := worldlightmap.Binding{Kind: "sector-floor", Index: 0, Edge: -1, Chart: -1}
	edges := [2]map[[3]float32][2]float32{{}, {}}
	for i, tile := range [][4]float64{{0, 0, 32, 32}, {32, 0, 128, 128}} {
		subdivisionCap := 32
		if i == 1 {
			subdivisionCap = 1
		}
		selection := worldlightmap.SurfaceSelection{
			Bindings:              []worldlightmap.Binding{binding},
			Clip:                  &tile,
			LiquidEdgeStep:        2,
			LiquidMaxSubdivisions: subdivisionCap,
		}
		layout, err := worldlightmap.CompileSelectedReceivers(
			document,
			worldlightmap.Options{TexelsPerUnit: .125, PageSize: 64},
			[]int{0},
			nil,
			selection,
		)
		if err != nil {
			t.Fatal(err)
		}
		mesh, err := CompileMesh(document, []int{0}, nil, layout)
		if err != nil {
			t.Fatal(err)
		}
		data, err := EncodeMesh(mesh, &document)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := CompileReady(&mesh, data, &document, &layout, ReadyOptions{}); err != nil {
			t.Fatal(err)
		}
		for p, uv := range liquidSurfaceVertices(&mesh, 0) {
			if p[0] == 32 && p[1] >= 0 && p[1] <= 32 {
				edges[i][p] = uv
			}
		}
	}
	if len(edges[0]) != 17 || len(edges[1]) != 17 {
		t.Fatalf("common boundary must have every2m anchor: %d %d", len(edges[0]), len(edges[1]))
	}
	for p, uv := range edges[0] {
		if other, ok := edges[1][p]; !ok || other != uv {
			t.Fatal("coarse/fine world/wave boundary mismatch", p, uv, other)
		}
	}
}
