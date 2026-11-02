package reflectionplan

import (
	"math"

	sdkworld "github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldreflection"
)

const maxReflectionProbeExtent = 65536

func sectorReflectionProbe(index int, sector sdkworld.Sector) (worldreflection.Probe, bool) {
	probe := worldreflection.Probe{
		ID:   index,
		Low:  [3]float32{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32},
		High: [3]float32{-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32},
	}

	center := reflectionProbeCenter(sector)
	for _, wall := range sector.Walls {
		point := [3]float32{
			float32(wall.Start.X),
			float32(wall.Start.Y),
			float32(sector.Floor.A*wall.Start.X + sector.Floor.B*wall.Start.Y + sector.Floor.C),
		}
		for axis := range 3 {
			probe.Low[axis] = min(probe.Low[axis], point[axis])
			probe.High[axis] = max(probe.High[axis], point[axis])
		}

		probe.High[2] = max(probe.High[2], float32(sector.Ceiling.A*wall.Start.X+sector.Ceiling.B*wall.Start.Y+sector.Ceiling.C))
	}

	floor := sector.Floor.A*center.X + sector.Floor.B*center.Y + sector.Floor.C
	ceiling := sector.Ceiling.A*center.X + sector.Ceiling.B*center.Y + sector.Ceiling.C
	probe.Origin = [3]float32{float32(center.X), float32(center.Y), float32((floor + ceiling) * .5)}

	valid := true
	for axis := range 3 {
		valid = valid && probe.Low[axis] < probe.Origin[axis] && probe.Origin[axis] < probe.High[axis] &&
			probe.High[axis]-probe.Low[axis] <= maxReflectionProbeExtent
	}

	return probe, valid
}

// Integrate signed triangle areas around a local origin. Additional portal-edge
// vertices do not pull the capture point toward the more densely split wall.
func reflectionProbeCenter(sector sdkworld.Sector) sdkworld.Vec2 {
	const triangleVertexCount = 3

	origin := sector.Walls[0].Start

	var (
		weighted sdkworld.Vec2
		area     float64
	)

	for _, wall := range sector.Walls {
		a, b := sdkworld.Vec2{X: wall.Start.X - origin.X, Y: wall.Start.Y - origin.Y},
			sdkworld.Vec2{X: wall.End.X - origin.X, Y: wall.End.Y - origin.Y}
		cross := a.X*b.Y - b.X*a.Y
		area += cross
		weighted.X += (a.X + b.X) * cross
		weighted.Y += (a.Y + b.Y) * cross
	}
	// Worlds are validated as nondegenerate convex polygons before import.
	return sdkworld.Vec2{X: origin.X + weighted.X/(triangleVertexCount*area), Y: origin.Y + weighted.Y/(triangleVertexCount*area)}
}
