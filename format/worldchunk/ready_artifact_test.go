package worldchunk

import (
	"bytes"
	"encoding/binary"
	"math"
	"reflect"
	"slices"
	"sync"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
)

func readyArtifactAdmission(document *world.Document, mesh *Mesh, baked bool) ReadyAdmission {
	return ReadyAdmission{
		MaterialCount: len(world.MaterialIDs(document)),
		Sectors:       mesh.Sectors,
		Solids:        mesh.Solids,
		Triangles:     len(mesh.Triangles),
		Lightmap:      baked,
	}
}

func TestReadyArtifactPreservesCompiledBytesAndIndependentPublication(t *testing.T) {
	for _, baked := range []bool{false, true} {
		document, mesh, _, _, ready := readyFixture(t, baked)
		encoded, err := EncodeReady(ready)
		if err != nil {
			t.Fatal(err)
		}
		admission := readyArtifactAdmission(&document, &mesh, baked)
		admission.Surfaces = ready.Surfaces
		decoded, err := DecodeReadyArtifact(encoded, admission)
		if err != nil || !reflect.DeepEqual(decoded, ready) {
			t.Fatal("compiled artifact changed", err)
		}
		again, err := EncodeReady(decoded)
		if err != nil || !bytes.Equal(encoded, again) {
			t.Fatal("compiled scalar wire changed", err)
		}
		decoded.Batches[0].Vertices[0]++
		decoded.Batches[0].Indices[0]++
		decoded.Batches[0].Owners[0].Count++
		decoded.Surfaces[0].Primary++
		if _, err := DecodeReadyArtifact(encoded, admission); err != nil {
			t.Fatal("output aliased input or admission", err)
		}
	}
}

func TestReadyArtifactTrustsBuildProofWithoutReconstructingGeometry(t *testing.T) {
	document, mesh, encodedMesh, layout, ready := readyFixture(t, true)
	// These finite authored-value changes violate the offline proof. Runtime
	// accepts approved compiled bytes structurally; its whole-module digest is
	// the caller's boundary. In particular no rest bounds or UVs are recomputed.
	ready.Batches[0].Vertices[0] += 100
	ready.Batches[0].Vertices[6] += .25
	ready.Batches[0].Vertices[3] = .123
	ready.MeshSHA256 = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	encoded, err := EncodeReady(ready)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeReadyArtifact(encoded, readyArtifactAdmission(&document, &mesh, true)); err != nil {
		t.Fatal("runtime repeated build geometry proof", err)
	}
	if err := ValidateReady(&ready, &mesh, encodedMesh, &document, layout, ReadyOptions{}); err == nil {
		t.Fatal("offline oracle lost full authored proof")
	}
}

func TestReadyArtifactRejectsInvalidAdmissionAndReferences(t *testing.T) {
	document, mesh, _, _, source := readyFixture(t, true)
	encoded, err := EncodeReady(source)
	if err != nil {
		t.Fatal(err)
	}
	admission := readyArtifactAdmission(&document, &mesh, true)
	for name, change := range map[string]func(*ReadyAdmission){
		"zero materials":      func(a *ReadyAdmission) { a.MaterialCount = 0 },
		"material slot bound": func(a *ReadyAdmission) { a.MaterialCount = 1 },
		"no triangles":        func(a *ReadyAdmission) { a.Triangles = 0 },
		"triangle mismatch":   func(a *ReadyAdmission) { a.Triangles++ },
		"triangle overflow":   func(a *ReadyAdmission) { a.Triangles = MaxTriangles + 1 },
		"missing sectors":     func(a *ReadyAdmission) { a.Sectors = nil },
		"negative sector":     func(a *ReadyAdmission) { a.Sectors = []int{-1} },
		"sector limit":        func(a *ReadyAdmission) { a.Sectors = []int{world.MaxSectors} },
		"duplicate sector":    func(a *ReadyAdmission) { a.Sectors = []int{0, 0} },
		"wrong owner":         func(a *ReadyAdmission) { a.Sectors = []int{1} },
		"solid limit":         func(a *ReadyAdmission) { a.Solids = []int{world.MaxStaticSolids} },
		"layout missing":      func(a *ReadyAdmission) { a.Lightmap = false },
		"different table":     func(a *ReadyAdmission) { a.Surfaces = []ReadySurface{} },
	} {
		t.Run(name, func(t *testing.T) {
			changed := admission
			change(&changed)
			if _, err := DecodeReadyArtifact(encoded, changed); err == nil {
				t.Fatal("invalid compiled admission accepted")
			}
		})
	}
	for name, change := range map[string]func(*Ready){
		"mixed triangle surface": func(r *Ready) {
			r.Batches[0].SurfaceIDs[1] = (r.Batches[0].SurfaceIDs[0] + 1) % uint32(len(r.Surfaces))
		},
		"batch surface representative": func(r *Ready) { r.Batches[0].Surface = (r.Batches[0].Surface + 1) % len(r.Surfaces) },
		"surface material bound":       func(r *Ready) { r.Surfaces[0].Primary = admission.MaterialCount },
		"duplicate surface":            func(r *Ready) { r.Surfaces = append(r.Surfaces, r.Surfaces[0]) },
		"source index":                 func(r *Ready) { r.Batches[0].Sources[0] = uint32(admission.Triangles) },
		"source order":                 func(r *Ready) { r.Batches[0].Sources[1] = r.Batches[0].Sources[0] },
		"owner gap":                    func(r *Ready) { r.Batches[0].Owners[0].First = 3 },
		"owner truncated":              func(r *Ready) { r.Batches[0].Owners[0].Count -= 3 },
		"owner reversed count":         func(r *Ready) { r.Batches[0].Owners[0].Count = -3 },
		"owner nontriangle":            func(r *Ready) { r.Batches[0].Owners[0].Count = 1 },
		"owner excessive":              func(r *Ready) { r.Batches[0].Owners[0].Count = math.MaxInt32 },
		"owner both IDs":               func(r *Ready) { r.Batches[0].Owners[0].Solid = 0 },
		"owner neither ID":             func(r *Ready) { r.Batches[0].Owners[0].Sector = -1 },
		"owner invalid sentinel":       func(r *Ready) { r.Batches[0].Owners[0].Solid = -2 },
		"owner unknown solid":          func(r *Ready) { r.Batches[0].Owners[0].Sector = -1; r.Batches[0].Owners[0].Solid = 0 },
		"owner reversed bounds":        func(r *Ready) { r.Batches[0].Owners[0].Bounds[0] = r.Batches[0].Owners[0].Bounds[3] + 1 },
		"owner nonfinite bounds":       func(r *Ready) { r.Batches[0].Owners[0].Bounds[0] = math.NaN() },
		"owner coordinate bound":       func(r *Ready) { r.Batches[0].Owners[0].Bounds[0] = -world.MaxCoordinate - 1 },
		"missing lightmap":             func(r *Ready) { r.Batches[0].LightmapUV = nil; r.Batches[0].LightmapBasis = nil },
	} {
		t.Run(name, func(t *testing.T) {
			changed, err := decodeReadyBinary(encoded)
			if err != nil {
				t.Fatal(err)
			}
			change(&changed)
			bad, err := EncodeReady(changed)
			if err != nil {
				t.Fatal("fixture must be structurally encodable", err)
			}
			if _, err := DecodeReadyArtifact(bad, admission); err == nil {
				t.Fatal("invalid compiled reference accepted")
			}
		})
	}
	// Complete consumption, allocation bounds, flags and scalar finiteness remain
	// required even after a caller verifies the enclosing module digest.
	for _, bad := range [][]byte{nil, encoded[:len(encoded)-1], append(slices.Clone(encoded), 0)} {
		if _, err := DecodeReadyArtifact(bad, admission); err == nil {
			t.Fatal("invalid wire accepted")
		}
	}
	firstVertex := readyHeaderBytes + len(source.Surfaces)*readySurfaceBytes + readyBatchHeaderBytes
	bad := slices.Clone(encoded)
	binary.LittleEndian.PutUint32(bad[firstVertex:], math.Float32bits(float32(math.Inf(1))))
	if _, err := DecodeReadyArtifact(bad, admission); err == nil {
		t.Fatal("nonfinite compiled vertex accepted")
	}
}

func TestReadyArtifactRejectsSplitClassificationAndReorderedBatches(t *testing.T) {
	document, mesh, _, source := readyBatchFixture(t)
	admission := readyArtifactAdmission(&document, &mesh, false)
	for _, split := range []bool{false, true} {
		ready := source
		ready.Batches = slices.Clone(source.Batches)
		if split {
			batch := source.Batches[0]
			ready.Batches = append(
				[]ReadyBatch{
					readyBatchPart(t, batch, 0, 1, mesh, &document),
					readyBatchPart(t, batch, 1, len(batch.Sources), mesh, &document),
				},
				ready.Batches[1:]...)
		} else {
			ready.Batches[0], ready.Batches[1] = ready.Batches[1], ready.Batches[0]
		}
		encoded, err := EncodeReady(ready)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := DecodeReadyArtifact(encoded, admission); err == nil {
			t.Fatal("draw reordering/amplification accepted")
		}
	}
}

func TestReadyArtifactConcurrentDecodeRetainsNoSharedScratch(t *testing.T) {
	document, mesh, _, _, ready := readyFixture(t, false)
	encoded, err := EncodeReady(ready)
	if err != nil {
		t.Fatal(err)
	}
	admission := readyArtifactAdmission(&document, &mesh, false)
	var workers sync.WaitGroup
	for range 4 {
		workers.Go(func() {
			for range 16 {
				decoded, err := DecodeReadyArtifact(encoded, admission)
				if err != nil {
					t.Error(err)
					return
				}
				decoded.Batches[0].Vertices[0]++
				decoded.Surfaces[0].Primary++
			}
		})
	}
	workers.Wait()
	if _, err := DecodeReadyArtifact(encoded, admission); err != nil {
		t.Fatal(err)
	}
}

func BenchmarkReadyArtifact(b *testing.B) {
	document, mesh, encodedMesh, layout, ready := readyFixture(b, true)
	encoded, err := EncodeReady(ready)
	if err != nil {
		b.Fatal(err)
	}
	admission := readyArtifactAdmission(&document, &mesh, true)
	b.Run("offline-full-proof", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := DecodeReady(encoded, &mesh, encodedMesh, &document, layout, ReadyOptions{}); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("compiled-artifact", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := DecodeReadyArtifact(encoded, admission); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func TestReadyArtifactAcceptsOfflineLiquidSkyFrameAndSolidVariants(t *testing.T) {
	for _, variant := range []string{"liquid", "sky", "sky-frame", "solid"} {
		t.Run(variant, func(t *testing.T) {
			document := fixture(t)
			switch variant {
			case "liquid":
				document = liquidFixture(t)
			case "sky":
				document = skyPortalFixture(t, false, false)
			case "sky-frame":
				document = skyPortalFixture(t, false, true)
			case "solid":
				document.StaticSolids = &world.StaticSolids{
					Version: 1,
					Items: []world.Solid{
						{
							ID:             "prepared-solid",
							Footprint:      []world.Vec2{{X: 1, Y: 1}, {X: 2, Y: 1}, {X: 2, Y: 2}, {X: 1, Y: 2}},
							Bottom:         world.Plane{},
							Top:            world.Plane{C: 1},
							SideMaterial:   3,
							TopMaterial:    3,
							BottomMaterial: 3,
						},
					},
				}
			}
			plan, err := Plan(document, Options{NoLightmap: true, MaxSectors: 1})
			if err != nil {
				t.Fatal(err)
			}
			for _, chunk := range plan.Chunks {
				encodedMesh, err := EncodeMesh(chunk.Mesh, &document)
				if err != nil {
					t.Fatal(err)
				}
				ready, err := CompileReady(&chunk.Mesh, encodedMesh, &document, nil, ReadyOptions{})
				if err != nil {
					t.Fatal(err)
				}
				if err := ValidateReady(&ready, &chunk.Mesh, encodedMesh, &document, nil, ReadyOptions{}); err != nil {
					t.Fatal(err)
				}
				encoded, err := EncodeReady(ready)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := DecodeReadyArtifact(encoded, readyArtifactAdmission(&document, &chunk.Mesh, false)); err != nil {
					t.Fatal("offline-approved variant refused", err)
				}
			}
		})
	}
}
