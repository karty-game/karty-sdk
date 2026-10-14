package worldlightmapbake

import (
	"fmt"
	"math"
	"testing"
)

// Exhaustive scalar geometry is independent of production construction and
// traversal or box predicates. It pins nearest distance and semantic-surface ties, not an old tree.
type scalarOracle struct{ triangles []triangle }

func newScalarOracle(triangles []triangle) *scalarOracle {
	return &scalarOracle{triangles: append([]triangle(nil), triangles...)}
}

func (a *scalarOracle) hit(o, d vec, limit float64) (int, float64) {
	found, nearest := -1, limit
	for _, triangle := range a.triangles {
		if distance, ok := scalarIntersect(triangle, o, d, limit); ok &&
			(distance < nearest || found >= 0 && distance == nearest && triangle.surface < found) {
			found, nearest = triangle.surface, distance
		}
	}
	return found, nearest
}

func checkScalarRay(t *testing.T, original *scalarOracle, packed *accelerator, o, d vec, limit float64) {
	t.Helper()
	want, wantDistance := original.hit(o, d, limit)
	got, gotDistance := packed.hit(o, d, limit)
	if got != want || gotDistance != wantDistance {
		t.Fatalf("origin=%v direction=%v limit=%.17g: hit %d/%.17g, want %d/%.17g", o, d, limit, got, gotDistance, want, wantDistance)
	}
	if packed.blocked(o, d, limit) != (want >= 0) {
		t.Fatalf("origin=%v direction=%v limit=%.17g: visibility differs from exhaustive scalar intersections", o, d, limit)
	}
}

// Repacking child references and their matching bounds changes traversal order
// without changing geometry or semantic surface IDs. This also exercises box lanes
// with different distance orders; correctness must not rely on builder order.
func permuteWideChildren(a *accelerator, rotate bool) {
	for index := range a.wideNodes {
		n := &a.wideNodes[index]
		count := int(n.count)
		oldChildren := n.children
		var oldBounds [24]float64
		copy(oldBounds[:], a.wideBounds[24*index:24*index+24])
		for child := range count {
			previous := count - 1 - child
			if rotate {
				previous = (child + 1) % count
			}
			n.children[child] = oldChildren[previous]
			for axis := range 6 {
				a.wideBounds[24*index+axis*4+child] = oldBounds[axis*4+previous]
			}
		}
	}
}

func TestBVHGeometryCoplanarAndReorderedChildren(t *testing.T) {
	for _, scaleFactor := range []float64{.001, 1, 100_000} {
		t.Run(fmt.Sprintf("scale=%g", scaleFactor), func(t *testing.T) {
			var triangles []triangle
			appendTriangle := func(a, b, c vec) {
				triangle := triangle{a: scale(a, scaleFactor), b: scale(b, scaleFactor), c: scale(c, scaleFactor), surface: len(triangles)}
				triangle.bounds = emptyBox().point(triangle.a).point(triangle.b).point(triangle.c)
				triangles = append(triangles, triangle)
			}
			// Exact duplicates, ulp-separated planes and shared diagonals carry
			// distinct material IDs. Small leaves can separate these competitors.
			for layer := range 9 {
				z := 2.0
				switch layer % 3 {
				case 1:
					z = math.Nextafter(z, math.Inf(1))
				case 2:
					z += 2e-9
				}
				for cell := range 12 {
					x, y := float64(cell%4)-2, float64(cell/4)-1
					a, b := vec{X: x, Y: y, Z: z}, vec{X: x + 1, Y: y, Z: z}
					c, d := vec{X: x + 1, Y: y + 1, Z: z}, vec{X: x, Y: y + 1, Z: z}
					appendTriangle(a, b, c)
					appendTriangle(a, c, d)
				}
			}
			original := newScalarOracle(triangles)
			packed := &accelerator{triangles: append([]triangle(nil), triangles...)}
			packed.prepare()
			if len(packed.wideNodes) < 2 {
				t.Fatal("fixture does not exercise internal child traversal")
			}
			for permutation := range 3 {
				if permutation > 0 {
					permuteWideChildren(packed, permutation == 2)
				}
				for cell := range 12 {
					x, y := float64(cell%4)-2, float64(cell/4)-1
					for _, offset := range []vec{{}, {X: .5, Y: .5}, {X: 1, Y: 1}, {X: -.5e-9, Y: .5}, {X: .5, Y: .5 + 1e-10}} {
						for _, above := range []bool{false, true} {
							o := scale(vec{X: x + offset.X, Y: y + offset.Y}, scaleFactor)
							d := vec{Z: 1}
							if above {
								o.Z, d.Z = 4*scaleFactor, -1
							}
							for _, limit := range []float64{2 * scaleFactor, math.Nextafter(2*scaleFactor, math.Inf(1)), math.Nextafter(2*scaleFactor, 0), math.Inf(1)} {
								checkScalarRay(t, original, packed, o, d, limit)
							}
							// Include oblique directions and near-parallel components,
							// so ordering cannot assume axis-aligned receiver rays.
							for _, d := range []vec{{X: .1, Y: -.15, Z: 1}, {X: 1e-16, Y: -1e-16, Z: 1}} {
								checkScalarRay(t, original, packed, o, unit(d), math.Inf(1))
							}
						}
					}
				}
			}
		})
	}
}

func TestBVHGeometryLargeRandomScene(t *testing.T) {
	for _, scaleFactor := range []float64{.001, 1, 100_000} {
		t.Run(fmt.Sprintf("scale=%g", scaleFactor), func(t *testing.T) {
			rng := random(990271)
			triangles := make([]triangle, 4097)
			for i := range triangles {
				a := vec{X: 32*rng.next() - 16, Y: 32*rng.next() - 16, Z: 32*rng.next() - 16}
				e1 := vec{X: 4*rng.next() - 2, Y: 4*rng.next() - 2, Z: 4*rng.next() - 2}
				e2 := vec{X: 4*rng.next() - 2, Y: 4*rng.next() - 2, Z: 4*rng.next() - 2}
				if i%97 == 0 {
					e2 = e1 // Degenerate triangles may share SIMD packets with hits.
				}
				triangle := triangle{
					a:       scale(a, scaleFactor),
					b:       scale(add(a, e1), scaleFactor),
					c:       scale(add(a, e2), scaleFactor),
					surface: i,
				}
				triangle.bounds = emptyBox().point(triangle.a).point(triangle.b).point(triangle.c)
				triangles[i] = triangle
			}
			original := newScalarOracle(triangles)
			packed := &accelerator{triangles: append([]triangle(nil), triangles...)}
			packed.prepare()
			for ray := range 300 {
				if ray == 100 || ray == 200 {
					permuteWideChildren(packed, ray == 200)
				}
				o := scale(vec{X: 40*rng.next() - 20, Y: 40*rng.next() - 20, Z: 40*rng.next() - 20}, scaleFactor)
				d := unit(vec{X: 2*rng.next() - 1, Y: 2*rng.next() - 1, Z: 2*rng.next() - 1})
				if ray%4 == 0 {
					triangle := triangles[ray%len(triangles)]
					target := scale(add(add(triangle.a, triangle.b), triangle.c), 1.0/3)
					d = unit(sub(target, o))
				}
				switch ray % 13 {
				case 1:
					d.X = 0
				case 2:
					d.Y = -1e-16
				}
				limit := 40 * scaleFactor * rng.next()
				if ray%3 == 0 {
					limit = math.Inf(1)
				}
				checkScalarRay(t, original, packed, o, d, limit)
				if found, distance := original.hit(o, d, math.Inf(1)); found >= 0 && ray%7 == 0 {
					// Strict upper endpoints and adjacent floating-point values are
					// important when a near hit tightens the remaining box tests.
					for _, endpoint := range []float64{distance, math.Nextafter(distance, 0), math.Nextafter(distance, math.Inf(1))} {
						checkScalarRay(t, original, packed, o, d, endpoint)
					}
				}
			}
		})
	}
}
