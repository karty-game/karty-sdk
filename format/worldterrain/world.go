package worldterrain

import (
	"math"

	"github.com/karty-game/karty-sdk/format/world"
)

const worldTolerance = 1e-5
const maxOwnershipEdgeChecks = 16 * 1024 * 1024

type sectorBounds struct{ minX, minY, maxX, maxY float64 }

func boundsOf(s *world.Sector) sectorBounds {
	b := sectorBounds{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
	for _, e := range s.Walls {
		b.minX = min(b.minX, e.Start.X)
		b.minY = min(b.minY, e.Start.Y)
		b.maxX = max(b.maxX, e.Start.X)
		b.maxY = max(b.maxY, e.Start.Y)
	}
	return b
}
func (b sectorBounds) contains(x, y float64) bool {
	return x >= b.minX-worldTolerance && x <= b.maxX+worldTolerance && y >= b.minY-worldTolerance && y <= b.maxY+worldTolerance
}

// ValidateForWorld checks a terrain association with an already validated world.
// Active corners and cell centres must have outdoor ownership below its finite
// ceilings. Interior footprints are grid-aligned rectangular exclusions in v1.
// All portals use reciprocal coincident edges; transformed/directed spaces fail.
func ValidateForWorld(d *Document, w *world.Document) error {
	if err := Validate(d); err != nil {
		return err
	}
	if w == nil || len(w.Sectors) == 0 || len(w.Sectors) > world.MaxSectors {
		return ErrTerrain
	}
	selected := make([]bool, len(w.Sectors))
	bounds := make([]sectorBounds, len(w.Sectors))
	for i := range w.Sectors {
		if len(w.Sectors[i].Walls) < 3 || len(w.Sectors[i].Walls) > world.MaxWallsPerSector {
			return ErrTerrain
		}
		bounds[i] = boundsOf(&w.Sectors[i])
	}
	for _, id := range d.ExteriorSectors {
		if int(id) >= len(w.Sectors) || !w.Sectors[id].Sky {
			return ErrTerrain
		}
		selected[id] = true
	}
	if err := ordinaryTopology(d, w); err != nil {
		return err
	}
	if err := validateInteriorHoles(d, w, selected, bounds); err != nil {
		return err
	}
	// Shared vertices are checked once, including all adjacent closed owners.
	active := make([]bool, len(d.Heights))
	cols := int(d.Columns)
	edgeChecks := 0
	for y := 0; y < int(d.Rows)-1; y++ {
		for x := 0; x < cols-1; x++ {
			if d.Holes[y*(cols-1)+x] != 0 {
				continue
			}
			a := y*cols + x
			for _, i := range []int{a, a + 1, a + cols, a + cols + 1} {
				active[i] = true
			}
			wx := (float64(d.Origin[0]+float32(x)*d.CellSize) + float64(d.Origin[0]+float32(x+1)*d.CellSize)) / 2
			wy := (float64(d.Origin[1]+float32(y)*d.CellSize) + float64(d.Origin[1]+float32(y+1)*d.CellSize)) / 2
			z := (float64(d.Heights[a+1]) + float64(d.Heights[a+cols])) * .5
			if err := pointOwned(d, w, bounds, wx, wy, z, &edgeChecks); err != nil {
				return err
			}
		}
	}
	for i, needed := range active {
		if !needed {
			continue
		}
		x := float64(d.Origin[0] + float32(i%cols)*d.CellSize)
		y := float64(d.Origin[1] + float32(i/cols)*d.CellSize)
		if err := pointOwned(d, w, bounds, x, y, float64(d.Heights[i]), &edgeChecks); err != nil {
			return err
		}
	}
	return nil
}
func pointOwned(d *Document, w *world.Document, bounds []sectorBounds, x, y, z float64, checks *int) error {
	found := false
	for _, id := range d.ExteriorSectors {
		if !bounds[id].contains(x, y) {
			continue
		}
		s := &w.Sectors[id]
		inside := true
		for _, e := range s.Walls {
			*checks++
			if *checks > maxOwnershipEdgeChecks {
				return ErrBounds
			}
			if (e.End.X-e.Start.X)*(y-e.Start.Y)-(e.End.Y-e.Start.Y)*(x-e.Start.X) < -worldTolerance {
				inside = false
				break
			}
		}
		if !inside {
			continue
		}
		ceiling := s.Ceiling.A*x + s.Ceiling.B*y + s.Ceiling.C
		if math.IsNaN(ceiling) || math.IsInf(ceiling, 0) || z >= ceiling-worldTolerance {
			return ErrTerrain
		}
		found = true
	}
	if !found {
		return ErrTerrain
	}
	return nil
}
func ordinaryTopology(d *Document, w *world.Document) error {
	for i, s := range w.Sectors {
		for _, e := range s.Walls {
			if e.Portal < 0 {
				continue
			}
			if int(e.Portal) >= len(w.Sectors) {
				return ErrTerrain
			}
			neighbor := &w.Sectors[e.Portal]
			reciprocal := false
			for j, other := range neighbor.Walls {
				if e.PortalWall != 0 && int(e.PortalWall) != j+1 {
					continue
				}
				if other.Portal == int32(i) && other.Start == e.End && other.End == e.Start {
					reciprocal = true
					break
				}
			}
			if !reciprocal {
				return ErrTerrain
			}
		}
	}
	seen := make([]bool, len(w.Sectors))
	queue := make([]int, 1, len(w.Sectors))
	queue[0] = int(d.ExteriorSectors[0])
	seen[queue[0]] = true
	for at := 0; at < len(queue); at++ {
		for _, e := range w.Sectors[queue[at]].Walls {
			if e.Portal >= 0 && !seen[e.Portal] {
				seen[e.Portal] = true
				queue = append(queue, int(e.Portal))
			}
		}
	}
	for _, id := range d.ExteriorSectors {
		if !seen[id] {
			return ErrTerrain
		}
	}
	return nil
}
func validateInteriorHoles(d *Document, w *world.Document, selected []bool, bounds []sectorBounds) error {
	minX, minY := float64(d.Origin[0]), float64(d.Origin[1])
	step := float64(d.CellSize)
	maxX, maxY := minX+step*float64(d.Columns-1), minY+step*float64(d.Rows-1)
	for i, s := range w.Sectors {
		b := bounds[i]
		if selected[i] || b.maxX <= minX+worldTolerance || b.minX >= maxX-worldTolerance || b.maxY <= minY+worldTolerance ||
			b.minY >= maxY-worldTolerance {
			continue
		}
		// Convex sectors with only axis-aligned edges are rectangles. Their grid
		// boundaries make whole-cell render/collision exclusions unambiguous.
		for _, e := range s.Walls {
			if math.Abs(e.Start.X-e.End.X) > worldTolerance && math.Abs(e.Start.Y-e.End.Y) > worldTolerance {
				return ErrTerrain
			}
			gx, gy := (e.Start.X-minX)/step, (e.Start.Y-minY)/step
			if math.Abs(gx-math.Round(gx)) > worldTolerance || math.Abs(gy-math.Round(gy)) > worldTolerance {
				return ErrTerrain
			}
		}
		x0, x1 := max(0, int(math.Round((b.minX-minX)/step))), min(int(d.Columns)-1, int(math.Round((b.maxX-minX)/step)))
		y0, y1 := max(0, int(math.Round((b.minY-minY)/step))), min(int(d.Rows)-1, int(math.Round((b.maxY-minY)/step)))
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				if d.Holes[y*(int(d.Columns)-1)+x] == 0 {
					return ErrTerrain
				}
			}
		}
	}
	return nil
}
