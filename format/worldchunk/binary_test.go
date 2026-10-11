package worldchunk

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
)

func changedWord(raw []byte, at int, value uint32) []byte {
	changed := bytes.Clone(raw)
	binary.LittleEndian.PutUint32(changed[at:at+4], value)
	return changed
}

func binaryCatalogFixture(t testing.TB) (world.Document, Catalog) {
	t.Helper()
	document := fixture(t)
	return document, catalogForDocument(t, document)
}

func catalogForDocument(t testing.TB, document world.Document) Catalog {
	t.Helper()
	plan, err := Plan(document, Options{NoLightmap: true, MaxSectors: 1})
	if err != nil {
		t.Fatal(err)
	}
	digest, err := GeometryDigest(&document)
	if err != nil {
		t.Fatal(err)
	}
	catalog := Catalog{Schema: Schema, GeometrySHA256: digest, SectorChunks: plan.SectorChunks, Candidates: plan.Candidates}
	for _, chunk := range plan.Chunks {
		catalog.Chunks = append(
			catalog.Chunks,
			Chunk{
				ID:            chunk.ID,
				Sectors:       chunk.Sectors,
				Solids:        chunk.Solids,
				ContentSHA256: strings.Repeat("a", 64),
				Size:          256,
				MeshBytes:     chunk.MeshBytes,
				Bounds:        chunk.Bounds,
			},
		)
	}
	return catalog
}

func TestBinaryMeshExactScalarRecordsAndCanonicalRoundTrip(t *testing.T) {
	document := fixture(t)
	plan, err := Plan(document, Options{NoLightmap: true, MaxSectors: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, chunk := range plan.Chunks {
		encoded, err := EncodeMesh(chunk.Mesh, &document)
		if err != nil {
			t.Fatal(err)
		}
		if string(encoded[:4]) != "KCM1" ||
			uint64(len(encoded)) != meshHeaderBytes+4*uint64(len(chunk.Sectors)+len(chunk.Solids))+DecodedMeshBytes(chunk.Mesh) {
			t.Fatal("mesh is not fixed binary scalar records")
		}
		decoded, err := DecodeMesh(encoded, &document)
		if err != nil {
			t.Fatal(err)
		}
		again, err := EncodeMesh(decoded, &document)
		if err != nil || !bytes.Equal(encoded, again) {
			t.Fatalf("binary mesh is not canonical: %v", err)
		}
		if !slices.Equal(decoded.Vertices, chunk.Mesh.Vertices) || !slices.Equal(decoded.Triangles, chunk.Mesh.Triangles) ||
			!slices.Equal(decoded.Indices, chunk.Mesh.Indices) {
			t.Fatal("binary scalar attributes changed")
		}
		vertexStart := meshHeaderBytes + 4*(len(chunk.Sectors)+len(chunk.Solids))
		triangleStart := vertexStart + vertexRecordBytes*len(chunk.Mesh.Vertices) + 4*len(chunk.Mesh.Indices)
		for name, data := range map[string][]byte{
			"reserved header":          changedWord(encoded, 28, 1),
			"version":                  changedWord(encoded, 4, meshHeaderBytes<<16|2),
			"length":                   encoded[:len(encoded)-1],
			"tail":                     append(bytes.Clone(encoded), 0),
			"vertex count":             changedWord(encoded, 16, math.MaxUint32),
			"index count":              changedWord(encoded, 20, 0),
			"triangle count":           changedWord(encoded, 24, math.MaxUint32),
			"surface enum":             changedWord(encoded, triangleStart+8, 7),
			"primary count truncation": changedWord(encoded, triangleStart+56, 256),
			"coverage truncation":      changedWord(encoded, triangleStart+148, 256),
			"repeat flag":              changedWord(encoded, triangleStart+152, 2),
			"sky flag":                 changedWord(encoded, triangleStart+208, 2),
			"nonfinite position":       changedWord(encoded, vertexStart, math.Float32bits(float32(math.NaN()))),
			"nonfinite basis":          changedWord(encoded, triangleStart+16, math.Float32bits(float32(math.Inf(1)))),
		} {
			t.Run(name, func(t *testing.T) {
				if _, err := DecodeMesh(data, &document); !errors.Is(err, ErrChunk) {
					t.Fatalf("malformed binary accepted: %v", err)
				}
			})
		}
	}
}

func TestBinaryCatalogOwnershipAndCandidateLengths(t *testing.T) {
	document, catalog := binaryCatalogFixture(t)
	encoded, err := EncodeCatalog(catalog, &document)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeCatalog(encoded, &document)
	if err != nil {
		t.Fatal(err)
	}
	again, err := EncodeCatalog(decoded, &document)
	if err != nil || !bytes.Equal(encoded, again) {
		t.Fatalf("catalog canonical roundtrip: %v", err)
	}
	if string(encoded[:4]) != "KCC2" || binary.LittleEndian.Uint16(encoded[4:6]) != 2 ||
		len(encoded) != catalogHeaderBytes+chunkRecordBytes*2+4*(2+2+2+4) {
		t.Fatal("catalog descriptor/list layout changed")
	}
	ownershipStart := catalogHeaderBytes + chunkRecordBytes*2
	candidateStart := ownershipStart + 4*(2+2)
	for name, data := range map[string][]byte{
		"bake source count overflow":  changedWord(encoded, 20, world.MaxLights+1),
		"bake source count length":    changedWord(encoded, 20, 1),
		"catalog version":             changedWord(encoded, 4, catalogHeaderBytes<<16|1),
		"header tail reserve":         changedWord(encoded, 56, 1),
		"descriptor reserve":          changedWord(encoded, catalogHeaderBytes+12, 1),
		"chunks overflow":             changedWord(encoded, 8, math.MaxUint32),
		"sector count overflow":       changedWord(encoded, 12, math.MaxUint32),
		"candidate total overflow":    changedWord(encoded, 16, math.MaxUint32),
		"descriptor sectors overflow": changedWord(encoded, catalogHeaderBytes+4, math.MaxUint32),
		"descriptor solids overflow":  changedWord(encoded, catalogHeaderBytes+8, math.MaxUint32),
		"duplicate sector":            changedWord(encoded, ownershipStart+4, 0),
		"owner mismatch":              changedWord(encoded, ownershipStart+8, 1),
		"candidate count overflow":    changedWord(encoded, candidateStart, math.MaxUint32),
		"candidate missing":           changedWord(encoded, candidateStart+4, 1),
		"truncated":                   encoded[:len(encoded)-1],
		"tail":                        append(bytes.Clone(encoded), 0),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeCatalog(data, &document); !errors.Is(err, ErrChunk) {
				t.Fatalf("malformed catalog accepted: %v", err)
			}
		})
	}
}

func TestBinaryPreflightRejectsCountsAndLengthsBeforeAllocation(t *testing.T) {
	document, catalog := binaryCatalogFixture(t)
	plan, err := Plan(document, Options{NoLightmap: true})
	if err != nil {
		t.Fatal(err)
	}
	meshBytes, err := EncodeMesh(plan.Chunks[0].Mesh, &document)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range [][]byte{changedWord(meshBytes, 16, math.MaxUint32), meshBytes[:meshHeaderBytes], append(bytes.Clone(meshBytes), 0)} {
		var mesh Mesh
		if err := decodeMeshBinary(raw, &mesh); err == nil {
			t.Fatal("bad mesh shape accepted")
		}
		if mesh.Vertices != nil || mesh.Indices != nil || mesh.Triangles != nil || mesh.Sectors != nil {
			t.Fatal("mesh allocated before full size/count validation")
		}
	}
	catalogBytes, err := EncodeCatalog(catalog, &document)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range [][]byte{changedWord(catalogBytes, catalogHeaderBytes+4, math.MaxUint32), catalogBytes[:catalogHeaderBytes], append(bytes.Clone(catalogBytes), 0)} {
		var result Catalog
		if err := decodeCatalogBinary(raw, &result); err == nil {
			t.Fatal("bad catalog shape accepted")
		}
		if result.Chunks != nil || result.SectorChunks != nil || result.Candidates != nil {
			t.Fatal("catalog allocated before full size/count validation")
		}
	}
	proxy, err := CompileShadowProxy(document)
	if err != nil {
		t.Fatal(err)
	}
	proxyBytes, err := EncodeShadowProxy(proxy, &document)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range [][]byte{changedWord(proxyBytes, 8, math.MaxUint32), proxyBytes[:shadowHeaderBytes], append(bytes.Clone(proxyBytes), 0)} {
		var result ShadowProxy
		if err := decodeShadowBinary(raw, &result); err == nil {
			t.Fatal("bad shadow shape accepted")
		}
		if result.Triangles != nil {
			t.Fatal("shadow allocated before full size/count validation")
		}
	}
}

func TestBinaryCatalogStoresAllSectorListsBeforeAllSolidLists(t *testing.T) {
	document := fixture(t)
	document.StaticSolids = &world.StaticSolids{Version: 1, Items: []world.Solid{
		{ID: "first", Footprint: []world.Vec2{{X: 1, Y: 1}, {X: 2, Y: 1}, {X: 2, Y: 2}, {X: 1, Y: 2}},
			Bottom: world.Plane{C: .5}, Top: world.Plane{C: 1}, SideMaterial: 3, TopMaterial: 3, BottomMaterial: 3},
		{ID: "second", Footprint: []world.Vec2{{X: 5, Y: 1}, {X: 6, Y: 1}, {X: 6, Y: 2}, {X: 5, Y: 2}},
			Bottom: world.Plane{C: .5}, Top: world.Plane{C: 1}, SideMaterial: 3, TopMaterial: 3, BottomMaterial: 3},
	}}
	catalog := catalogForDocument(t, document)
	encoded, err := EncodeCatalog(catalog, &document)
	if err != nil {
		t.Fatal(err)
	}
	at := catalogHeaderBytes + 2*chunkRecordBytes
	for i, want := range []uint32{0, 1, 0, 1, 0, 1} {
		if binary.LittleEndian.Uint32(encoded[at+4*i:at+4*i+4]) != want {
			t.Fatal("ownership arrays interleaved or lost global identity")
		}
	}
	decoded, err := DecodeCatalog(encoded, &document)
	if err != nil {
		t.Fatal(err)
	}
	for i, chunk := range decoded.Chunks {
		if !slices.Equal(chunk.Sectors, []int{i}) || !slices.Equal(chunk.Solids, []int{i}) {
			t.Fatal("binary ownership lists changed")
		}
	}
	if _, err := DecodeCatalog(changedWord(encoded, at+12, 0), &document); err == nil {
		t.Fatal("duplicate solid ownership accepted")
	}
}

func TestBinaryShadowAllowsEmptyCompleteCasterSet(t *testing.T) {
	document := fixture(t)
	document.Environment = &world.Environment{Version: 1, Preset: world.EnvironmentDaylight}
	for i := range document.Sectors {
		document.Sectors[i].Sky = true
		document.Sectors[i].Floor = world.Plane{}
		document.Sectors[i].Ceiling = world.Plane{C: 4}
		for wall := range document.Sectors[i].Walls {
			document.Sectors[i].Walls[wall].Sky = document.Sectors[i].Walls[wall].Portal < 0
		}
	}
	document.Animations = &world.Animations{Version: 1,
		Presets:   []world.AnimationPreset{{Name: "transparent", Kind: "liquid", Frequency: 1, Opacity: new(0.0)}},
		Materials: []world.MaterialAnimation{{Material: 1, Preset: "transparent"}},
	}
	proxy, err := CompileShadowProxy(document)
	if err != nil || len(proxy.Triangles) != 0 {
		t.Fatalf("unexpected opaque casters: %v", err)
	}
	encoded, err := EncodeShadowProxy(proxy, &document)
	if err != nil || len(encoded) != shadowHeaderBytes {
		t.Fatalf("empty binary proxy: %v", err)
	}
	decoded, err := DecodeShadowProxy(encoded, &document)
	if err != nil || decoded.Triangles == nil || len(decoded.Triangles) != 0 {
		t.Fatalf("empty complete caster set rejected: %v", err)
	}
}

func FuzzBinaryChunkCanonicalRoundTrip(f *testing.F) {
	document, catalog := binaryCatalogFixture(f)
	catalogBytes, err := EncodeCatalog(catalog, &document)
	if err != nil {
		f.Fatal(err)
	}
	plan, err := Plan(document, Options{NoLightmap: true})
	if err != nil {
		f.Fatal(err)
	}
	meshBytes, err := EncodeMesh(plan.Chunks[0].Mesh, &document)
	if err != nil {
		f.Fatal(err)
	}
	proxy, err := CompileShadowProxy(document)
	if err != nil {
		f.Fatal(err)
	}
	shadowBytes, err := EncodeShadowProxy(proxy, &document)
	if err != nil {
		f.Fatal(err)
	}
	for _, seed := range [][]byte{catalogBytes, meshBytes, shadowBytes, changedWord(meshBytes, 16, math.MaxUint32), {}, []byte("KCC1"), []byte("KCC2")} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) < 4 || len(raw) > MaxEncodedSize {
			return
		}
		var canonical []byte
		var err error
		switch string(raw[:4]) {
		case "KCC2":
			value, decodeErr := DecodeCatalog(raw, &document)
			if decodeErr != nil {
				return
			}
			canonical, err = EncodeCatalog(value, &document)
		case "KCM1":
			value, decodeErr := DecodeMesh(raw, &document)
			if decodeErr != nil {
				return
			}
			canonical, err = EncodeMesh(value, &document)
		case "KCS1":
			value, decodeErr := DecodeShadowProxy(raw, &document)
			if decodeErr != nil {
				return
			}
			canonical, err = EncodeShadowProxy(value, &document)
		default:
			return
		}
		if err != nil || !bytes.Equal(raw, canonical) {
			t.Fatalf("accepted binary was not exact canonical records: %v", err)
		}
	})
}
