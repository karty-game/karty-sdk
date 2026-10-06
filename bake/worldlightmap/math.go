package worldlightmapbake

import (
	"math"

	"github.com/karty-game/karty-sdk/format/world"
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
func square(v float64) float64      { return v * v }
func halfLambert(c float64) float64 { return square(.5*c + .5) }
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

// Frames depend only on the fixed geometric normal, not the ray sample.
type sampleFrame struct{ tangent, bitangent vec }

func newSampleFrame(n vec) sampleFrame {
	t, b := frame(n)
	return sampleFrame{t, b}
}

func (f sampleFrame) direction(n vec, radius, height, v float64) vec {
	angle := 2 * math.Pi * v
	return add(add(scale(f.tangent, radius*math.Cos(angle)), scale(f.bitangent, radius*math.Sin(angle))), scale(n, height))
}

func (f sampleFrame) hemisphere(n vec, u, v float64) vec {
	return f.direction(n, math.Sqrt(u), math.Sqrt(1-u), v)
}

// A bounded per-bake table preserves the exact initial stratified samples.
type initialSample struct {
	radius, height, angle float64
	seed                  uint64
}

func initialSamples(count int) []initialSample {
	samples := make([]initialSample, count)
	for i := range samples {
		u := (float64(i) + .5) / float64(count)
		samples[i] = initialSample{math.Sqrt(u), math.Sqrt(1 - u), float64(i) * .6180339887498949, uint64(i+1) * 0xa0761d6478bd642f}
	}
	return samples
}
