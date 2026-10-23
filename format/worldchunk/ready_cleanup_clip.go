package worldchunk

import (
	"slices"

	"github.com/karty-game/karty-sdk/format/world"
)

// Subtract a convex volume as a disjoint sequence of outside half-space pieces.
// No tolerance expands an occluder: positive authored gaps stay positive. Exact
// semantic coplanarity is handled explicitly rather than by an epsilon slab.
func cleanupSubtract(polygons [][]world.Vec3, planes []cleanupPlane, source cleanupPlane) ([][]world.Vec3, bool, error) {
	result := make([][]world.Vec3, 0, len(polygons))
	changed := false
	for _, polygon := range polygons {
		inside := polygon
		outside := make([][]world.Vec3, 0)
		for _, plane := range planes {
			front, back := cleanupSplit(inside, plane)
			if len(front) >= 3 {
				if len(outside) >= MaxTriangles {
					return nil, false, ErrReadyCapacity
				}
				outside = append(outside, front)
			}
			inside = back
			if len(inside) < 3 {
				break
			}
		}
		if len(inside) < 3 || cleanupPolygonArea(inside, source.normal) == 0 {
			// An empty intersection must not introduce needless boundary splits.
			if len(result) >= MaxTriangles {
				return nil, false, ErrReadyCapacity
			}
			result = append(result, polygon)
			continue
		}
		changed = true
		if len(result)+len(outside) > MaxTriangles {
			return nil, false, ErrReadyCapacity
		}
		result = append(result, outside...)
	}
	return result, changed, nil
}

func cleanupSplit(polygon []world.Vec3, plane cleanupPlane) ([]world.Vec3, []world.Vec3) {
	if len(polygon) == 0 {
		return nil, nil
	}
	front, back := make([]world.Vec3, 0, len(polygon)+1), make([]world.Vec3, 0, len(polygon)+1)
	previous := polygon[len(polygon)-1]
	previousDistance := plane.distance(previous)
	for _, point := range polygon {
		distance := plane.distance(point)
		if (distance > 0) != (previousDistance > 0) {
			intersection := cleanupIntersection(previous, point, plane)
			front = cleanupPush(front, intersection)
			back = cleanupPush(back, intersection)
		}
		if distance > 0 {
			front = cleanupPush(front, point)
		} else {
			back = cleanupPush(back, point)
		}
		previous, previousDistance = point, distance
	}
	return cleanupClose(front), cleanupClose(back)
}

func cleanupIntersection(a, b world.Vec3, plane cleanupPlane) world.Vec3 {
	// Shared triangle boundaries use the same arithmetic direction, independently
	// of their winding or source order, before final float32 vertex storage.
	if cleanupPointLess(b, a) {
		a, b = b, a
	}
	da, db := plane.distance(a), plane.distance(b)
	if da == 0 {
		return a
	}
	if db == 0 {
		return b
	}
	return add(a, scale(sub(b, a), da/(da-db)))
}

func cleanupPointLess(a, b world.Vec3) bool {
	if a.X != b.X {
		return a.X < b.X
	}
	if a.Y != b.Y {
		return a.Y < b.Y
	}
	return a.Z < b.Z
}

func cleanupPush(points []world.Vec3, point world.Vec3) []world.Vec3 {
	if len(points) == 0 || points[len(points)-1] != point {
		points = append(points, point)
	}
	return points
}

func cleanupClose(points []world.Vec3) []world.Vec3 {
	if len(points) > 1 && points[0] == points[len(points)-1] {
		points = points[:len(points)-1]
	}
	return points
}

func cleanupPolygonArea(polygon []world.Vec3, normal world.Vec3) float64 {
	area := 0.0
	for index := 1; index+1 < len(polygon); index++ {
		area += dot(cross(sub(polygon[index], polygon[0]), sub(polygon[index+1], polygon[0])), normal)
	}
	return area
}

// Strictly positive convex ears retain all boundary vertices, including every
// collinear authored/T-junction point. A packed-precision ambiguity falls back
// to the original source triangle instead of discarding a possible surface.
func cleanupTriangulate(polygon []world.Vec3, normal world.Vec3) ([][3]world.Vec3, bool) {
	points := make([]world.Vec3, 0, len(polygon))
	for _, point := range polygon {
		if len(points) == 0 || packed(points[len(points)-1]) != packed(point) {
			points = append(points, point)
		}
	}
	if len(points) > 1 && packed(points[0]) == packed(points[len(points)-1]) {
		points = points[:len(points)-1]
	}
	if len(points) < 3 {
		return nil, true
	}
	result := make([][3]world.Vec3, 0, len(points)-2)
	for len(points) > 3 {
		found := false
		for index, b := range points {
			a, c := points[(index+len(points)-1)%len(points)], points[(index+1)%len(points)]
			storedA, storedB, storedC := unpacked(packed(a)), unpacked(packed(b)), unpacked(packed(c))
			if dot(cross(sub(storedB, storedA), sub(storedC, storedA)), normal) <= 0 || cleanupRemainingArea(points, index, normal) <= 0 {
				continue
			}
			result = append(result, [3]world.Vec3{a, b, c})
			points = slices.Delete(points, index, index+1)
			found = true
			break
		}
		if !found {
			return nil, false
		}
	}
	a, b, c := unpacked(packed(points[0])), unpacked(packed(points[1])), unpacked(packed(points[2]))
	area := dot(cross(sub(b, a), sub(c, a)), normal)
	if area < 0 {
		return nil, false
	}
	if area > 0 {
		result = append(result, [3]world.Vec3{points[0], points[1], points[2]})
	}
	return result, true
}

// Leave positive area for the final triangle, so an otherwise valid ear cannot
// strand an authored collinear boundary chain in a discarded zero-area tail.
func cleanupRemainingArea(points []world.Vec3, removed int, normal world.Vec3) float64 {
	var first, previous world.Vec3
	count, area := 0, 0.0
	for index, point := range points {
		if index == removed {
			continue
		}
		point = unpacked(packed(point))
		if count == 0 {
			first = point
		} else if count >= 2 {
			area += dot(cross(sub(previous, first), sub(point, first)), normal)
		}
		previous = point
		count++
	}
	return area
}
