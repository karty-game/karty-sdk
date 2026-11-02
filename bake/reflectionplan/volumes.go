package reflectionplan

import (
	"math"

	sdkworld "github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldreflection"
)

const reflectionMidpoint = .5

// Authored convex pieces share one static environment, independent of how
// resource budgets partition their meshes. Unnamed cells retain sector identity.
func reflectionVolumes(document *sdkworld.Document) ([sdkworld.MaxSectors]int, [sdkworld.MaxSectors]worldreflection.Probe) {
	var (
		groups [sdkworld.MaxSectors]int
		probes [sdkworld.MaxSectors]worldreflection.Probe
	)

	for index := range groups {
		groups[index] = -1
	}

	for seed, sector := range document.Sectors {
		if groups[seed] >= 0 || sector.Sky {
			continue
		}

		members := []int{seed}
		groups[seed] = seed

		for next := 0; next < len(members); next++ {
			for _, wall := range document.Sectors[members[next]].Walls {
				other := int(wall.Portal)
				if other < 0 || groups[other] >= 0 || !reflectionRoomNeighbors(document, members[next], wall) {
					continue
				}

				groups[other] = seed
				members = append(members, other)
			}
		}

		probe := buildReflectionVolume(document, seed, members)
		probes[seed] = probe
	}

	return groups, probes
}

func reflectionRoomNeighbors(document *sdkworld.Document, index int, wall sdkworld.Wall) bool {
	left, right := document.Sectors[index], document.Sectors[wall.Portal]
	if !reflectionCeilingContinuous(left, right) || right.Sky {
		return false
	}

	same := left.SourceRoom != "" && left.SourceRoom == right.SourceRoom && left.Instance == right.Instance
	width := math.Hypot(wall.End.X-wall.Start.X, wall.End.Y-wall.Start.Y)

	const openingFraction = .75
	if !same && width*width < openingFraction*openingFraction*min(reflectionSectorArea(left), reflectionSectorArea(right)) {
		return false
	}

	for _, reverse := range right.Walls {
		if int(reverse.Portal) == index && math.Hypot(wall.Start.X-reverse.End.X, wall.Start.Y-reverse.End.Y) < 1e-9 &&
			math.Hypot(wall.End.X-reverse.Start.X, wall.End.Y-reverse.Start.Y) < 1e-9 {
			return true
		}
	}

	return false
}

func reflectionSectorArea(sector sdkworld.Sector) float64 {
	origin := sector.Walls[0].Start

	area := 0.0
	for _, wall := range sector.Walls {
		area += (wall.Start.X-origin.X)*(wall.End.Y-origin.Y) - (wall.End.X-origin.X)*(wall.Start.Y-origin.Y)
	}

	return math.Abs(area) * reflectionMidpoint
}

func reflectionOriginClear(document *sdkworld.Document, members []int, origin [3]float32) bool {
	positionX, positionY, height := float64(origin[0]), float64(origin[1]), float64(origin[2])
	inside := false

	for _, index := range members {
		cell := document.Sectors[index]
		if height <= cell.Floor.A*positionX+cell.Floor.B*positionY+cell.Floor.C ||
			height >= cell.Ceiling.A*positionX+cell.Ceiling.B*positionY+cell.Ceiling.C {
			continue
		}

		contained := true

		for _, wall := range cell.Walls {
			if (wall.End.X-wall.Start.X)*(positionY-wall.Start.Y)-(wall.End.Y-wall.Start.Y)*(positionX-wall.Start.X) < -1e-7 {
				contained = false

				break
			}
		}

		inside = inside || contained
	}

	if !inside {
		return false
	}

	if document.StaticSolids != nil {
		for _, solid := range document.StaticSolids.Items {
			if height <= solid.Bottom.A*positionX+solid.Bottom.B*positionY+solid.Bottom.C ||
				height >= solid.Top.A*positionX+solid.Top.B*positionY+solid.Top.C {
				continue
			}

			contained := true

			for index, point := range solid.Footprint {
				end := solid.Footprint[(index+1)%len(solid.Footprint)]
				if (end.X-point.X)*(positionY-point.Y)-(end.Y-point.Y)*(positionX-point.X) < -1e-7 {
					contained = false

					break
				}
			}

			if contained {
				return false
			}
		}
	}

	return true
}

func buildReflectionVolume(document *sdkworld.Document, seed int, members []int) worldreflection.Probe {
	sector := document.Sectors[seed]

	probe, valid := sectorReflectionProbe(seed, sector)
	if !valid {
		return worldreflection.Probe{}
	}

	var center sdkworld.Vec2

	area := 0.0

	for _, index := range members {
		cell := document.Sectors[index]

		candidate, ok := sectorReflectionProbe(index, cell)
		if !ok {
			return worldreflection.Probe{}
		}

		weight := reflectionSectorArea(cell)
		xy := reflectionProbeCenter(cell)
		center.X += xy.X * weight
		center.Y += xy.Y * weight
		area += weight

		for axis := range 3 {
			probe.Low[axis] = min(probe.Low[axis], candidate.Low[axis])
			probe.High[axis] = max(probe.High[axis], candidate.High[axis])
		}
	}

	center.X /= area
	center.Y /= area

	probe.Origin = [3]float32{
		float32(center.X),
		float32(center.Y),
		float32(
			((sector.Floor.A+sector.Ceiling.A)*center.X + (sector.Floor.B+sector.Ceiling.B)*center.Y + sector.Floor.C + sector.Ceiling.C) * reflectionMidpoint,
		),
	}
	probe.Origin = placeReflectionCaptureOrigin(document, members, center, probe)

	if !reflectionOriginClear(document, members, probe.Origin) {
		return worldreflection.Probe{}
	}

	probe.Low[2] -= reflectionWaveExtent(document)

	return probe
}

func reflectionWaveExtent(document *sdkworld.Document) float32 {
	extent := float32(0)

	if document.Animations != nil {
		for _, preset := range document.Animations.Presets {
			if preset.Kind == "liquid" {
				extent = max(extent, float32(preset.SurfaceAmplitude))
			}
		}
	}

	return extent
}

// A staircase built from short, equal-headroom cells is one space. A ceiling
// change with a level floor remains a room boundary.
func reflectionCeilingContinuous(left, right sdkworld.Sector) bool {
	if left.Ceiling == right.Ceiling {
		return true
	}

	if left.Floor.A != right.Floor.A || left.Floor.B != right.Floor.B ||
		left.Ceiling.A != right.Ceiling.A || left.Ceiling.B != right.Ceiling.B {
		return false
	}

	floorStep, ceilingStep := right.Floor.C-left.Floor.C, right.Ceiling.C-left.Ceiling.C

	return math.Abs(floorStep) <= .4 && math.Abs(floorStep) > 1e-7 && math.Abs(floorStep-ceilingStep) < 1e-7
}
