package worldchunk

import (
	"slices"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
)

func readyBatchFixture(t testing.TB) (world.Document, Mesh, []byte, Ready) {
	t.Helper()
	document := fixture(t)
	document.Environment = &world.Environment{Version: 1, Preset: world.EnvironmentDaylight}
	document.Sectors[0].Sky = true
	plan, err := Plan(document, Options{NoLightmap: true, MaxSectors: 1})
	if err != nil {
		t.Fatal(err)
	}
	mesh := plan.Chunks[0].Mesh
	encoded, err := EncodeMesh(mesh, &document)
	if err != nil {
		t.Fatal(err)
	}
	ready, err := CompileReady(&mesh, encoded, &document, nil, ReadyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(ready.Batches) < 2 || len(ready.Batches[0].Sources) < 2 {
		t.Fatal("fixture requires multiple batches and a splittable batch")
	}
	return document, mesh, encoded, ready
}

func readyBatchPart(t *testing.T, source ReadyBatch, from, to int, mesh Mesh, document *world.Document) ReadyBatch {
	t.Helper()
	result := source
	result.Sources = source.Sources[from:to]
	result.SurfaceIDs = source.SurfaceIDs[from*3 : to*3]
	result.Surface = int(result.SurfaceIDs[0])
	result.Vertices = source.Vertices[from*3*ReadyStride : to*3*ReadyStride]
	result.WaveUV = source.WaveUV[from*6 : to*6]
	if len(source.LightmapUV) > 0 {
		result.LightmapUV = source.LightmapUV[from*6 : to*6]
		result.LightmapBasis = source.LightmapBasis[from*18 : to*18]
	}
	if len(source.MaterialBasis) > 0 {
		result.MaterialBasis = source.MaterialBasis[from*24 : to*24]
	}
	result.Indices = make([]uint32, len(result.SurfaceIDs))
	for index := range result.Indices {
		result.Indices[index] = uint32(index)
	}
	result.Owners = nil
	materials, _, err := readyMaterials(document, ReadyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for index, id := range result.Sources {
		readyAppendOwner(&result, &mesh, int(id), index*3, materials[mesh.Triangles[id].Material].amplitude)
	}
	return result
}

func TestReadyRejectsEquivalentBatchReorderingAndSplitting(t *testing.T) {
	document, mesh, encoded, ready := readyBatchFixture(t)
	t.Run("draw order", func(t *testing.T) {
		rejectReadyMutation(
			t,
			document,
			mesh,
			encoded,
			nil,
			ready,
			func(changed *Ready) { changed.Batches[0], changed.Batches[1] = changed.Batches[1], changed.Batches[0] },
		)
	})
	t.Run("draw amplification", func(t *testing.T) {
		rejectReadyMutation(t, document, mesh, encoded, nil, ready, func(changed *Ready) {
			source := changed.Batches[0]
			first := readyBatchPart(t, source, 0, 1, mesh, &document)
			second := readyBatchPart(t, source, 1, len(source.Sources), mesh, &document)
			rest := slices.Clone(changed.Batches[1:])
			changed.Batches = append([]ReadyBatch{first, second}, rest...)
		})
	})
}
