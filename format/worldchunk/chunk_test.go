package worldchunk

import (
	"bytes"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/karty-game/karty-sdk/format/cartridge"
	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

func fixture(t testing.TB) world.Document {
	t.Helper()
	encoded, err := os.ReadFile("../world/testdata/two-room.world.bin")
	if err != nil {
		t.Fatal(err)
	}
	document, err := world.Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	document.Version = world.Version
	document.Sectors[0].Walls[1].PortalWall = 4
	document.Sectors[1].Walls[3].PortalWall = 2
	return document
}

func TestPlanBudgetsAndConservativeVisibility(t *testing.T) {
	document := fixture(t)
	options := Options{MaxSectors: 1, Lightmap: worldlightmap.Options{PageSize: 512, TexelsPerUnit: 2}}
	first, err := Plan(document, options)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Plan(document, options)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(first)
	b, _ := json.Marshal(second)
	if !bytes.Equal(a, b) || len(first.Chunks) != 2 || !slices.Equal(first.SectorChunks, []int{0, 1}) {
		t.Fatal("nondeterministic or wrong partition")
	}
	for _, set := range first.Candidates {
		if !slices.Equal(set, []int{0, 1}) {
			t.Fatal("reachable chunk omitted")
		}
	}
	for _, chunk := range first.Chunks {
		if chunk.Layout.TexelsPerUnit != 2 || chunk.MeshBytes != DecodedMeshBytes(chunk.Mesh) {
			t.Fatal("density or mesh byte budget changed")
		}
		encoded, err := EncodeMesh(chunk.Mesh, &document)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := DecodeMesh(encoded, &document)
		if err != nil {
			t.Fatal(err)
		}
		if err := ValidateMeshBinding(&decoded, &chunk.Layout); err != nil {
			t.Fatal(err)
		}
	}
	one, err := Plan(document, Options{NoLightmap: true})
	if err != nil || len(one.Chunks) != 1 {
		t.Fatalf("mesh-only ordinary adjacency: %v", err)
	}
	for _, triangle := range one.Chunks[0].Mesh.Triangles {
		if triangle.Chart != -1 {
			t.Fatal("mesh-only chunk has chart")
		}
	}
	if _, err := Plan(
		document,
		Options{NoLightmap: true, MaxMeshBytes: 1},
	); err == nil ||
		!strings.Contains(err.Error(), "cannot fit one chunk") {
		t.Fatal("oversized single sector did not fail explicitly")
	}
	limited, err := Plan(document, Options{NoLightmap: true, MaxMeshBytes: first.Chunks[0].MeshBytes + 512})
	if err != nil || len(limited.Chunks) != 2 {
		t.Fatalf("mesh budget ignored: %v", err)
	}
}

func TestTransformedPortalIsChunkBoundaryAndReachable(t *testing.T) {
	document := fixture(t)
	for i := range document.Sectors[1].Walls {
		document.Sectors[1].Walls[i].Start.X += 100
		document.Sectors[1].Walls[i].End.X += 100
	}
	document.Sectors[1].Floor.C -= document.Sectors[1].Floor.A * 100
	document.Sectors[1].Ceiling.C -= document.Sectors[1].Ceiling.A * 100
	document.Contents[0].Position.X += 100
	plan, err := Plan(document, Options{NoLightmap: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Chunks) != 2 {
		t.Fatal("transformed spaces were combined")
	}
	for _, set := range plan.Candidates {
		if !slices.Equal(set, []int{0, 1}) {
			t.Fatal("transformed destination omitted")
		}
	}
	for _, chunk := range plan.Chunks {
		for _, triangle := range chunk.Mesh.Triangles {
			if triangle.Sector != chunk.Sectors[0] {
				t.Fatal("chunk changed global sector identity")
			}
		}
	}
}

func TestMeshAndCatalogValidateCompletePayload(t *testing.T) {
	document := fixture(t)
	plan, err := Plan(document, Options{NoLightmap: true, MaxSectors: 1})
	if err != nil {
		t.Fatal(err)
	}
	mesh := plan.Chunks[0].Mesh
	mutations := map[string]func(*Mesh){
		"last index":    func(m *Mesh) { m.Indices[len(m.Indices)-1] = uint32(len(m.Vertices)) },
		"last material": func(m *Mesh) { m.Triangles[len(m.Triangles)-1].Material = 999 },
		"last normal":   func(m *Mesh) { m.Triangles[len(m.Triangles)-1].Normal = [3]float32{} },
		"missing triangle": func(m *Mesh) {
			m.Triangles = m.Triangles[:len(m.Triangles)-1]
			m.Vertices = m.Vertices[:len(m.Vertices)-3]
			m.Indices = m.Indices[:len(m.Indices)-3]
		},
		"bounds":      func(m *Mesh) { m.Bounds[3]++ },
		"stale world": func(m *Mesh) { m.GeometrySHA256 = strings.Repeat("0", 64) },
		"last owner":  func(m *Mesh) { m.Triangles[len(m.Triangles)-1].Sector = 1 },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			copyMesh := mesh
			copyMesh.Vertices = slices.Clone(mesh.Vertices)
			copyMesh.Indices = slices.Clone(mesh.Indices)
			copyMesh.Triangles = slices.Clone(mesh.Triangles)
			mutate(&copyMesh)
			encoded, err := encodeMeshBinary(copyMesh)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := DecodeMesh(encoded, &document); err == nil {
				t.Fatal("malformed batch accepted")
			}
		})
	}
	encoded, _ := EncodeMesh(mesh, &document)
	for _, suffix := range []string{" ", "{}"} {
		if _, err := DecodeMesh(append(slices.Clone(encoded), []byte(suffix)...), &document); err == nil {
			t.Fatal("noncanonical/trailing data accepted")
		}
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
				Size:          123,
				ContentSHA256: strings.Repeat("a", 64),
				MeshBytes:     chunk.MeshBytes,
				Bounds:        chunk.Bounds,
			},
		)
	}
	encoded, err = EncodeCatalog(catalog, &document)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeCatalog(encoded, &document); err != nil {
		t.Fatal(err)
	}
	catalog.Candidates[1] = []int{1}
	if _, err := EncodeCatalog(catalog, &document); err == nil {
		t.Fatal("missing reachable neighbor accepted")
	}
}

func TestChunkCapabilityDependency(t *testing.T) {
	manifest := cartridge.Manifest{ProjectName: "chunks", Compiler: "test", Width: 1, Height: 1, Features: []string{Feature}}
	if _, err := cartridge.EncodeManifest(manifest); err == nil {
		t.Fatal("chunk feature without world accepted")
	}
	manifest.Features = append(manifest.Features, world.Feature)
	if _, err := cartridge.EncodeManifest(manifest); err != nil {
		t.Fatal(err)
	}
}

func TestReceiverAtlasSplitPreservesDensity(t *testing.T) {
	document := fixture(t)
	document.Sectors[1].Floor = document.Sectors[0].Floor
	document.Sectors[1].Ceiling = document.Sectors[0].Ceiling
	for sector := range document.Sectors {
		for wall := range document.Sectors[sector].Walls {
			document.Sectors[sector].Walls[wall].Start.X *= 10
			document.Sectors[sector].Walls[wall].Start.Y *= 2.5
			document.Sectors[sector].Walls[wall].End.X *= 10
			document.Sectors[sector].Walls[wall].End.Y *= 2.5
		}
	}
	document.Contents[0].Position.X *= 10
	document.Contents[0].Position.Y *= 2.5
	document.Contents[0].Position.Z = 3
	plan, err := Plan(document, Options{Lightmap: worldlightmap.Options{PageSize: 512, TexelsPerUnit: 8}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Chunks) != 2 {
		t.Fatal("oversized merged floor chart did not split")
	}
	for _, chunk := range plan.Chunks {
		if chunk.Layout.TexelsPerUnit != 8 {
			t.Fatal("density silently reduced")
		}
	}
}
