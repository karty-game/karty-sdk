package worldlightmapbake

import (
	"github.com/karty-game/karty-sdk/format/world"
	"math"
)

type vec = world.Vec3

func add(a, b vec) vec           { return vec{X: a.X + b.X, Y: a.Y + b.Y, Z: a.Z + b.Z} }
func sub(a, b vec) vec           { return vec{X: a.X - b.X, Y: a.Y - b.Y, Z: a.Z - b.Z} }
func scale(a vec, s float64) vec { return vec{X: a.X * s, Y: a.Y * s, Z: a.Z * s} }
func mul(a, b vec) vec           { return vec{X: a.X * b.X, Y: a.Y * b.Y, Z: a.Z * b.Z} }
func dot(a, b vec) float64       { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }
func cross(a, b vec) vec {
	return vec{X: a.Y*b.Z - a.Z*b.Y, Y: a.Z*b.X - a.X*b.Z, Z: a.X*b.Y - a.Y*b.X}
}
func unit(a vec) vec { return scale(a, 1/math.Sqrt(dot(a, a))) }
func component(a vec, i int) float64 {
	if i == 0 {
		return a.X
	}
	if i == 1 {
		return a.Y
	}
	return a.Z
}
func maximum(a vec) float64 { return math.Max(a.X, math.Max(a.Y, a.Z)) }
func epsilon(p vec) float64 {
	return math.Max(1e-8, math.Max(math.Abs(p.X), math.Max(math.Abs(p.Y), math.Abs(p.Z)))*1e-12)
}
func halfLambert(c float64) float64 { return math.Pow(.5*c+.5, 2) }
func frame(n vec) (vec, vec) {
	a := vec{Z: 1}
	if math.Abs(n.Z) > .9 {
		a = vec{Y: 1}
	}
	t := unit(cross(a, n))
	return t, cross(n, t)
}

var rnmBasis = [3]vec{
	{X: math.Sqrt(2.0 / 3), Z: 1 / math.Sqrt(3)},
	{X: -1 / math.Sqrt(6), Y: 1 / math.Sqrt(2), Z: 1 / math.Sqrt(3)},
	{X: -1 / math.Sqrt(6), Y: -1 / math.Sqrt(2), Z: 1 / math.Sqrt(3)},
}

type random uint64

func (r *random) next() float64 {
	*r += 0x9e3779b97f4a7c15
	z := uint64(*r)
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return float64((z^(z>>31))>>11) * (1.0 / (1 << 53))
}
func hemisphere(n vec, u, v float64) vec {
	t, b := frame(n)
	radius := math.Sqrt(u)
	angle := 2 * math.Pi * v
	return add(add(scale(t, radius*math.Cos(angle)), scale(b, radius*math.Sin(angle))), scale(n, math.Sqrt(1-u)))
}
