package reflectionplan

import (
	"math"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldreflection"
)

func reflectionSolidBindings(
	document *world.Document,
	groups *[world.MaxSectors]int,
	volumes *[world.MaxSectors]worldreflection.Probe,
) []int {
	if document.StaticSolids == nil {
		return nil
	}

	solids := document.StaticSolids.Items

	bindings := make([]int, len(solids))
	for index := range solids {
		bindings[index] = reflectionProbeForSolid(document, index, groups, volumes)
	}

	if document.Fluids == nil {
		return bindings
	}

	for _, fluid := range document.Fluids.Items {
		members := make([]int, 0, len(fluid.Solids))
		for _, id := range fluid.Solids {
			for index, solid := range solids {
				if solid.ID == id {
					members = append(members, index)

					break
				}
			}
		}

		assignReflectionLiquidComponents(solids, members, bindings, volumes)
	}

	return bindings
}

// A connected, coplanar liquid sheet must not change environment at a storage
// boundary or over a stair beneath it. Disconnected pools retain separate probes.
func assignReflectionLiquidComponents(
	solids []world.Solid,
	members, bindings []int,
	volumes *[world.MaxSectors]worldreflection.Probe,
) {
	assigned := make([]bool, len(members))
	for seed := range members {
		if assigned[seed] {
			continue
		}

		assigned[seed] = true

		component := []int{seed}
		for next := 0; next < len(component); next++ {
			left := solids[members[component[next]]]
			for candidate, index := range members {
				if !assigned[candidate] && left.Top == solids[index].Top &&
					reflectionPolygonsTouch(left.Footprint, solids[index].Footprint) {
					assigned[candidate] = true
					component = append(component, candidate)
				}
			}
		}

		best := reflectionLiquidWinner(solids, members, component, bindings)
		if best >= 0 && reflectionLiquidFits(volumes[best], solids, members, component) {
			for _, candidate := range component {
				bindings[members[candidate]] = best
			}
		}
	}
}

func reflectionPolygonsTouch(left, right []world.Vec2) bool {
	for _, polygon := range [][]world.Vec2{left, right} {
		for index, point := range polygon {
			end := polygon[(index+1)%len(polygon)]
			axisX, axisY := point.Y-end.Y, end.X-point.X
			lowLeft, highLeft := reflectionPolygonProjection(left, axisX, axisY)

			lowRight, highRight := reflectionPolygonProjection(right, axisX, axisY)
			if highLeft < lowRight-1e-7 || highRight < lowLeft-1e-7 {
				return false
			}
		}
	}

	return true
}
func reflectionPolygonProjection(polygon []world.Vec2, axisX, axisY float64) (float64, float64) {
	low, high := math.Inf(1), math.Inf(-1)

	for _, point := range polygon {
		value := point.X*axisX + point.Y*axisY
		low = min(low, value)
		high = max(high, value)
	}

	return low, high
}

func reflectionLiquidWinner(solids []world.Solid, members, component, bindings []int) int {
	var areas [world.MaxSectors]float64

	best := -1
	largest := 0.0

	for _, candidate := range component {
		index := members[candidate]

		probeID := bindings[index]
		if probeID < 0 || probeID >= world.MaxSectors {
			continue
		}

		polygon := solids[index].Footprint
		area := 0.0

		for vertex, point := range polygon {
			end := polygon[(vertex+1)%len(polygon)]
			area += point.X*end.Y - end.X*point.Y
		}

		areas[probeID] += math.Abs(area)
		if best < 0 || areas[probeID] > largest || areas[probeID] == largest && probeID < best {
			best, largest = probeID, areas[probeID]
		}
	}

	return best
}

// Connected water can cross a doorway. Preserve separate chamber assignments
// unless one projection volume can cover the complete connected sheet.
func reflectionLiquidFits(probe worldreflection.Probe, solids []world.Solid, members, component []int) bool {
	for _, candidate := range component {
		solid := solids[members[candidate]]
		for _, point := range solid.Footprint {
			position := [3]float32{float32(point.X), float32(point.Y), float32(solid.Top.A*point.X + solid.Top.B*point.Y + solid.Top.C)}
			for axis := range 3 {
				if position[axis] < probe.Low[axis]-.001 || position[axis] > probe.High[axis]+.001 {
					return false
				}
			}
		}
	}

	return true
}
