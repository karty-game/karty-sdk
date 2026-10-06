package worldlightmapbake

import (
	"math"
	"testing"

	"simd"
)

func scalarBoxRay(b box, o, d vec, limit float64) bool {
	low := 0.0
	high := limit
	for axis := range 3 {
		v := component(d, axis)
		p := component(o, axis)
		a := component(b.lo, axis)
		c := component(b.hi, axis)
		if math.Abs(v) < 1e-15 {
			if p < a-1e-9 || p > c+1e-9 {
				return false
			}
			continue
		}
		x, y := (a-p)/v, (c-p)/v
		if x > y {
			x, y = y, x
		}
		low = math.Max(low, x)
		high = math.Min(high, y)
		if high < low {
			return false
		}
	}
	return true
}

// The old arithmetic lives only in tests as a numerical and benchmark oracle.
// intersect is double-sided: a back-facing wall still blocks light.
func scalarIntersect(t triangle, o, d vec, limit float64) (float64, bool) {
	e1, e2 := sub(t.b, t.a), sub(t.c, t.a)
	p := cross(d, e2)
	det := dot(e1, p)
	if math.Abs(det) < 1e-15 {
		return 0, false
	}
	s := sub(o, t.a)
	u := dot(s, p) / det
	if u < -1e-9 || u > 1+1e-9 {
		return 0, false
	}
	q := cross(s, e1)
	v := dot(d, q) / det
	if v < -1e-9 || u+v > 1+1e-9 {
		return 0, false
	}
	distance := dot(e2, q) / det
	return distance, distance > epsilon(o)*.25 && distance < limit
}
func scalarHit(a *accelerator, o, d vec, limit float64) (int, float64) {
	if len(a.nodes) == 0 {
		return -1, limit
	}
	stack := [64]int{0}
	count := 1
	found := -1
	for count > 0 {
		count--
		n := a.nodes[stack[count]]
		if !scalarBoxRay(n.bounds, o, d, limit) {
			continue
		}
		if n.left >= 0 {
			stack[count] = n.left
			stack[count+1] = n.right
			count += 2
			continue
		}
		for _, t := range a.triangles[n.start:n.end] {
			if distance, ok := scalarIntersect(t, o, d, limit); ok {
				limit = distance
				found = t.surface
			}
		}
	}
	return found, limit
}

func rayFixture() (*accelerator, *accelerator) {
	var triangles []triangle
	rng := random(77)
	for i := range 257 {
		p := vec{X: 20*rng.next() - 10, Y: 20*rng.next() - 10, Z: 10*rng.next() - 5}
		e1 := vec{X: 3*rng.next() - .5, Y: rng.next() - .5, Z: rng.next() - .5}
		e2 := vec{X: rng.next() - .5, Y: 3*rng.next() - .5, Z: rng.next() - .5}
		t := triangle{a: p, b: add(p, e1), c: add(p, e2), surface: i}
		t.bounds = emptyBox().point(t.a).point(t.b).point(t.c)
		triangles = append(triangles, t)
	}
	// Include degenerate triangles and shared edges; no lane may turn a miss
	// into a hit because a neighbouring lane divides by zero or reads padding.
	for i := range 3 {
		t := triangle{a: vec{X: float64(i)}, b: vec{X: float64(i) + 1}, c: vec{X: float64(i) + 1}, surface: 257 + i}
		t.bounds = emptyBox().point(t.a).point(t.b).point(t.c)
		triangles = append(triangles, t)
	}
	scalar := &accelerator{triangles: append([]triangle(nil), triangles...)}
	scalar.build(0, len(scalar.triangles))
	packed := &accelerator{triangles: triangles}
	packed.prepare()
	return scalar, packed
}

func TestSIMDRays(t *testing.T) {
	t.Logf("SIMD width=%d bits, emulated=%v", simd.VectorBitSize(), simd.Emulated())
	scalar, packed := rayFixture()
	rng := random(123)
	for i := range 10000 {
		o := vec{X: 24*rng.next() - 12, Y: 24*rng.next() - 12, Z: 12*rng.next() - 6}
		d := unit(vec{X: 2*rng.next() - 1, Y: 2*rng.next() - 1, Z: 2*rng.next() - 1})
		switch i % 11 {
		case 0:
			d = vec{Z: 1}
		case 1:
			d = vec{X: 1e-16, Y: 1}
		case 2:
			d = vec{X: 1}
		}
		limit := 20 * rng.next()
		if i%3 == 0 {
			limit = math.Inf(1)
		}
		want, wd := scalarHit(scalar, o, d, limit)
		got, gd := packed.hit(o, d, limit)
		if got != want || gd != wd {
			t.Fatalf("ray %d: hit=%d distance=%.17g; want hit=%d distance=%.17g", i, got, gd, want, wd)
		}
		if packed.blocked(o, d, limit) != (want >= 0) {
			t.Fatalf("ray %d: any-hit differs from nearest-hit", i)
		}
	}
	if n := testing.AllocsPerRun(100, func() { packed.hit(vec{Z: 4}, vec{Z: -1}, 10); packed.blocked(vec{Z: 4}, vec{Z: -1}, 10) }); n != 0 {
		t.Fatalf("ray queries allocated %g times", n)
	}
}

func TestSIMDLeafBoundaries(t *testing.T) {
	for count := 1; count <= 33; count++ {
		a := &accelerator{}
		for i := range count {
			t := triangle{a: vec{Z: float64(i + 1)}, b: vec{X: 1, Z: float64(i + 1)}, c: vec{Y: 1, Z: float64(i + 1)}, surface: i}
			t.bounds = emptyBox().point(t.a).point(t.b).point(t.c)
			a.triangles = append(a.triangles, t)
		}
		a.prepare()
		for _, xy := range []vec{{}, {X: .5, Y: .5}, {X: 1}, {Y: 1}, {X: -.01}} {
			hit, distance := a.hit(xy, vec{Z: 1}, math.Inf(1))
			want := 0
			if xy.X < 0 {
				want = -1
			}
			if hit != want || (hit >= 0 && distance != 1) {
				t.Fatalf("%d triangles at %v: hit=%d distance=%g", count, xy, hit, distance)
			}
			if a.blocked(xy, vec{Z: 1}, 1) {
				t.Fatalf("strict endpoint became a blocker")
			}
		}
	}
	var empty accelerator
	if hit, _ := empty.hit(vec{}, vec{Z: 1}, 1); hit != -1 || empty.blocked(vec{}, vec{Z: 1}, 1) {
		t.Fatal("empty scene blocked ray")
	}
}

func BenchmarkRayTrace(b *testing.B) {
	scalar, packed := rayFixture()
	var origins, directions [256]vec
	rng := random(42)
	for i := range origins {
		origins[i] = vec{X: 24*rng.next() - 12, Y: 24*rng.next() - 12, Z: 6}
		directions[i] = unit(vec{X: rng.next() - .5, Y: rng.next() - .5, Z: -1})
	}
	b.Logf("SIMD width=%d bits, emulated=%v", simd.VectorBitSize(), simd.Emulated())
	for _, mode := range []string{"scalar-nearest", "simd-nearest", "simd-shadow"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			i := 0
			for b.Loop() {
				j := i % len(origins)
				i++
				switch mode {
				case "scalar-nearest":
					scalarHit(scalar, origins[j], directions[j], 20)
				case "simd-nearest":
					packed.hit(origins[j], directions[j], 20)
				case "simd-shadow":
					packed.blocked(origins[j], directions[j], 20)
				}
			}
		})
	}
}
