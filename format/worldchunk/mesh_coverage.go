package worldchunk

import (
	"fmt"
	"math"
	"slices"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

type meshEdge [2][3]float32

type meshCoverage struct {
	surface    worldlightmap.Surface
	polygons   [][]world.Vec3
	edges      map[meshEdge]int
	mapping    *world.SurfaceUV
	secondary  *world.SurfaceSecondary
	bases      [3]Basis
	attributes []meshAttributeDomain
}

func renderPolygons(document *world.Document, surface worldlightmap.Surface) [][]world.Vec3 {
	binding := surface.Binding
	if binding.Kind == "sector-ceiling" && document.Sectors[binding.Index].Sky {
		return [][]world.Vec3{capPolygon(document.Sectors[binding.Index], true)}
	}
	if binding.Kind == "sector-wall" {
		sector := document.Sectors[binding.Index]
		wall := sector.Walls[binding.Edge]
		if wall.Sky {
			return [][]world.Vec3{{at(wall.Start, sector.Floor), at(wall.End, sector.Floor),
				at(wall.End, sector.Ceiling), at(wall.Start, sector.Ceiling)}}
		}
	}
	return surface.Polygons
}

// Four coordinate ULPs accommodate packed endpoints and barycentric samples.
// Keep each axis independent: a large X must not relax a precise Z plane.
func coordinateTolerance(coordinate float64) float64 {
	value := float32(coordinate)
	return max(1e-7, 4*math.Abs(float64(math.Nextafter32(value, float32(math.Inf(1))))-float64(value)))
}

func projectedTolerance(position, direction world.Vec3) float64 {
	return math.Abs(direction.X)*coordinateTolerance(position.X) +
		math.Abs(direction.Y)*coordinateTolerance(position.Y) +
		math.Abs(direction.Z)*coordinateTolerance(position.Z)
}

func polygonOrientation(polygon []world.Vec3, normal world.Vec3) int {
	sum := world.Vec3{}
	for index, point := range polygon {
		sum = add(sum, cross(point, polygon[(index+1)%len(polygon)]))
	}
	if dot(sum, normal) < 0 {
		return -1
	}
	return 1
}

func insideRenderPolygon(position world.Vec3, polygon []world.Vec3, normal world.Vec3) bool {
	if len(polygon) < 3 {
		return false
	}
	tolerance := max(projectedTolerance(position, normal), projectedTolerance(polygon[0], normal))
	if math.Abs(dot(sub(position, polygon[0]), normal)) > tolerance {
		return false
	}
	orientation := float64(polygonOrientation(polygon, normal))
	for index, start := range polygon {
		end := polygon[(index+1)%len(polygon)]
		if packed(start) == packed(end) {
			continue // A clipping intersection can repeat at stored precision.
		}
		edge := sub(end, start)
		direction := cross(normal, edge)
		tolerance := max(projectedTolerance(position, direction), projectedTolerance(start, direction))
		if orientation*dot(direction, sub(position, start)) < -tolerance {
			return false
		}
	}
	return true
}

func (coverage *meshCoverage) addTriangle(vertices []Vertex) error {
	for _, vertex := range vertices {
		position := unpacked(vertex.Position)
		inside := false
		for _, polygon := range coverage.polygons {
			if insideRenderPolygon(position, polygon, coverage.surface.Normal) {
				inside = true
				break
			}
		}
		if !inside {
			return fmt.Errorf("vertex %v outside authored %s %d/%d: %w", position,
				coverage.surface.Binding.Kind, coverage.surface.Binding.Index, coverage.surface.Binding.Edge, ErrChunk)
		}
	}
	if coverage.edges == nil {
		coverage.edges = make(map[meshEdge]int)
	}
	for index, vertex := range vertices {
		a, b := vertex.Position, vertices[(index+1)%3].Position
		sign := 1
		if liquidPositionBefore(b, a) {
			a, b, sign = b, a, -1
		}
		edge := meshEdge{a, b}
		coverage.edges[edge] += sign
		if coverage.edges[edge] == 0 {
			delete(coverage.edges, edge)
		}
	}
	return nil
}

type boundaryEvent struct {
	at           float64
	want, actual int
}

type boundaryLine struct {
	origin, delta world.Vec3
	axis          int
	low, high     float64
	tolerance     float64
	events        []boundaryEvent
}

func (line *boundaryLine) parameter(point world.Vec3) float64 {
	return (coordinate(point, line.axis) - coordinate(line.origin, line.axis)) / coordinate(line.delta, line.axis)
}

func (line *boundaryLine) contains(a, b world.Vec3, tolerance float64) bool {
	length := math.Sqrt(dot(line.delta, line.delta))
	for _, point := range [2]world.Vec3{a, b} {
		crossing := cross(sub(point, line.origin), line.delta)
		if math.Sqrt(dot(crossing, crossing)) > tolerance*length {
			return false
		}
	}
	return true
}

func (line *boundaryLine) containsStored(a, b world.Vec3) bool {
	for _, direction := range [3]world.Vec3{
		{Y: line.delta.Z, Z: -line.delta.Y},
		{X: -line.delta.Z, Z: line.delta.X},
		{X: line.delta.Y, Y: -line.delta.X},
	} {
		for _, point := range [2]world.Vec3{a, b} {
			tolerance := max(projectedTolerance(point, direction), projectedTolerance(line.origin, direction))
			if math.Abs(dot(sub(point, line.origin), direction)) > tolerance {
				return false
			}
		}
	}
	return true
}

func (line *boundaryLine) append(a, b world.Vec3, count int, actual bool) {
	start, end := line.parameter(a), line.parameter(b)
	if start > end {
		start, end, count = end, start, -count
	}
	first, last := boundaryEvent{at: start}, boundaryEvent{at: end}
	if actual {
		first.actual, last.actual = count, -count
	} else {
		first.want, last.want = count, -count
		line.low, line.high = min(line.low, start), max(line.high, end)
	}
	line.events = append(line.events, first, last)
}

// Existing triangle edges cancel in pairs. The remaining directed boundary
// must cover the authored boundary once, including disconnected wall strips.
// This validates coverage without generating or subdividing any triangle.
func (coverage *meshCoverage) validateBoundary(work *cooperativeWork) error {
	var lines []boundaryLine
	for _, polygon := range coverage.polygons {
		stored := make([]world.Vec3, len(polygon))
		for index, point := range polygon {
			stored[index] = unpacked(packed(point))
		}
		sum := world.Vec3{}
		for index, point := range stored {
			sum = add(sum, cross(point, stored[(index+1)%len(stored)]))
		}
		if dot(sum, coverage.surface.Normal) == 0 {
			continue // An entire receiver can collapse in the stored precision.
		}
		orientation := polygonOrientation(polygon, coverage.surface.Normal)
		for index, a := range polygon {
			b := polygon[(index+1)%len(polygon)]
			if packed(a) == packed(b) {
				continue
			}
			found := -1
			for candidate := range lines {
				work.checkpoint(1024)
				if lines[candidate].contains(a, b, 1e-9) {
					found = candidate
					break
				}
			}
			if found < 0 {
				delta := sub(b, a)
				axis := 0
				for candidate := 1; candidate < 3; candidate++ {
					if math.Abs(coordinate(delta, candidate)) > math.Abs(coordinate(delta, axis)) {
						axis = candidate
					}
				}
				found = len(lines)
				lines = append(lines, boundaryLine{origin: a, delta: delta, axis: axis, low: math.Inf(1), high: math.Inf(-1)})
			}
			lines[found].append(a, b, orientation, false)
		}
	}
	for edge, count := range coverage.edges {
		work.checkpoint(1024)
		a, b := unpacked(edge[0]), unpacked(edge[1])
		found := false
		for index := range lines {
			line := &lines[index]
			tolerance := max(coordinateTolerance(coordinate(a, line.axis)), coordinateTolerance(coordinate(b, line.axis)),
				coordinateTolerance(coordinate(line.origin, line.axis)))
			parameterTolerance := tolerance / math.Abs(coordinate(line.delta, line.axis))
			start, end := line.parameter(a), line.parameter(b)
			if min(start, end) < line.low-parameterTolerance || max(start, end) > line.high+parameterTolerance ||
				!line.containsStored(a, b) {
				continue
			}
			line.tolerance = max(line.tolerance, parameterTolerance)
			line.append(a, b, count, true)
			found = true
			break
		}
		if !found {
			return fmt.Errorf("unmatched interior triangle edge: %w", ErrChunk)
		}
	}
	for _, line := range lines {
		work.checkpoint(1024)
		slices.SortFunc(line.events, func(a, b boundaryEvent) int { return compare(a.at, b.at) })
		want, actual := 0, 0
		for next := 0; next < len(line.events); {
			start := line.events[next].at
			for next < len(line.events) && line.events[next].at-start <= line.tolerance {
				want += line.events[next].want
				actual += line.events[next].actual
				next++
			}
			if want != actual {
				return fmt.Errorf("triangle boundary has a gap or overlap: %w", ErrChunk)
			}
		}
	}
	return nil
}
