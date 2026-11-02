package worldgpuchunk

import (
	"math"

	"github.com/karty-game/karty-sdk/format/world"
)

// PortalSpaces labels admitted sectors by their lowest connected sector index.
// Only reciprocal portals with an identity coordinate transform join spaces.
// Directed, transformed and absent links never imply coordinate adjacency.
// This is bounded topology inspection, not visibility or prefetch selection.
func PortalSpaces(document *world.Document) ([]uint32, error) {
	if document == nil || len(document.Sectors) == 0 || len(document.Sectors) > world.MaxSectors {
		return nil, ErrChunk
	}
	parents := make([]uint32, len(document.Sectors))
	for index := range parents {
		parents[index] = uint32(index)
	}
	root := func(index uint32) uint32 {
		for parents[index] != index {
			parents[index] = parents[parents[index]]
			index = parents[index]
		}
		return index
	}
	for sector, source := range document.Sectors {
		for edge, wall := range source.Walls {
			if wall.Portal < 0 {
				continue
			}
			if int(wall.Portal) >= len(parents) {
				return nil, ErrChunk
			}
			targets := document.Sectors[wall.Portal].Walls
			if int(wall.PortalWall) > len(targets) {
				return nil, ErrChunk
			}
			for targetEdge, target := range targets {
				if wall.PortalWall != 0 && targetEdge != int(wall.PortalWall)-1 {
					continue
				}
				if target.Portal != int32(sector) || document.Version >= world.Version && int(target.PortalWall) != edge+1 ||
					!identityPortalEdges(wall, target) {
					continue
				}
				left, right := root(uint32(sector)), root(uint32(wall.Portal))
				parents[max(left, right)] = min(left, right)
				break
			}
		}
	}
	for index := range parents {
		parents[index] = root(uint32(index))
	}
	return parents, nil
}

func identityPortalEdges(source, target world.Wall) bool {
	const epsilon = 1e-9
	return math.Hypot(source.Start.X-target.End.X, source.Start.Y-target.End.Y) <= epsilon &&
		math.Hypot(source.End.X-target.Start.X, source.End.Y-target.Start.Y) <= epsilon
}
