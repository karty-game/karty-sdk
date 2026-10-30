package worldchunk

import (
	"math"
	"slices"

	"github.com/karty-game/karty-sdk/format/world"
)

type pointIndex [3][]world.Vec3

func coordinate(v world.Vec3, axis int) float64 {
	if axis == 0 {
		return v.X
	}
	if axis == 1 {
		return v.Y
	}
	return v.Z
}
func newPointIndex(points []world.Vec3) pointIndex {
	var result pointIndex
	for axis := range 3 {
		result[axis] = slices.Clone(points)
		slices.SortFunc(result[axis], func(a, b world.Vec3) int { return compare(coordinate(a, axis), coordinate(b, axis)) })
	}
	return result
}
func compare(a, b float64) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}
func (index pointIndex) split(polygon []world.Vec3) []world.Vec3 {
	result := make([]world.Vec3, 0, len(polygon))
	for i, a := range polygon {
		b := polygon[(i+1)%len(polygon)]
		delta := sub(b, a)
		length := dot(delta, delta)
		axis := 0
		for candidate := 1; candidate < 3; candidate++ {
			if math.Abs(coordinate(delta, candidate)) > math.Abs(coordinate(delta, axis)) {
				axis = candidate
			}
		}
		cuts := []float64{0}
		low, high := math.Min(coordinate(a, axis), coordinate(b, axis)), math.Max(coordinate(a, axis), coordinate(b, axis))
		start, _ := slices.BinarySearchFunc(index[axis], low, func(p world.Vec3, v float64) int { return compare(coordinate(p, axis), v) })
		for _, p := range index[axis][start:] {
			if coordinate(p, axis) > high {
				break
			}
			t := dot(sub(p, a), delta) / length
			if t <= 1e-9 || t >= 1-1e-9 {
				continue
			}
			difference := sub(p, add(a, scale(delta, t)))
			if dot(difference, difference) <= 1e-16 {
				cuts = append(cuts, t)
			}
		}
		slices.Sort(cuts)
		last := -1.0
		for _, t := range cuts {
			if t-last > 1e-9 {
				point := a
				if t != 0 {
					// Subdivide the edge that will actually be stored. Interpolating
					// float64 endpoints can cross a float32 rounding boundary near
					// a corner and turn consecutive cuts into a reversed spike.
					storedA, storedB := unpacked(packed(a)), unpacked(packed(b))
					point = add(storedA, scale(sub(storedB, storedA), t))
				}
				result = append(result, point)
				last = t
			}
		}
	}
	return result
}
