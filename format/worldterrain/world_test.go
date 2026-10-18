package worldterrain

import (
	"errors"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
)

func terrainSector(id string, x0, y0, x1, y1 float64) world.Sector {
	return world.Sector{ID: id, SourceRoom: id, Sky: true, Ceiling: world.Plane{C: 10}, Walls: []world.Wall{
		{Start: world.Vec2{X: x0, Y: y0}, End: world.Vec2{X: x1, Y: y0}, Portal: -1},
		{Start: world.Vec2{X: x1, Y: y0}, End: world.Vec2{X: x1, Y: y1}, Portal: -1},
		{Start: world.Vec2{X: x1, Y: y1}, End: world.Vec2{X: x0, Y: y1}, Portal: -1},
		{Start: world.Vec2{X: x0, Y: y1}, End: world.Vec2{X: x0, Y: y0}, Portal: -1},
	}}
}
func terrainWorld() world.Document {
	return world.Document{
		Version:     world.Version,
		Environment: &world.Environment{Version: world.EnvironmentVersion, Preset: world.EnvironmentDaylight},
		Sectors:     []world.Sector{terrainSector("field", 0, 0, 2, 2)},
	}
}

func TestWorldOwnershipChecksCornersCeilingsAndInteriorCells(t *testing.T) {
	for _, name := range []string{"complete", "unowned corner", "above ceiling", "thin nonaligned house", "unmasked house", "masked house"} {
		t.Run(name, func(t *testing.T) {
			d, w := fixture(), terrainWorld()
			switch name {
			case "unowned corner":
				w.Sectors[0] = terrainSector("field", 0, 0, 1.9, 2)
			case "above ceiling":
				w.Sectors[0].Ceiling.C = 3.9
			case "thin nonaligned house":
				house := terrainSector("house", .4, .4, .6, .6)
				house.Sky = false
				w.Sectors = append(w.Sectors, house)
			case "unmasked house", "masked house":
				house := terrainSector("house", 1, 1, 2, 2)
				house.Sky = false
				w.Sectors = append(w.Sectors, house)
				if name == "masked house" {
					d.Holes[3] = 1
				}
			}
			if err := world.Validate(&w); err != nil {
				t.Fatal("test must use a valid sector world", err)
			}
			err := ValidateForWorld(&d, &w)
			if name == "complete" || name == "masked house" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("unsupported terrain association accepted")
			}
		})
	}
}

func TestWorldOwnershipRequiresOrdinaryConnectedOwners(t *testing.T) {
	for _, name := range []string{"ordinary", "directed", "transformed", "disconnected", "shared low ceiling"} {
		t.Run(name, func(t *testing.T) {
			d, w := fixture(), terrainWorld()
			d.ExteriorSectors = []uint16{0, 1}
			w.Sectors = []world.Sector{terrainSector("west", 0, 0, 1, 2), terrainSector("east", 1, 0, 2, 2)}
			w.Sectors[0].Walls[1].Portal = 1
			w.Sectors[0].Walls[1].PortalWall = 4
			w.Sectors[1].Walls[3].Portal = 0
			w.Sectors[1].Walls[3].PortalWall = 2
			switch name {
			case "directed":
				w.Sectors[1].Walls[3].Portal = -1
				w.Sectors[1].Walls[3].PortalWall = 0
			case "transformed":
				w.Sectors[1] = terrainSector("east", 3, 0, 4, 2)
				w.Sectors[1].Walls[3].Portal = 0
				w.Sectors[1].Walls[3].PortalWall = 2
			case "disconnected":
				w.Sectors[0].Walls[1].Portal = -1
				w.Sectors[0].Walls[1].PortalWall = 0
				w.Sectors[1].Walls[3].Portal = -1
				w.Sectors[1].Walls[3].PortalWall = 0
			case "shared low ceiling":
				w.Sectors[1].Ceiling.C = 3.9
			}
			if err := world.Validate(&w); err != nil {
				t.Fatal("test must use a valid sector world", err)
			}
			err := ValidateForWorld(&d, &w)
			if name == "ordinary" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("unsupported terrain topology accepted")
			}
		})
	}
}

func TestTerrainTriangleBudgetCountsOnlyPresentCells(t *testing.T) {
	d := fixture()
	d.Columns, d.Rows = 257, 257
	nv := int(d.Columns) * int(d.Rows)
	d.Heights = make([]float32, nv)
	d.Normals = make([][3]float32, nv)
	for i := range d.Normals {
		d.Normals[i] = [3]float32{0, 0, 1}
	}
	d.Holes = make([]byte, 256*256)
	d.Patches[0].Columns, d.Patches[0].Rows = 256, 256
	if err := Validate(&d); !errors.Is(err, ErrBounds) {
		t.Fatal("terrain triangle budget did not reject full 257-square field", err)
	}
	for i := MaxTriangles / 2; i < len(d.Holes); i++ {
		d.Holes[i] = 1
	}
	if err := Validate(&d); err != nil {
		t.Fatal("holes should permit the largest lattice within triangle budget", err)
	}
}
