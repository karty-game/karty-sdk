package worldchunk

import (
	"math"

	"github.com/karty-game/karty-sdk/format/world"
)

// Inference is deliberately conservative. Decomposition edges preserve authored
// identity; otherwise only broad, level, full-height ordinary openings merge.
// This is a grouping hint, not proof that the resulting space fits a cubemap.
func inferEnclosures(document *world.Document, groups []int) []int {
	ids := make([]int, len(document.Sectors))
	areas := make([]float64, len(ids))
	for index, sector := range document.Sectors {
		ids[index] = index
		for _, wall := range sector.Walls {
			areas[index] += wall.Start.X*wall.End.Y - wall.End.X*wall.Start.Y
		}
		areas[index] = math.Abs(areas[index]) * .5
	}
	for index, sector := range document.Sectors {
		for edge, wall := range sector.Walls {
			other := int(wall.Portal)
			if other <= index || !ordinary(document, index, edge) ||
				groups != nil && groups[index] != groups[other] {
				continue
			}
			neighbor := document.Sectors[other]
			if sector.Floor != neighbor.Floor || sector.Ceiling != neighbor.Ceiling || sector.Sky != neighbor.Sky {
				continue
			}
			internal := sector.SourceRoom != "" && sector.SourceRoom == neighbor.SourceRoom &&
				sector.Instance == neighbor.Instance && wall.SourceEdge == "" && reciprocalSourceEdge(document, index, edge) == ""
			if !internal && (sector.Sky || !broadEnclosureOpening(wall, areas[index], areas[other])) {
				continue
			}
			from, to := max(ids[index], ids[other]), min(ids[index], ids[other])
			for candidate, id := range ids {
				if id == from {
					ids[candidate] = to
				}
			}
		}
	}
	return ids
}

func reciprocalSourceEdge(document *world.Document, sector, edge int) string {
	wall := document.Sectors[sector].Walls[edge]
	if wall.PortalWall > 0 {
		return document.Sectors[wall.Portal].Walls[wall.PortalWall-1].SourceEdge
	}
	for _, candidate := range document.Sectors[wall.Portal].Walls {
		if candidate.Portal == int32(sector) && coincident(wall, candidate) {
			return candidate.SourceEdge
		}
	}
	return ""
}

func broadEnclosureOpening(wall world.Wall, leftArea, rightArea float64) bool {
	// A shared opening must span at least 3/4 of the smaller cell's
	// characteristic width. Tiny doors between large cells remain boundaries.
	const widthFraction = .75
	dx, dy := wall.End.X-wall.Start.X, wall.End.Y-wall.Start.Y
	return dx*dx+dy*dy >= widthFraction*widthFraction*min(leftArea, rightArea)
}
