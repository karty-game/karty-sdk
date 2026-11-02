package worldgpuchunk

import (
	"slices"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
)

func portalSpacesFixture() world.Document {
	sectors := make([]world.Sector, 3)
	for index := range sectors {
		x := float64(index)
		points := []world.Vec2{{X: x, Y: 0}, {X: x + 1, Y: 0}, {X: x + 1, Y: 1}, {X: x, Y: 1}}
		for edge, start := range points {
			sectors[index].Walls = append(sectors[index].Walls, world.Wall{Start: start, End: points[(edge+1)%len(points)], Portal: -1})
		}
	}
	for index := range 2 {
		sectors[index].Walls[1].Portal = int32(index + 1)
		sectors[index].Walls[1].PortalWall = 4
		sectors[index+1].Walls[3].Portal = int32(index)
		sectors[index+1].Walls[3].PortalWall = 2
	}
	return world.Document{Version: world.Version, Sectors: sectors}
}

func TestPortalSpacesPreserveTopologyAcrossOverlappingCoordinates(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		change func(*world.Document)
		want   []uint32
	}{
		{"ordinary", func(*world.Document) {}, []uint32{0, 0, 0}},
		{"translated", func(d *world.Document) {
			for i := range d.Sectors[2].Walls {
				d.Sectors[2].Walls[i].Start.X -= 2
				d.Sectors[2].Walls[i].End.X -= 2
			}
		}, []uint32{0, 0, 2}},
		{"directed", func(d *world.Document) { d.Sectors[2].Walls[3].Portal = -1; d.Sectors[2].Walls[3].PortalWall = 0 }, []uint32{0, 0, 2}},
		{"unlinked-overlap", func(d *world.Document) {
			d.Sectors[1].Walls[1].Portal = -1
			d.Sectors[2].Walls[3].Portal = -1
			for i := range d.Sectors[2].Walls {
				d.Sectors[2].Walls[i].Start.X -= 2
				d.Sectors[2].Walls[i].End.X -= 2
			}
		}, []uint32{0, 0, 2}},
		{"wrong-reciprocal-edge", func(d *world.Document) { d.Sectors[2].Walls[3].PortalWall = 1 }, []uint32{0, 0, 2}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			document := portalSpacesFixture()
			test.change(&document)
			got, err := PortalSpaces(&document)
			if err != nil || !slices.Equal(got, test.want) {
				t.Fatal(got, err)
			}
		})
	}
}

func TestPortalSpacesRejectOutOfBoundsLinks(t *testing.T) {
	t.Parallel()
	for _, test := range []func(*world.Document){func(d *world.Document) { d.Sectors = nil }, func(d *world.Document) { d.Sectors[0].Walls[1].Portal = 99 }, func(d *world.Document) { d.Sectors[0].Walls[1].PortalWall = 99 }} {
		document := portalSpacesFixture()
		test(&document)
		if _, err := PortalSpaces(&document); err == nil {
			t.Fatal("invalid portal references accepted")
		}
	}
	if _, err := PortalSpaces(nil); err == nil {
		t.Fatal("nil world accepted")
	}
}
