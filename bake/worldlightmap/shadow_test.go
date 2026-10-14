package worldlightmapbake

import (
	"math"
	"testing"
)

// At a large dominant coordinate, the normal offset changes epsilon even
// though the shadow ray travels along another axis. Reusing epsilon(p) as the
// intersection guard accepts a blocker that the original shifted-origin ray
// rejects. The same distinction exists across epsilon's minimum-value boundary.
func TestShadowReceiverOffsetGuard(t *testing.T) {
	for _, p := range []vec{{X: 1e12}, {X: -1e12}, {X: math.Nextafter(10_000, 0)}} {
		n := vec{X: 1}
		if p.X < 0 {
			n.X = -1
		}
		offset := epsilon(p)
		origin := add(p, scale(n, offset))
		guard := epsilon(origin)
		if guard <= offset {
			t.Fatalf("fixture at %v did not change shifted-origin epsilon: offset %.17g, guard %.17g", p, offset, guard)
		}
		// This plane lies above the unshifted minimum but below the shifted
		// minimum. Keep the triangle interior far from its barycentric edges.
		z := (offset*.25 + guard*.25) * .5
		if !(z > offset*.25 && z < guard*.25) {
			t.Fatalf("fixture cannot distinguish guards %.17g and %.17g", offset, guard)
		}
		tri := triangle{
			a:       vec{X: origin.X - 2, Y: -2, Z: z},
			b:       vec{X: origin.X + 2, Y: -2, Z: z},
			c:       vec{X: origin.X, Y: 2, Z: z},
			surface: 0,
		}
		tri.bounds = emptyBox().point(tri.a).point(tri.b).point(tri.c)
		triangles := []triangle{tri}
		original := newScalarOracle(triangles)
		s := scene{accel: accelerator{triangles: append([]triangle(nil), triangles...)}}
		s.accel.prepare()
		d := vec{Z: 1}
		if hit, _ := original.hit(origin, d, 10-offset); hit >= 0 {
			t.Fatal("original shifted-origin guard accepted the near plane")
		}
		var c counters
		if !s.visible(p, n, d, 10, &c) {
			t.Fatalf("offset origin %v must reject plane %.17g under guard %.17g", origin, z, guard)
		}
		if c.shadow != 1 || c.bounce != 0 {
			t.Fatalf("visibility counters = %+v, want one shadow ray", c)
		}
		// The point-light loop passes the receiver's epsilon once for all its
		// light queries. The kernel must still derive its shifted-origin guard.
		c = counters{}
		if !s.visibleEpsilon(p, n, d, 10, offset, &c) || c.shadow != 1 {
			t.Fatal("shared receiver epsilon changed near-plane visibility or counters")
		}
	}
}

func TestShadowVisibilityBoundariesAndPacketTails(t *testing.T) {
	for _, coordinate := range []float64{0, 10_000, 1e12} {
		p := vec{X: coordinate}
		n := vec{X: 1}
		offset := epsilon(p)
		origin := add(p, scale(n, offset))
		guard := epsilon(origin)
		for _, gap := range []float64{guard * .25, math.Nextafter(guard*.25, math.Inf(1)), guard, 4 * guard, 10 * guard} {
			for _, count := range []int{1, 2, 7, 8, 17} {
				triangles := make([]triangle, count)
				for i := range triangles {
					// Duplicate faces test partial SIMD packets and leaf tails;
					// different material IDs must not affect opaque visibility.
					tri := triangle{
						a:       vec{X: origin.X - 2, Y: -2, Z: gap},
						b:       vec{X: origin.X + 2, Y: -2, Z: gap},
						c:       vec{X: origin.X, Y: 2, Z: gap},
						surface: i,
					}
					tri.bounds = emptyBox().point(tri.a).point(tri.b).point(tri.c)
					triangles[i] = tri
				}
				original := newScalarOracle(triangles)
				s := scene{accel: accelerator{triangles: append([]triangle(nil), triangles...)}}
				s.accel.prepare()
				for _, receiver := range []vec{p, add(p, vec{Y: 2}), add(p, vec{Y: math.Nextafter(2, math.Inf(1))}), add(p, vec{Y: -3})} {
					o := add(receiver, scale(n, epsilon(receiver)))
					for _, limit := range []float64{gap, math.Nextafter(gap, 0), math.Nextafter(gap, math.Inf(1)), gap + guard*.5, math.Inf(1)} {
						want, _ := original.hit(o, vec{Z: 1}, limit)
						if got := s.accel.blocked(o, vec{Z: 1}, limit); got != (want >= 0) {
							t.Fatalf(
								"coordinate=%g gap=%.17g triangles=%d origin=%v limit=%.17g: blocked=%v want=%v",
								coordinate,
								gap,
								count,
								o,
								limit,
								got,
								want >= 0,
							)
						}
						// The visible wrapper shortens the finite ray by epsilon of
						// the receiver. Evaluate the subtraction in its original
						// order, including cancellation near the strict endpoint.
						lightDistance := limit + epsilon(receiver)
						want, _ = original.hit(o, vec{Z: 1}, lightDistance-epsilon(receiver))
						var c counters
						if got := s.visible(receiver, n, vec{Z: 1}, lightDistance, &c); got != (want < 0) {
							t.Fatalf(
								"coordinate=%g gap=%.17g triangles=%d receiver=%v distance=%.17g: visible=%v want=%v",
								coordinate,
								gap,
								count,
								receiver,
								lightDistance,
								got,
								want < 0,
							)
						}
						if c.shadow != 1 {
							t.Fatalf("visibility counted %d shadow rays, want 1", c.shadow)
						}
						c = counters{}
						if got := s.visibleEpsilon(
							receiver,
							n,
							vec{Z: 1},
							lightDistance,
							epsilon(receiver),
							&c,
						); got != (want < 0) ||
							c.shadow != 1 {
							t.Fatalf("shared epsilon changed visibility/counters: visible=%v want=%v counters=%+v", got, want < 0, c)
						}
					}
				}
			}
		}
	}
	var empty scene
	var c counters
	if !empty.visible(vec{}, vec{Z: 1}, vec{Z: 1}, math.Inf(1), &c) || c.shadow != 1 {
		t.Fatal("empty accelerator must pass visibility and count the query")
	}
}
