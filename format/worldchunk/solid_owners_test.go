package worldchunk

import (
	"slices"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
)

func TestSolidOwnersMatchesSingleSectorPlanWithoutChangingWorld(t *testing.T) {
	t.Parallel()
	document := fixture(t)
	for index := range document.Sectors {
		document.Sectors[index].Floor = world.Plane{}
		document.Sectors[index].Ceiling = world.Plane{C: 4}
	}
	document.StaticSolids = &world.StaticSolids{Version: 1, Items: []world.Solid{
		{
			ID:             "both-rooms",
			Footprint:      []world.Vec2{{X: 3, Y: 1}, {X: 5, Y: 1}, {X: 5, Y: 2}, {X: 3, Y: 2}},
			Bottom:         world.Plane{C: .5},
			Top:            world.Plane{C: 1},
			SideMaterial:   1,
			TopMaterial:    1,
			BottomMaterial: 1,
		},
		{
			ID:             "second-room",
			Footprint:      []world.Vec2{{X: 5, Y: 1}, {X: 6, Y: 1}, {X: 6, Y: 2}, {X: 5, Y: 2}},
			Bottom:         world.Plane{C: .5},
			Top:            world.Plane{C: 1},
			SideMaterial:   1,
			TopMaterial:    1,
			BottomMaterial: 1,
		},
	}}
	before, err := world.Encode(document)
	if err != nil {
		t.Fatal(err)
	}
	owners, err := SolidOwners(&document)
	if err != nil || !slices.Equal(owners, []int{0, 1}) {
		t.Fatal("first-overlap ownership changed", owners, err)
	}
	plan, err := Plan(document, Options{NoLightmap: true, MaxSectors: 1})
	if err != nil {
		t.Fatal(err)
	}
	seen := make([]bool, len(owners))
	for _, chunk := range plan.Chunks {
		for _, solid := range chunk.Solids {
			if len(chunk.Sectors) != 1 || chunk.Sectors[0] != owners[solid] || seen[solid] {
				t.Fatal("standalone ownership differs from planner", owners, chunk.Sectors, solid)
			}
			seen[solid] = true
		}
	}
	if slices.Contains(seen, false) {
		t.Fatal("planner lost a solid")
	}
	after, err := world.Encode(document)
	if err != nil || !slices.Equal(before, after) {
		t.Fatal("owner query changed compiled world", err)
	}
}

func TestSolidOwnersPreservesWorldAdmissionAndEmptyResults(t *testing.T) {
	t.Parallel()
	if _, err := SolidOwners(nil); err == nil {
		t.Fatal("nil document accepted")
	}
	document := fixture(t)
	document.StaticSolids = nil
	if owners, err := SolidOwners(&document); err != nil || owners != nil {
		t.Fatal("empty solid set changed", owners, err)
	}
	document.Version = 0
	if _, err := SolidOwners(&document); err == nil {
		t.Fatal("invalid compiled world accepted")
	}
}
