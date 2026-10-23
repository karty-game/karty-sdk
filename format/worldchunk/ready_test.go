package worldchunk

import (
	"bytes"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

func readyPlanFixture(t testing.TB, baked bool) (world.Document, PlanResult) {
	t.Helper()
	document := fixture(t)
	document.Sectors[1].FloorMaterial = 99
	plan, err := Plan(
		document,
		Options{NoLightmap: !baked, MaxSectors: 1, Lightmap: worldlightmap.Options{PageSize: 512, TexelsPerUnit: 2}},
	)
	if err != nil {
		t.Fatal(err)
	}
	return document, plan
}

func readyFixture(t testing.TB, baked bool) (world.Document, Mesh, []byte, *worldlightmap.Layout, Ready) {
	t.Helper()
	document, plan := readyPlanFixture(t, baked)
	mesh := plan.Chunks[0].Mesh
	encoded, err := EncodeMesh(mesh, &document)
	if err != nil {
		t.Fatal(err)
	}
	var layout *worldlightmap.Layout
	if baked {
		layout = &plan.Chunks[0].Layout
	}
	ready, err := CompileReady(&mesh, encoded, &document, layout, ReadyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return document, mesh, encoded, layout, ready
}

func TestReadyCanonicalRoundTripPreservesAttributesAndOwners(t *testing.T) {
	for _, baked := range []bool{false, true} {
		t.Run(map[bool]string{false: "unbaked", true: "baked"}[baked], func(t *testing.T) {
			document, mesh, encodedMesh, layout, ready := readyFixture(t, baked)
			encoded, err := EncodeReady(ready)
			if err != nil {
				t.Fatal(err)
			}
			if string(encoded[:4]) != "KCR1" {
				t.Fatal("ready publication is not bounded scalar binary")
			}
			decoded, err := DecodeReady(encoded, &mesh, encodedMesh, &document, layout, ReadyOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(ready, decoded) {
				t.Fatal("ready roundtrip changed scalar attributes")
			}
			again, err := EncodeReady(decoded)
			if err != nil || !bytes.Equal(encoded, again) {
				t.Fatal("ready encoding is not canonical", err)
			}
			fresh, err := CompileReady(&mesh, encodedMesh, &document, layout, ReadyOptions{})
			if err != nil {
				t.Fatal(err)
			}
			freshBytes, err := EncodeReady(fresh)
			if err != nil || !bytes.Equal(encoded, freshBytes) {
				t.Fatal("repeated offline preparation is nondeterministic", err)
			}
			var sources int
			for _, batch := range ready.Batches {
				sources += len(batch.Sources)
				for index, source := range batch.Sources {
					triangle := mesh.Triangles[source]
					for corner := range 3 {
						vertex := mesh.Vertices[int(source)*3+corner]
						packed := batch.Vertices[(index*3+corner)*ReadyStride:]
						if !slices.Equal(packed[:3], vertex.Position[:]) || !slices.Equal(packed[3:6], triangle.Normal[:]) ||
							!slices.Equal(packed[24:26], vertex.BandUV[:]) {
							t.Fatal("offline packing changed source position, normal or band UV")
						}
						if !slices.Equal(batch.WaveUV[(index*3+corner)*2:(index*3+corner+1)*2], vertex.WaveUV[:]) {
							t.Fatal("offline packing changed wave coordinates")
						}
					}
				}
				var covered int
				for _, owner := range batch.Owners {
					if owner.First != covered {
						t.Fatal("owner ranges lost source ordering")
					}
					covered += owner.Count
				}
				if covered != len(batch.Indices) {
					t.Fatal("owner ranges omitted indices")
				}
			}
			if sources != len(mesh.Triangles) {
				t.Fatal("ready batches omitted source geometry")
			}
		})
	}
}

func TestReadySharedSurfaceIDsRemainStableAcrossChunkOrder(t *testing.T) {
	document, plan := readyPlanFixture(t, false)
	meshes := make([]Mesh, len(plan.Chunks))
	for index, chunk := range plan.Chunks {
		meshes[index] = chunk.Mesh
	}
	table, err := ReadySurfaces(meshes, &document)
	if err != nil {
		t.Fatal(err)
	}
	if len(table) < 2 {
		t.Fatal("fixture requires distinct surface slots")
	}
	slices.Reverse(meshes)
	reversed, err := ReadySurfaces(meshes, &document)
	if err != nil || !slices.Equal(table, reversed) {
		t.Fatal("surface identity depends on chunk arrival order", err)
	}
	// A caller may supply a different canonical order; IDs must preserve it.
	supplied := slices.Clone(table)
	slices.Reverse(supplied)
	for _, mesh := range meshes {
		encodedMesh, err := EncodeMesh(mesh, &document)
		if err != nil {
			t.Fatal(err)
		}
		options := ReadyOptions{Surfaces: supplied}
		ready, err := CompileReady(&mesh, encodedMesh, &document, nil, options)
		if err != nil || !slices.Equal(ready.Surfaces, supplied) {
			t.Fatal("chunk rewrote the level-wide table", err)
		}
		encoded, err := EncodeReady(ready)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := DecodeReady(encoded, &mesh, encodedMesh, &document, nil, options); err != nil {
			t.Fatal(err)
		}
		if err := ValidateReady(&ready, &mesh, encodedMesh, &document, nil, ReadyOptions{Surfaces: table}); err == nil {
			t.Fatal("different global slot order was accepted")
		}
		original := ready.Surfaces[0]
		supplied[0].Primary++
		if ready.Surfaces[0] != original {
			t.Fatal("compiled table aliases caller-owned options")
		}
		supplied[0] = original
	}
	for _, bad := range [][]ReadySurface{{}, append(slices.Clone(table), table[0]), {{Primary: math.MaxInt32}}} {
		mesh := meshes[0]
		encoded, err := EncodeMesh(mesh, &document)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := CompileReady(&mesh, encoded, &document, nil, ReadyOptions{Surfaces: bad}); err == nil {
			t.Fatal("invalid shared surface table accepted")
		}
	}
	missingRejected := false
	for _, mesh := range meshes {
		encoded, err := EncodeMesh(mesh, &document)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := CompileReady(&mesh, encoded, &document, nil, ReadyOptions{Surfaces: table[:1]}); err != nil {
			missingRejected = true
		}
	}
	if !missingRejected {
		t.Fatal("shared table silently grew to fit missing keys")
	}
}

func rejectReadyMutation(
	t *testing.T,
	document world.Document,
	mesh Mesh,
	encodedMesh []byte,
	layout *worldlightmap.Layout,
	source Ready,
	mutate func(*Ready),
) {
	t.Helper()
	encoded, err := EncodeReady(source)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := decodeReadyBinary(encoded)
	if err != nil {
		t.Fatal(err)
	}
	mutate(&changed)
	if err := ValidateReady(&changed, &mesh, encodedMesh, &document, layout, ReadyOptions{}); err == nil {
		t.Fatal("corrupt renderer sidecar passed semantic validation")
	}
	encoded, err = EncodeReady(changed)
	if err != nil {
		return
	}
	if _, err := DecodeReady(encoded, &mesh, encodedMesh, &document, layout, ReadyOptions{}); err == nil {
		t.Fatal("corrupt renderer sidecar decoded")
	}
}

func TestReadyRejectsGeometryAttributesCoverageAndOwnerCorruption(t *testing.T) {
	document, mesh, encodedMesh, layout, ready := readyFixture(t, true)
	mutations := map[string]func(*Ready){
		"digest":               func(ready *Ready) { ready.MeshSHA256 = strings.Repeat("f", 64) },
		"vertex position":      func(ready *Ready) { ready.Batches[0].Vertices[0] += .1 },
		"normal":               func(ready *Ready) { ready.Batches[0].Vertices[3] += .1 },
		"UV":                   func(ready *Ready) { ready.Batches[0].Vertices[6] += .1 },
		"wave UV":              func(ready *Ready) { ready.Batches[0].WaveUV[0] += .1 },
		"lightmap UV":          func(ready *Ready) { ready.Batches[0].LightmapUV[0] += .1 },
		"lightmap basis":       func(ready *Ready) { ready.Batches[0].LightmapBasis[0] += .1 },
		"material basis":       func(ready *Ready) { ready.Batches[0].MaterialBasis[0] += .1 },
		"source duplicate":     func(ready *Ready) { ready.Batches[0].Sources[1] = ready.Batches[0].Sources[0] },
		"source out of range":  func(ready *Ready) { ready.Batches[0].Sources[0] = math.MaxUint32 },
		"surface out of range": func(ready *Ready) { ready.Batches[0].SurfaceIDs[0] = math.MaxUint32 },
		"batch material":       func(ready *Ready) { ready.Batches[0].Surface = (ready.Batches[0].Surface + 1) % len(ready.Surfaces) },
		"missing batch":        func(ready *Ready) { ready.Batches = ready.Batches[:len(ready.Batches)-1] },
		"duplicate batch":      func(ready *Ready) { ready.Batches = append(ready.Batches, ready.Batches[0]) },
		"owner bounds":         func(ready *Ready) { ready.Batches[0].Owners[0].Bounds[0] -= .1 },
		"owner source":         func(ready *Ready) { ready.Batches[0].Owners[0].Sector++ },
		"owner omission":       func(ready *Ready) { ready.Batches[0].Owners[0].Count -= 3 },
		"owner offset":         func(ready *Ready) { ready.Batches[0].Owners[0].First = 3 },
		"static flag":          func(ready *Ready) { ready.Batches[0].StaticMaterial = !ready.Batches[0].StaticMaterial },
		"sky flag":             func(ready *Ready) { ready.Batches[0].Sky = !ready.Batches[0].Sky },
		"masked flag":          func(ready *Ready) { ready.Batches[0].Masked = !ready.Batches[0].Masked },
		"winding":              func(ready *Ready) { ready.Batches[0].Winding = -ready.Batches[0].Winding },
	}
	for name, mutation := range mutations {
		t.Run(name, func(t *testing.T) { rejectReadyMutation(t, document, mesh, encodedMesh, layout, ready, mutation) })
	}
	changedMesh := bytes.Clone(encodedMesh)
	changedMesh[len(changedMesh)-1] ^= 1
	if err := ValidateReady(&ready, &mesh, changedMesh, &document, layout, ReadyOptions{}); err == nil {
		t.Fatal("changed semantic mesh bytes retained sidecar association")
	}
}

func TestReadyRejectsReorderedSourceStreams(t *testing.T) {
	document, mesh, encodedMesh, layout, ready := readyFixture(t, false)
	rejectReadyMutation(t, document, mesh, encodedMesh, layout, ready, func(changed *Ready) {
		batch := &changed.Batches[0]
		batch.Sources[0], batch.Sources[1] = batch.Sources[1], batch.Sources[0]
		for _, stream := range []struct {
			values []float32
			stride int
		}{
			{batch.Vertices, ReadyStride}, {batch.WaveUV, 2}, {batch.LightmapUV, 2}, {batch.LightmapBasis, 6}, {batch.MaterialBasis, 8},
		} {
			if len(stream.values) == 0 {
				continue
			}
			for index := range stream.stride * 3 {
				stream.values[index], stream.values[index+stream.stride*3] = stream.values[index+stream.stride*3], stream.values[index]
			}
		}
		for index := range 3 {
			batch.SurfaceIDs[index], batch.SurfaceIDs[index+3] = batch.SurfaceIDs[index+3], batch.SurfaceIDs[index]
		}
	})
}

func TestReadyBinaryRejectsTruncationCountsAndFlags(t *testing.T) {
	document, mesh, encodedMesh, layout, ready := readyFixture(t, false)
	encoded, err := EncodeReady(ready)
	if err != nil {
		t.Fatal(err)
	}
	batch := readyHeaderBytes + len(ready.Surfaces)*readySurfaceBytes
	cases := [][]byte{
		encoded[:readyHeaderBytes-1],
		encoded[:batch-1],
		encoded[:batch+readyBatchHeaderBytes-1],
		encoded[:len(encoded)-1],
		append(bytes.Clone(encoded), 0),
		changedWord(encoded, 4, readyHeaderBytes<<16|2),
		changedWord(encoded, 40, math.MaxUint32),
		changedWord(encoded, 44, math.MaxUint32),
		changedWord(encoded, readyHeaderBytes+16, 3),
		changedWord(encoded, readyHeaderBytes+20, 4),
		changedWord(encoded, batch+4, 64),
		changedWord(encoded, batch+8, 2),
		changedWord(encoded, batch+12, math.MaxUint32),
	}
	for index, raw := range cases {
		if _, err := DecodeReady(raw, &mesh, encodedMesh, &document, layout, ReadyOptions{}); err == nil {
			t.Fatal("malformed binary accepted", index)
		}
	}
	for end := range readyHeaderBytes {
		if _, err := decodeReadyBinary(encoded[:end]); err == nil {
			t.Fatal("truncated header accepted", end)
		}
	}
}

func BenchmarkReadyPublication(b *testing.B) {
	document, mesh, encodedMesh, layout, ready := readyFixture(b, false)
	encoded, err := EncodeReady(ready)
	if err != nil {
		b.Fatal(err)
	}
	b.Run("validate", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if err := ValidateReady(&ready, &mesh, encodedMesh, &document, layout, ReadyOptions{}); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("decode", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := DecodeReady(encoded, &mesh, encodedMesh, &document, layout, ReadyOptions{}); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("compile", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := CompileReady(&mesh, encodedMesh, &document, layout, ReadyOptions{}); err != nil {
				b.Fatal(err)
			}
		}
	})
}
