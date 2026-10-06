package worldsource

import (
	"math"
	"unicode/utf8"
)

func validIdentifier(value string) bool {
	return len(value) > 0 && len(value) <= MaxIdentifierBytes && utf8.ValidString(value)
}

func validVec2(value Vec2) bool {
	return finite(value.X) && finite(value.Y)
}

func validVec3(value Vec3) bool {
	return finite(value.X) && finite(value.Y) && finite(value.Z)
}

func validPlane(value Plane) bool {
	return finite(value.A) && finite(value.B) && finite(value.C)
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && math.Abs(value) <= MaxCoordinate
}

func distanceSquared(left, right Vec2) float64 {
	dx, dy := right.X-left.X, right.Y-left.Y

	return dx*dx + dy*dy
}

func cross(start, end, point Vec2) float64 {
	return (end.X-start.X)*(point.Y-start.Y) - (end.Y-start.Y)*(point.X-start.X)
}

func planeHeight(plane Plane, point Vec2) float64 {
	return plane.A*point.X + plane.B*point.Y + plane.C
}
