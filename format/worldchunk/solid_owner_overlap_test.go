package worldchunk

import (
	"slices"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
)

func TestSolidOwnerPrefersPositiveRoomAreaOverEarlierBoundaryTouch(t *testing.T) {
	t.Parallel()
	document := fixture(t)
	for index := range document.Sectors {
		document.Sectors[index].Floor = world.Plane{}
		document.Sectors[index].Ceiling = world.Plane{C: 4}
	}
	document.StaticSolids = &world.StaticSolids{Version: 1, Items: []world.Solid{
		solidOwnershipBox("touch-first", 4, 1, 5, 2),
		solidOwnershipBox("positive-both", 3.5, 1, 4.5, 2),
		solidOwnershipBox("exterior-touch-only", -1, 1, 0, 2),
	}}
	owners, err := SolidOwners(&document)
	if err != nil || !slices.Equal(owners, []int{1, 0, 0}) {
		t.Fatal("touch-only receiver displaced positive room area or exterior ownership changed", owners, err)
	}
	plan, err := Plan(document, Options{NoLightmap: true, MaxSectors: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, chunk := range plan.Chunks {
		for _, solid := range chunk.Solids {
			if chunk.Sectors[0] != owners[solid] {
				t.Fatal("planner did not use positive-area ownership", chunk.Sectors, solid, owners)
			}
		}
	}
	supplied := []int{0, 0, 0}
	override, err := Plan(document, Options{NoLightmap: true, MaxSectors: 1, SolidOwners: supplied})
	if err != nil || !slices.Equal(override.Chunks[0].Solids, []int{0, 1, 2}) {
		t.Fatal("explicit previously valid owner override changed", err)
	}
}

func TestSolidFootprintRequiresPolygonAreaRatherThanBoundingBox(t *testing.T) {
	t.Parallel()
	sector := world.Sector{Walls: []world.Wall{
		{Start: world.Vec2{}, End: world.Vec2{X: 4}},
		{Start: world.Vec2{X: 4}, End: world.Vec2{Y: 4}},
		{Start: world.Vec2{Y: 4}, End: world.Vec2{}},
	}}
	if solidFootprintOverlap(solidOwnershipBox("bbox-only", 3, 3, 3.5, 3.5), sector) {
		t.Fatal("AABB overlap outside a triangular room counted as positive area")
	}
	if solidFootprintOverlap(solidOwnershipBox("edge-only", 2, 2, 3, 3), sector) {
		t.Fatal("diagonal boundary contact counted as positive area")
	}
	if !solidFootprintOverlap(solidOwnershipBox("positive", 1, 1, 2, 2), sector) {
		t.Fatal("positive polygon intersection lost")
	}
}

func solidOwnershipBox(id string, x0, y0, x1, y1 float64) world.Solid {
	return world.Solid{ID: id, Footprint: []world.Vec2{{X: x0, Y: y0}, {X: x1, Y: y0}, {X: x1, Y: y1}, {X: x0, Y: y1}},
		Bottom: world.Plane{C: .5}, Top: world.Plane{C: 1}, SideMaterial: 1, TopMaterial: 1, BottomMaterial: 1}
}
