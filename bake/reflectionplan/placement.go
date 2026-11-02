package reflectionplan

import (
	"math"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldreflection"
)

const (
	reflectionPlacementGrid        = 12
	reflectionPlacementRefinements = 8
	reflectionPlacementTolerance   = .001
)

type reflectionPlacementWall struct{ start, end world.Vec2 }

type reflectionPlacementSolid struct {
	// Outward unit planes give a conservative distance to a convex solid,
	// including sloped tops/bottoms and noncolliding decorative geometry.
	planes [][4]float64
}

type reflectionPlacement struct {
	document                  *world.Document
	members                   []int
	walls                     []reflectionPlacementWall
	solids                    []reflectionPlacementSolid
	center                    world.Vec2
	best                      [3]float32
	clearance, centerDistance float64
}

// Search open space at local midheight, rather than selecting the first point
// outside an obstacle. Internal decomposition edges are not physical walls.
// A bounded grid plus local refinement is deterministic and independent of
// render chunks. This optimizes geometric clearance, not reflection accuracy.
func placeReflectionCaptureOrigin(document *world.Document, members []int, center world.Vec2, probe worldreflection.Probe) [3]float32 {
	search := reflectionPlacement{document: document, members: members, center: center, best: probe.Origin, clearance: -1}
	search.prepare(probe)
	search.consider(center)

	for _, index := range members {
		cell := document.Sectors[index]
		xy := reflectionProbeCenter(cell)
		search.consider(xy)
		// These seeds preserve narrow cells that a room-wide grid can miss.
		for _, wall := range cell.Walls {
			search.consider(world.Vec2{X: (xy.X + wall.Start.X) * reflectionMidpoint, Y: (xy.Y + wall.Start.Y) * reflectionMidpoint})
		}
	}

	width, depth := float64(probe.High[0]-probe.Low[0]), float64(probe.High[1]-probe.Low[1])

	for y := range reflectionPlacementGrid {
		for x := range reflectionPlacementGrid {
			search.consider(world.Vec2{
				X: float64(probe.Low[0]) + (float64(x)+reflectionMidpoint)*width/reflectionPlacementGrid,
				Y: float64(probe.Low[1]) + (float64(y)+reflectionMidpoint)*depth/reflectionPlacementGrid,
			})
		}
	}

	stepX, stepY := width/reflectionPlacementGrid, depth/reflectionPlacementGrid

	for range reflectionPlacementRefinements {
		origin := search.best

		for y := -1; y <= 1; y++ {
			for x := -1; x <= 1; x++ {
				search.consider(world.Vec2{X: float64(origin[0]) + float64(x)*stepX, Y: float64(origin[1]) + float64(y)*stepY})
			}
		}

		stepX *= reflectionMidpoint
		stepY *= reflectionMidpoint
	}

	return search.best
}

func (search *reflectionPlacement) prepare(probe worldreflection.Probe) {
	var inside [world.MaxSectors]bool
	for _, index := range search.members {
		inside[index] = true
	}

	for _, index := range search.members {
		for _, wall := range search.document.Sectors[index].Walls {
			if wall.Portal < 0 || !inside[wall.Portal] {
				search.walls = append(search.walls, reflectionPlacementWall{wall.Start, wall.End})
			}
		}
	}

	if search.document.StaticSolids == nil {
		return
	}

	for _, solid := range search.document.StaticSolids.Items {
		low, high := world.Vec2{X: math.Inf(1), Y: math.Inf(1)}, world.Vec2{X: math.Inf(-1), Y: math.Inf(-1)}
		for _, point := range solid.Footprint {
			low.X, low.Y = min(low.X, point.X), min(low.Y, point.Y)
			high.X, high.Y = max(high.X, point.X), max(high.Y, point.Y)
		}

		if high.X < float64(probe.Low[0]) || low.X > float64(probe.High[0]) ||
			high.Y < float64(probe.Low[1]) || low.Y > float64(probe.High[1]) {
			continue
		}

		obstacle := reflectionPlacementSolid{planes: make([][4]float64, 0, len(solid.Footprint)+2)}
		for index, point := range solid.Footprint {
			end := solid.Footprint[(index+1)%len(solid.Footprint)]
			deltaX, deltaY := end.X-point.X, end.Y-point.Y
			length := math.Hypot(deltaX, deltaY)

			if length > 0 {
				obstacle.planes = append(obstacle.planes,
					[4]float64{deltaY / length, -deltaX / length, 0, (deltaX*point.Y - deltaY*point.X) / length},
				)
			}
		}

		for _, plane := range [][4]float64{
			{solid.Top.A, solid.Top.B, -1, solid.Top.C},
			{-solid.Bottom.A, -solid.Bottom.B, 1, -solid.Bottom.C},
		} {
			length := math.Sqrt(plane[0]*plane[0] + plane[1]*plane[1] + 1)
			// Authored planes above point into the solid; reverse for distance.
			for axis := range plane {
				plane[axis] /= -length
			}

			obstacle.planes = append(obstacle.planes, plane)
		}

		search.solids = append(search.solids, obstacle)
	}
}

func (search *reflectionPlacement) consider(point world.Vec2) {
	origin, valid := search.origin(point)
	if !valid {
		return
	}

	clearance := search.distance(origin)
	if clearance <= 0 {
		return
	}

	distance := (point.X-search.center.X)*(point.X-search.center.X) + (point.Y-search.center.Y)*(point.Y-search.center.Y)
	if clearance > search.clearance+reflectionPlacementTolerance ||
		math.Abs(clearance-search.clearance) <= reflectionPlacementTolerance && distance < search.centerDistance {
		search.best, search.clearance, search.centerDistance = origin, clearance, distance
	}
}

func (search *reflectionPlacement) origin(point world.Vec2) ([3]float32, bool) {
	// Round before containment and scoring: the stored float32 origin must be
	// the position that was validated, including in worlds far from zero.
	positionX, positionY := float64(float32(point.X)), float64(float32(point.Y))

	for _, index := range search.members {
		cell := search.document.Sectors[index]
		inside := true

		for _, wall := range cell.Walls {
			if (wall.End.X-wall.Start.X)*(positionY-wall.Start.Y)-(wall.End.Y-wall.Start.Y)*(positionX-wall.Start.X) < -1e-7 {
				inside = false

				break
			}
		}

		if inside {
			floor, ceiling := cell.Floor.A*positionX+cell.Floor.B*positionY+cell.Floor.C, cell.Ceiling.A*positionX+cell.Ceiling.B*positionY+cell.Ceiling.C
			height := float32((floor + ceiling) * reflectionMidpoint)

			if float64(height) > floor && float64(height) < ceiling {
				return [3]float32{float32(positionX), float32(positionY), height}, true
			}
		}
	}

	return [3]float32{}, false
}

func (search *reflectionPlacement) distance(origin [3]float32) float64 {
	positionX, positionY, height := float64(origin[0]), float64(origin[1]), float64(origin[2])
	clearance := math.Inf(1)

	for _, wall := range search.walls {
		deltaX, deltaY := wall.end.X-wall.start.X, wall.end.Y-wall.start.Y
		length := deltaX*deltaX + deltaY*deltaY

		if length == 0 {
			continue
		}

		t := max(0, min(1, ((positionX-wall.start.X)*deltaX+(positionY-wall.start.Y)*deltaY)/length))
		clearance = min(clearance, math.Hypot(positionX-wall.start.X-t*deltaX, positionY-wall.start.Y-t*deltaY))
	}

	for _, solid := range search.solids {
		distance := math.Inf(-1)
		for _, plane := range solid.planes {
			distance = max(distance, plane[0]*positionX+plane[1]*positionY+plane[2]*height+plane[3])
		}

		clearance = min(clearance, distance)
	}

	return clearance
}
