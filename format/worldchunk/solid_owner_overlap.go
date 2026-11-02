package worldchunk

import "github.com/karty-game/karty-sdk/format/world"

// Ordinary room ownership prefers real footprint area over boundary contact.
// The existing conservative fallback retains exterior trims that only touch a
// room. Explicit owner overrides keep their separately validated old policy.
func solidReceiverOwner(solid world.Solid, sectors []world.Sector) int {
	fallback := -1
	for index, sector := range sectors {
		if !overlaps(solid, sector) {
			continue
		}
		if fallback < 0 {
			fallback = index
		}
		if solidFootprintOverlap(solid, sector) {
			return index
		}
	}
	return fallback
}

func solidFootprintOverlap(solid world.Solid, sector world.Sector) bool {
	polygon := make([]world.Vec3, len(solid.Footprint))
	for index, point := range solid.Footprint {
		polygon[index] = world.Vec3{X: point.X, Y: point.Y}
	}
	for _, wall := range sector.Walls {
		plane := cleanupPlane{
			normal:   world.Vec3{X: wall.End.Y - wall.Start.Y, Y: wall.Start.X - wall.End.X},
			anchor:   world.Vec3{X: wall.Start.X, Y: wall.Start.Y},
			anchored: true,
		}
		_, polygon = cleanupSplit(polygon, plane)
		if len(polygon) < 3 {
			return false
		}
	}
	return cleanupPolygonArea(polygon, world.Vec3{Z: 1}) > 0
}
