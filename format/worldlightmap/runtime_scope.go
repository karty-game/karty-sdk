package worldlightmap

import (
	"fmt"
	"math"

	"github.com/karty-game/karty-sdk/format/world"
)

// Runtime recipe version 1 covers one ordinary connected physical space. A
// transformed or one-way portal never combines disconnected bake components.
func validateRuntimeScope(document *world.Document) error {
	visited := make([]bool, len(document.Sectors))
	visited[0] = true
	queue := []int{0}
	for next := 0; next < len(queue); next++ {
		sector := queue[next]
		for edge, wall := range document.Sectors[sector].Walls {
			if !ordinaryPortal(document, sector, edge, wall) || visited[wall.Portal] {
				continue
			}
			visited[wall.Portal] = true
			queue = append(queue, int(wall.Portal))
		}
	}
	if len(queue) != len(document.Sectors) {
		return fmt.Errorf("runtime baking requires one ordinary physical component: %w", ErrLayout)
	}
	if document.StaticSolids == nil {
		return nil
	}
	rooms := make([][4]float64, len(document.Sectors))
	for index, sector := range document.Sectors {
		points := make([]world.Vec2, len(sector.Walls))
		for edge, wall := range sector.Walls {
			points[edge] = wall.Start
		}
		rooms[index] = xyBounds(points)
	}
	for _, solid := range document.StaticSolids.Items {
		bounds := xyBounds(solid.Footprint)
		associated := false
		for _, room := range rooms {
			// This conservative association is intentionally based on XY bounds;
			// exact mesh clipping and movement tests remain the host's concern.
			if bounds[0] <= room[2]+1e-9 && bounds[2] >= room[0]-1e-9 && bounds[1] <= room[3]+1e-9 && bounds[3] >= room[1]-1e-9 {
				associated = true
				break
			}
		}
		if !associated {
			return fmt.Errorf("solid %q has no runtime receiver component: %w", solid.ID, ErrLayout)
		}
	}
	return nil
}

func xyBounds(points []world.Vec2) [4]float64 {
	bounds := [4]float64{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
	for _, point := range points {
		bounds[0] = math.Min(bounds[0], point.X)
		bounds[1] = math.Min(bounds[1], point.Y)
		bounds[2] = math.Max(bounds[2], point.X)
		bounds[3] = math.Max(bounds[3], point.Y)
	}
	return bounds
}

func runtimeEmitterInside(document *world.Document, position world.Vec3) bool {
	for _, sector := range document.Sectors {
		xy := world.Vec2{X: position.X, Y: position.Y}
		if position.Z < elevation(sector.Floor, xy) || position.Z > elevation(sector.Ceiling, xy) {
			continue
		}
		inside := true
		for _, wall := range sector.Walls {
			if (wall.End.X-wall.Start.X)*(position.Y-wall.Start.Y)-(wall.End.Y-wall.Start.Y)*(position.X-wall.Start.X) < -1e-8 {
				inside = false
				break
			}
		}
		if inside {
			return true
		}
	}
	return false
}
