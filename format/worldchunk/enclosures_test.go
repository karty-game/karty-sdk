package worldchunk

import (
	"slices"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

// Two enclosures are interleaved in authored order, with a narrow doorway
// between their first cells. Authored-order growth steals the neighbor's cell
// and leaves two fragments; enclosure growth can retain two complete spaces.
func enclosureFixture() world.Document {
	polygon := func(id, source string, points ...world.Vec2) world.Sector {
		sector := world.Sector{ID: id, SourceRoom: source, Ceiling: world.Plane{C: 4}}
		for index, point := range points {
			sector.Walls = append(sector.Walls, world.Wall{
				Start: point, End: points[(index+1)%len(points)], Portal: -1, SourceEdge: "boundary",
			})
		}
		return sector
	}
	document := world.Document{Version: world.Version, Sectors: []world.Sector{
		polygon("a#0", "a", world.Vec2{X: 0, Y: 0}, world.Vec2{X: 3, Y: 0}, world.Vec2{X: 4, Y: 1.5},
			world.Vec2{X: 4, Y: 2.5}, world.Vec2{X: 3, Y: 4}, world.Vec2{X: 0, Y: 4}),
		polygon("b#0", "b", world.Vec2{X: 5, Y: 0}, world.Vec2{X: 8, Y: 0}, world.Vec2{X: 8, Y: 4},
			world.Vec2{X: 5, Y: 4}, world.Vec2{X: 4, Y: 2.5}, world.Vec2{X: 4, Y: 1.5}),
		polygon("a#1", "a", world.Vec2{X: 0, Y: 4}, world.Vec2{X: 3, Y: 4}, world.Vec2{X: 3, Y: 8}, world.Vec2{X: 0, Y: 8}),
		polygon("b#1", "b", world.Vec2{X: 5, Y: 4}, world.Vec2{X: 8, Y: 4}, world.Vec2{X: 8, Y: 8}, world.Vec2{X: 5, Y: 8}),
	}}
	connect := func(left, edge, right, target int, internal bool) {
		a, b := &document.Sectors[left].Walls[edge], &document.Sectors[right].Walls[target]
		a.Portal, b.Portal = int32(right), int32(left)
		a.PortalWall, b.PortalWall = uint16(target+1), uint16(edge+1)
		if internal {
			a.SourceEdge, b.SourceEdge = "", ""
		}
	}
	connect(0, 2, 1, 4, false)
	connect(0, 4, 2, 0, true)
	connect(1, 2, 3, 0, true)
	return document
}

func TestEnclosurePlanningKeepsCompleteSpacesBeforeCrossingDoorway(t *testing.T) {
	t.Parallel()
	document := enclosureFixture()
	legacy, err := Plan(document, Options{NoLightmap: true, MaxSectors: 2})
	if err != nil || len(legacy.Chunks) != 3 || legacy.Enclosures != nil {
		t.Fatal("legacy policy changed", legacy.SectorChunks, err)
	}
	options := Options{NoLightmap: true, MaxSectors: 2, EnclosureAware: true}
	plan, err := Plan(document, options)
	if err != nil || len(plan.Chunks) != 2 || !slices.Equal(plan.SectorChunks, []int{0, 1, 0, 1}) ||
		!slices.Equal(plan.Enclosures, []int{0, 1, 0, 1}) {
		t.Fatal("enclosures fragmented", plan, err)
	}
	repeat, err := Plan(document, options)
	if err != nil || !slices.Equal(plan.SectorChunks, repeat.SectorChunks) {
		t.Fatal("nondeterministic plan", err)
	}
	for _, candidates := range plan.Candidates {
		if !slices.Equal(candidates, []int{0, 1}) {
			t.Fatal("doorway reachability changed", candidates)
		}
	}
}

func TestEnclosureIdentitySurvivesBudgetsAndPacking(t *testing.T) {
	t.Parallel()
	document := enclosureFixture()
	for _, limit := range []int{1, 2, 4} {
		plan, err := Plan(document, Options{NoLightmap: true, EnclosureAware: true, MaxSectors: limit})
		if err != nil || !slices.Equal(plan.Enclosures, []int{0, 1, 0, 1}) {
			t.Fatal("chunk budget changed enclosure identity", limit, plan.Enclosures, err)
		}
		for _, chunk := range plan.Chunks {
			if len(chunk.Sectors) > limit {
				t.Fatal("enclosure bypassed sector budget")
			}
		}
	}
	options := Options{EnclosureAware: true, CompactLightmaps: true, MaxSectors: 2,
		Lightmap: worldlightmap.Options{PageSize: 512, TexelsPerUnit: 2}}
	plan, err := Plan(document, options)
	if err != nil || len(plan.Chunks) != 2 || plan.SectorChunks[0] != plan.SectorChunks[2] ||
		plan.SectorChunks[1] != plan.SectorChunks[3] {
		t.Fatal("lightmap packing split complete enclosures", err)
	}
	for _, chunk := range plan.Chunks {
		if chunk.Layout.TexelsPerUnit != 2 || chunk.MeshBytes != DecodedMeshBytes(chunk.Mesh) {
			t.Fatal("density or resource accounting changed")
		}
	}
}

func TestEnclosureInferenceRespectsPhysicalAndAuthoredBoundaries(t *testing.T) {
	t.Parallel()
	for name, edit := range map[string]func(*world.Document, *Options){
		"broad opening": func(d *world.Document, _ *Options) { d.Sectors[2].SourceRoom = "extension" },
		"ceiling break": func(d *world.Document, _ *Options) { d.Sectors[2].Ceiling.C = 3 },
		"open sky": func(d *world.Document, _ *Options) {
			d.Sectors[2].Sky = true
			d.Environment = &world.Environment{Version: world.EnvironmentVersion, Preset: world.EnvironmentDaylight}
		},
		"explicit area": func(_ *world.Document, o *Options) { o.SectorGroups = []int{0, 0, 1, 0} },
		"transformed portal": func(d *world.Document, _ *Options) {
			for index := range d.Sectors[2].Walls {
				d.Sectors[2].Walls[index].Start.X += 100
				d.Sectors[2].Walls[index].End.X += 100
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			document := enclosureFixture()
			options := Options{NoLightmap: true, EnclosureAware: true}
			edit(&document, &options)
			plan, err := Plan(document, options)
			if err != nil {
				t.Fatal(err)
			}
			joined := plan.Enclosures[0] == plan.Enclosures[2]
			if joined != (name == "broad opening") || plan.Enclosures[0] == plan.Enclosures[1] {
				t.Fatal("incorrect enclosure boundary", plan.Enclosures)
			}
		})
	}
}

func TestEnclosurePlanningCannotOverrideMeshOrEncodedBudgets(t *testing.T) {
	t.Parallel()
	document := enclosureFixture()
	individual, err := Plan(document, Options{NoLightmap: true, MaxSectors: 1})
	if err != nil {
		t.Fatal(err)
	}
	var meshLimit, encodedLimit uint64
	for _, chunk := range individual.Chunks {
		encoded, encodeErr := EncodeMesh(chunk.Mesh, &document)
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		meshLimit, encodedLimit = max(meshLimit, chunk.MeshBytes), max(encodedLimit, uint64(len(encoded)))
	}
	for _, options := range []Options{
		{NoLightmap: true, EnclosureAware: true, MaxMeshBytes: meshLimit},
		{NoLightmap: true, EnclosureAware: true, MaxEncodedBytes: encodedLimit},
	} {
		plan, planErr := Plan(document, options)
		if planErr != nil || len(plan.Chunks) != 4 || !slices.Equal(plan.Enclosures, []int{0, 1, 0, 1}) {
			t.Fatal("hard budget or enclosure identity lost", plan.SectorChunks, planErr)
		}
	}
}
