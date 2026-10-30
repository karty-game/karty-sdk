package worldterrain

import "github.com/karty-game/karty-sdk/format/world"

// PlaneAt returns the exact triangle plane z=A*x+B*y+C used by CompilePatch
// and HeightAt. The document must already be validated and remain immutable.
func PlaneAt(document *Document, x, y float64) (world.Plane, bool) {
	if _, ok := HeightAt(document, x, y); !ok {
		return world.Plane{}, false
	}
	cx, fx, _ := axisCell(document.Origin[0], document.CellSize, int(document.Columns), x)
	cy, fy, _ := axisCell(document.Origin[1], document.CellSize, int(document.Rows), y)
	point := func(gx, gy int) world.Vec3 {
		return world.Vec3{
			X: float64(document.Origin[0] + float32(gx)*document.CellSize),
			Y: float64(document.Origin[1] + float32(gy)*document.CellSize),
			Z: float64(document.Heights[gy*int(document.Columns)+gx]),
		}
	}
	a, b, c := point(cx, cy), point(cx+1, cy), point(cx, cy+1)
	if fx+fy > 1 {
		a, b = b, point(cx+1, cy+1)
	}
	ux, uy, uz := b.X-a.X, b.Y-a.Y, b.Z-a.Z
	vx, vy, vz := c.X-a.X, c.Y-a.Y, c.Z-a.Z
	nx, ny, nz := uy*vz-uz*vy, uz*vx-ux*vz, ux*vy-uy*vx
	plane := world.Plane{A: -nx / nz, B: -ny / nz}
	plane.C = a.Z - plane.A*a.X - plane.B*a.Y
	return plane, true
}
