package worldgpuchunk

import (
	"encoding/binary"
	"slices"
	"strings"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldchunk"
	"github.com/karty-game/karty-sdk/format/worldmaterial"
)

func compilerFixture(t *testing.T) (worldchunk.Ready, *world.Document, worldmaterial.Layout) {
	t.Helper()
	document := &world.Document{Version: world.Version, Sectors: []world.Sector{{ID: "room"}}}
	layout, err := worldmaterial.NewLayoutV2([]uint32{7})
	if err != nil {
		t.Fatal(err)
	}
	batch := worldchunk.ReadyBatch{
		Surface:        0,
		StaticMaterial: true,
		Winding:        1,
		Indices:        []uint32{0, 1, 2},
		SurfaceIDs:     []uint32{0, 0, 0},
		Sources:        []uint32{0},
		Owners:         []worldchunk.ReadyOwner{{Sector: 0, Solid: -1, Count: 3, Bounds: [6]float64{0, 0, 0, 1, 1, 0}}},
	}
	for _, point := range [][2]float32{{0, 0}, {1, 0}, {0, 1}} {
		var vertex [worldchunk.ReadyStride]float32
		vertex[0], vertex[1], vertex[5] = point[0], point[1], 1
		vertex[6], vertex[7], vertex[18] = point[0], point[1], 1
		batch.Vertices = append(batch.Vertices, vertex[:]...)
		batch.MaterialBasis = append(batch.MaterialBasis, 1, 0, 0, 1, 1, 0, 0, 1)
		batch.WaveUV = append(batch.WaveUV, point[0], point[1])
		batch.LightmapUV = append(batch.LightmapUV, .5, .5)
		batch.LightmapBasis = append(batch.LightmapBasis, 1, 0, 0, 0, 1, 0)
	}
	return worldchunk.Ready{
		Schema:     worldchunk.ReadySchema,
		MeshSHA256: strings.Repeat("0", 64),
		Surfaces:   []worldchunk.ReadySurface{{Primary: 0, Secondary: 0, Band: 0}},
		Batches:    []worldchunk.ReadyBatch{batch},
	}, document, layout
}

func TestCompileReadyProducesLocalFinalGPUStreams(t *testing.T) {
	t.Parallel()
	ready, document, layout := compilerFixture(t)
	data, err := CompileReady(ready, document, layout, CompileOptions{RGBMRange: 8, SectorProbes: []uint32{2}})
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Vertices) != 3*VertexStride || len(data.Addresses) != 3*AddressStride || len(data.Indices) != 3*IndexStride ||
		len(data.Records) != RecordStride ||
		len(data.Draws) != 1 {
		t.Fatal("unexpected final buffer sizes")
	}
	if data.RecordRefs[0] != (RecordRef{MaterialID: 7, OwnerKind: OwnerSector, OwnerID: 0, SurfaceKind: 1}) {
		t.Fatal("owner metadata", data.RecordRefs)
	}
	if binary.LittleEndian.Uint32(data.Records[308:]) != 2 {
		t.Fatal("lost shared reflection slot")
	}
	for vertex := range 3 {
		if binary.LittleEndian.Uint32(data.Addresses[vertex*AddressStride:]) != uint32(vertex) ||
			binary.LittleEndian.Uint32(data.Addresses[vertex*AddressStride+4:]) != 0 ||
			binary.LittleEndian.Uint32(data.Indices[vertex*IndexStride:]) != uint32(vertex) {
			t.Fatal("nonlocal GPU address", vertex)
		}
	}
	before := slices.Clone(data.Vertices)
	ready.Batches[0].Vertices[0] = 99
	if !slices.Equal(before, data.Vertices) {
		t.Fatal("compiler output aliases source")
	}
}

func TestCompileReadyDeduplicatesOnlyCompleteVertexAttributes(t *testing.T) {
	t.Parallel()
	ready, document, layout := compilerFixture(t)
	batch := &ready.Batches[0]
	batch.Indices = append(batch.Indices, 3, 4, 5)
	batch.Vertices = append(batch.Vertices, batch.Vertices...)
	batch.SurfaceIDs = append(batch.SurfaceIDs, batch.SurfaceIDs...)
	batch.WaveUV = append(batch.WaveUV, batch.WaveUV...)
	batch.MaterialBasis = append(batch.MaterialBasis, batch.MaterialBasis...)
	batch.LightmapUV = append(batch.LightmapUV, batch.LightmapUV...)
	batch.LightmapBasis = append(batch.LightmapBasis, batch.LightmapBasis...)
	batch.Sources = append(batch.Sources, 0)
	batch.Owners[0].Count = 6
	data, err := CompileReady(ready, document, layout, CompileOptions{RGBMRange: 1})
	if err != nil || len(data.Vertices) != 3*VertexStride || len(data.Indices) != 6*IndexStride {
		t.Fatal("offline indexing failed", err)
	}
	batch.MaterialBasis = nil
	batch.StaticMaterial = false
	fallback, err := CompileReady(ready, document, layout, CompileOptions{RGBMRange: 1})
	if err != nil || !slices.Equal(data.Vertices, fallback.Vertices) {
		t.Fatal("explicit and computed tangent bases differ", err)
	}
}

func TestCompileReadyRejectsInvalidTrailingGeometryAtomically(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"index", "material", "owner", "projection"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			ready, document, layout := compilerFixture(t)
			switch mode {
			case "index":
				ready.Batches[0].Indices[2] = 100
			case "material":
				ready.Surfaces[0].Primary = 5
			case "owner":
				ready.Batches[0].Owners[0].Sector = 8
			case "projection":
				ready.Batches[0].StaticMaterial = false
				ready.Batches[0].Vertices[18] = .5
			}
			data, err := CompileReady(ready, document, layout, CompileOptions{RGBMRange: 1})
			if err == nil || len(data.Vertices) != 0 || len(data.Records) != 0 {
				t.Fatal("invalid source published partial GPU data", err)
			}
		})
	}
}

func TestCompileReadyBatchesChunkWithoutMergingOwnerSurfaceAndProbeRecords(t *testing.T) {
	t.Parallel()
	ready, document, _ := compilerFixture(t)
	document.Sectors = append(document.Sectors, world.Sector{ID: "other"})
	materials, err := worldmaterial.NewLayoutV2([]uint32{7, 9})
	if err != nil {
		t.Fatal(err)
	}
	materials.Materials[1].Coverage = worldmaterial.CoverageMasked
	ready.Surfaces = append(ready.Surfaces, worldchunk.ReadySurface{Primary: 1, Secondary: 1, Band: 1})
	template := ready.Batches[0]
	ready.Batches = nil
	for index := range 4 {
		batch := template
		batch.Vertices = slices.Clone(template.Vertices)
		batch.SurfaceIDs = slices.Clone(template.SurfaceIDs)
		batch.Owners = slices.Clone(template.Owners)
		owner := index % 2
		batch.Owners[0].Sector = owner
		shift := float32(owner*10 + index/2*2)
		if index >= 2 {
			batch.Surface = 1
			batch.Masked = true
			for vertex := range batch.SurfaceIDs {
				batch.SurfaceIDs[vertex] = 1
			}
		}
		for vertex := range 3 {
			batch.Vertices[vertex*worldchunk.ReadyStride] += shift
			if index >= 2 {
				batch.Vertices[vertex*worldchunk.ReadyStride+5] = -1
			}
		}
		batch.Owners[0].Bounds = [6]float64{float64(shift), 0, 0, float64(shift) + 1, 1, 0}
		ready.Batches = append(ready.Batches, batch)
	}
	data, err := CompileReady(ready, document, materials, CompileOptions{RGBMRange: 1, SectorProbes: []uint32{2, 5}})
	if err != nil {
		t.Fatal(err)
	}
	if len(data.RecordRefs) != 4 || len(data.Draws) != 1 || len(data.Vertices) != 12*VertexStride {
		t.Fatal("chunk batching changed immutable records/vertices", len(data.RecordRefs), len(data.Draws))
	}
	if data.RecordRefs[0].SurfaceKind != 1 || data.RecordRefs[2].SurfaceKind != 2 {
		t.Fatal("floor/ceiling record identity merged", data.RecordRefs)
	}
	if binary.LittleEndian.Uint32(data.Records[2*RecordStride+296:]) != 1 {
		t.Fatal("mixed masked material lost its record alpha")
	}
	want := []uint32{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}
	for index, address := range want {
		if binary.LittleEndian.Uint32(data.Indices[index*IndexStride:]) != address {
			t.Fatal("owner index range was not prepared offline", index)
		}
	}
	draw := data.Draws[0]
	if draw.FirstIndex != 0 || draw.IndexCount != 12 || draw.Record != 0 || draw.AlphaMode != 0 ||
		!slices.Equal(draw.Sectors, []uint32{0, 1}) || draw.Bounds[0] != 0 || draw.Bounds[3] != 13 {
		t.Fatal("chunk batch lost independent owners or their geometry bounds", draw)
	}
	for index, probe := range []uint32{2, 5, 2, 5} {
		if binary.LittleEndian.Uint32(data.Records[index*RecordStride+308:]) != probe {
			t.Fatal("chunk batching replaced independent reflection slots", index)
		}
		for vertex := range 3 {
			if binary.LittleEndian.Uint32(data.Addresses[(index*3+vertex)*AddressStride+4:]) != uint32(index) {
				t.Fatal("chunk batching rewrote per-address record identity", index, vertex)
			}
		}
	}
}
