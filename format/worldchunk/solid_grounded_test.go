package worldchunk

import (
	"bytes"
	"fmt"
	"slices"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

func narrowGroundedFixtureSolid() world.Solid {
	return world.Solid{ID: "arbitrary-detail", Footprint: []world.Vec2{{X: 3, Y: 1}, {X: 5, Y: 1}, {X: 5, Y: 1.4}, {X: 3, Y: 1.4}},
		Bottom: world.Plane{C: -.08}, Top: world.Plane{C: .8}, SideMaterial: 1, TopMaterial: 1, BottomMaterial: 1}
}

func TestNarrowGroundedEligibilityUsesGeometryAndReceiverFloor(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		change func(*world.Solid, *world.Sector)
		want   bool
	}{
		{"grounded", func(_ *world.Solid, _ *world.Sector) {}, true},
		{"renamed", func(s *world.Solid, _ *world.Sector) { s.ID = "not-a-wall"; s.SideMaterial = 7 }, true},
		{"raised-floor", func(s *world.Solid, r *world.Sector) { s.Bottom.C += 10; s.Top.C += 10; r.Floor.C += 10 }, true},
		{"sloping-floor", func(s *world.Solid, r *world.Sector) { s.Bottom.A = .1; s.Top.A = .1; r.Floor.A = .1 }, true},
		{"floating", func(s *world.Solid, _ *world.Sector) { s.Bottom.C = .1 }, false},
		{"upper-storey", func(s *world.Solid, _ *world.Sector) { s.Bottom.C += 4; s.Top.C += 4 }, false},
		{"tall", func(s *world.Solid, _ *world.Sector) { s.Top.C = 1.5 }, false},
		{"deep", func(s *world.Solid, _ *world.Sector) { s.Bottom.C = -1 }, false},
		{"wide", func(s *world.Solid, _ *world.Sector) { s.Footprint[2].Y = 2; s.Footprint[3].Y = 2 }, false},
		{"short", func(s *world.Solid, _ *world.Sector) { s.Footprint[1].X = 4; s.Footprint[2].X = 4 }, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			solid, room := narrowGroundedFixtureSolid(), world.Sector{}
			test.change(&solid, &room)
			if narrowGroundedSolid(solid, room) != test.want {
				t.Fatal("grounded owner eligibility ignored physical shape or floor relationship")
			}
		})
	}
}

func TestGroundedOwnersAreDeterministicNameIndependentAndComplete(t *testing.T) {
	t.Parallel()
	document := fixture(t)
	for i := range document.Sectors {
		document.Sectors[i].Floor, document.Sectors[i].Ceiling = world.Plane{}, world.Plane{C: 4}
	}
	document.StaticSolids = &world.StaticSolids{Version: 1}
	for i := range 3 {
		solid := narrowGroundedFixtureSolid()
		solid.ID = fmt.Sprintf("detail-%d", i)
		document.StaticSolids.Items = append(document.StaticSolids.Items, solid)
	}
	options := worldlightmap.Options{PageSize: 512, TexelsPerUnit: 24, Padding: 2, CapsWithinSourceRoom: true, CompactPacking: true}
	before, err := world.Encode(document)
	if err != nil {
		t.Fatal(err)
	}
	owners, err := BalancedSolidOwners(document, options)
	if err != nil || !slices.Equal(owners, []int{0, 0, 0}) {
		t.Fatalf("low details were not concentrated into the first fitting owner: %v %v", owners, err)
	}
	after, err := world.Encode(document)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("owner policy changed source geometry")
	}
	for i := range document.StaticSolids.Items {
		document.StaticSolids.Items[i].ID = fmt.Sprintf("renamed-%d", i)
	}
	renamed, err := BalancedSolidOwners(document, options)
	if err != nil || !slices.Equal(owners, renamed) {
		t.Fatal("owner policy depends on solid identifiers or is nondeterministic", err)
	}
	plan, err := Plan(document, Options{Lightmap: options, SolidOwners: owners, CompactLightmaps: true})
	if err != nil {
		t.Fatal(err)
	}
	seen := make([]bool, len(owners))
	for _, chunk := range plan.Chunks {
		if chunk.Layout.TexelsPerUnit != 24 || chunk.Layout.Padding != 2 || len(chunk.Layout.Pages) != 1 ||
			chunk.Layout.Pages[0].Width != 512 {
			t.Fatal("grounded owner policy changed density or page limits")
		}
		for _, solid := range chunk.Solids {
			if seen[solid] {
				t.Fatal("grounded owner policy duplicated a solid")
			}
			seen[solid] = true
		}
	}
	if slices.Contains(seen, false) {
		t.Fatal("grounded owner policy omitted a solid")
	}
}

func TestGroundedOwnersCannotJoinDisconnectedOrTransformedSpaces(t *testing.T) {
	t.Parallel()
	for _, transformed := range []bool{false, true} {
		t.Run(fmt.Sprintf("transformed-%t", transformed), func(t *testing.T) {
			t.Parallel()
			document := fixture(t)
			document.StaticSolids = &world.StaticSolids{Version: 1, Items: []world.Solid{narrowGroundedFixtureSolid()}}
			if transformed {
				document.Sectors[0].Walls[1].PortalWall = 2
			} else {
				document.Sectors[0].Walls[1].Portal, document.Sectors[0].Walls[1].PortalWall = -1, 0
				document.Sectors[1].Walls[3].Portal, document.Sectors[1].Walls[3].PortalWall = -1, 0
			}
			if _, err := BalancedSolidOwners(document, worldlightmap.Options{PageSize: 512, TexelsPerUnit: 24}); err == nil {
				t.Fatal("grounded solid spanning distinct physical spaces was accepted")
			}
		})
	}
}

func TestGroundedConcentrationRetainsValidOwnersWhenNarrowPageIsFull(t *testing.T) {
	t.Parallel()
	document := fixture(t)
	document.Environment = &world.Environment{Version: world.EnvironmentVersion, Preset: world.EnvironmentDaylight}
	document.Contents = nil
	for i := range document.Sectors {
		document.Sectors[i].Floor, document.Sectors[i].Ceiling = world.Plane{}, world.Plane{C: 4}
		document.Sectors[i].Sky = true
		for edge := range document.Sectors[i].Walls {
			document.Sectors[i].Walls[edge].Sky = document.Sectors[i].Walls[edge].Portal < 0
			document.Sectors[i].Walls[edge].Start.X *= 1.6
			document.Sectors[i].Walls[edge].Start.Y *= 1.6
			document.Sectors[i].Walls[edge].End.X *= 1.6
			document.Sectors[i].Walls[edge].End.Y *= 1.6
		}
	}
	first, second := narrowGroundedFixtureSolid(), narrowGroundedFixtureSolid()
	second.ID = "restricted-detail"
	for i := range second.Footprint {
		second.Footprint[i].X -= 2.5
	}
	for _, solid := range []*world.Solid{&first, &second} {
		for i := range solid.Footprint {
			solid.Footprint[i].X *= 1.6
		}
		solid.Bottom.C, solid.Top.C = -.25, 1
	}
	document.StaticSolids = &world.StaticSolids{Version: 1, Items: []world.Solid{first, second}}
	options := worldlightmap.Options{PageSize: 512, TexelsPerUnit: 64, Padding: 2, CompactPacking: true}
	original := []int{1, 0}
	if _, err := Plan(document, Options{Lightmap: options, SolidOwners: original, MaxSectors: 1}); err != nil {
		t.Fatal("separately owned fixture must already fit at the retained density", err)
	}
	owners, err := groundedSolidOwners(document, options, original)
	if err != nil || !slices.Equal(owners, original) {
		t.Fatalf("infeasible concentration discarded a valid full-density plan: %v %v", owners, err)
	}
}
