package worldchunk

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

func compactFixture(t testing.TB) world.Document {
	t.Helper()
	document := fixture(t)
	document.Contents = nil
	document.Environment = &world.Environment{Version: 1, Preset: world.EnvironmentDaylight}
	document.Sectors = nil
	for i, span := range [][2]float64{{0, 2}, {2, 22}, {22, 24}} {
		points := []world.Vec2{{X: span[0]}, {X: span[1]}, {X: span[1], Y: 20}, {X: span[0], Y: 20}}
		sector := world.Sector{ID: []string{"west", "middle", "east"}[i], Floor: world.Plane{}, Ceiling: world.Plane{C: 30}, Sky: true}
		sector.SourceRoom = sector.ID
		for edge, start := range points {
			wall := world.Wall{Start: start, End: points[(edge+1)%4], Portal: -1, Sky: true}
			if edge == 1 && i < 2 {
				wall.Portal, wall.PortalWall, wall.Sky = int32(i+1), 4, false
			}
			if edge == 3 && i > 0 {
				wall.Portal, wall.PortalWall, wall.Sky = int32(i-1), 2, false
			}
			sector.Walls = append(sector.Walls, wall)
		}
		document.Sectors = append(document.Sectors, sector)
	}
	return document
}

func TestCompactLightmapsReusesPageAcrossOrdinaryReceiverGroups(t *testing.T) {
	document := compactFixture(t)
	options := Options{Lightmap: worldlightmap.Options{PageSize: 512, TexelsPerUnit: 24, CapsWithinSourceRoom: true, CompactPacking: true}}
	baseline, err := Plan(document, options)
	if err != nil || len(baseline.Chunks) != 3 {
		t.Fatalf("expected three connected greedy groups: %v", err)
	}
	options.CompactLightmaps = true
	plan, err := Plan(document, options)
	if err != nil || len(plan.Chunks) != 2 || !slices.Equal(plan.Chunks[0].Sectors, []int{0, 2}) {
		t.Fatalf("complementary physical receiver groups did not share a page: %v", err)
	}
	if !slices.Equal(plan.SectorChunks, []int{0, 1, 0}) {
		t.Fatal("global sector ownership was lost")
	}
	for _, set := range plan.Candidates {
		if !slices.Equal(set, []int{0, 1}) {
			t.Fatal("compaction changed complete visibility candidates")
		}
	}
	for _, chunk := range plan.Chunks {
		if chunk.Layout.TexelsPerUnit != 24 || chunk.Layout.Pages[0] != (worldlightmap.Page{Width: 512, Height: 512}) ||
			chunk.LightmapBytes != 512*512*12 {
			t.Fatal("compaction changed density or map admission costs")
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
	catalog := compactCatalog(t, document, plan)
	encoded, err := EncodeCatalog(catalog, &document)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeCatalog(encoded, &document); err != nil {
		t.Fatal(err)
	}
	again, err := Plan(document, options)
	before, _ := json.Marshal(plan)
	after, _ := json.Marshal(again)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("compact page identities changed: %v", err)
	}
}

func TestCompactLightmapsRetainsGroupsHardBudgetsAndPhysicalBoundaries(t *testing.T) {
	document := compactFixture(t)
	options := Options{
		CompactLightmaps: true,
		Lightmap:         worldlightmap.Options{PageSize: 512, TexelsPerUnit: 24, CapsWithinSourceRoom: true, CompactPacking: true},
	}
	for name, update := range map[string]func(*Options){
		"sector limit":    func(o *Options) { o.MaxSectors = 1 },
		"authored groups": func(o *Options) { o.SectorGroups = []int{0, 0, 1} },
	} {
		t.Run(name, func(t *testing.T) {
			limited := options
			update(&limited)
			plan, err := Plan(document, limited)
			if err != nil || len(plan.Chunks) != 3 {
				t.Fatalf("compaction bypassed grouping guard: %v", err)
			}
		})
	}
	for _, limited := range []Options{{NoLightmap: true, CompactLightmaps: true}, {CompactLightmaps: true, MaxMeshBytes: 1}, {CompactLightmaps: true, MaxEncodedBytes: 1}} {
		if _, err := Plan(document, limited); err == nil {
			t.Fatal("compaction bypassed exact mesh admission")
		}
	}
	for edge := range document.Sectors[2].Walls {
		document.Sectors[2].Walls[edge].Start.X += 100
		document.Sectors[2].Walls[edge].End.X += 100
	}
	plan, err := Plan(document, options)
	if err != nil || len(plan.Chunks) != 3 {
		t.Fatalf("compaction crossed transformed physical space: %v", err)
	}
	if connected(&document, []int{0, 2}) {
		t.Fatal("transformed membership accepted as one physical component")
	}
	// A current geometry digest must not let a producer claim transformed or
	// disconnected receiver membership in an otherwise complete catalog.
	combined := plan
	combined.Chunks = []PlannedChunk{plan.Chunks[0], plan.Chunks[1]}
	combined.Chunks[0].Sectors = []int{0, 2}
	combined.SectorChunks = []int{0, 1, 0}
	combined.Candidates = [][]int{{0, 1}, {0, 1}, {0, 1}}
	catalog := compactCatalog(t, document, combined)
	if _, err := EncodeCatalog(catalog, &document); err == nil {
		t.Fatal("catalog accepted transformed receiver membership")
	}
	document.Sectors[1].Walls[1].Portal, document.Sectors[1].Walls[1].PortalWall = -1, 0
	document.Sectors[1].Walls[1].Sky = true
	document.Sectors[2].Walls[3].Portal, document.Sectors[2].Walls[3].PortalWall = -1, 0
	document.Sectors[2].Walls[3].Sky = true
	if err := world.Validate(&document); err != nil {
		t.Fatal(err)
	}
	if connected(&document, []int{0, 2}) {
		t.Fatal("disconnected membership accepted as one physical component")
	}
	catalog.GeometrySHA256, err = GeometryDigest(&document)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := EncodeCatalog(catalog, &document); err == nil {
		t.Fatal("catalog accepted disconnected receiver membership")
	}
}

func compactCatalog(t testing.TB, document world.Document, plan PlanResult) Catalog {
	t.Helper()
	digest, err := GeometryDigest(&document)
	if err != nil {
		t.Fatal(err)
	}
	catalog := Catalog{Schema: Schema, GeometrySHA256: digest, SectorChunks: plan.SectorChunks, Candidates: plan.Candidates}
	for i, chunk := range plan.Chunks {
		catalog.Chunks = append(catalog.Chunks, Chunk{
			ID:      i,
			Sectors: chunk.Sectors,
			Solids:  chunk.Solids,
			ContentSHA256: strings.Repeat(
				"a",
				64,
			),
			Size:          123,
			MeshBytes:     chunk.MeshBytes,
			LightmapBytes: chunk.LightmapBytes,
			Bounds:        chunk.Bounds,
		})
	}
	return catalog
}
