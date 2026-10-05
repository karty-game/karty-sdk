package worldlightmapbake

import (
	"math"
	"sort"
)

type triangle struct {
	a, b, c vec
	surface int
	bounds  box
}
type box struct{ lo, hi vec }

func emptyBox() box {
	return box{vec{X: math.Inf(1), Y: math.Inf(1), Z: math.Inf(1)}, vec{X: math.Inf(-1), Y: math.Inf(-1), Z: math.Inf(-1)}}
}
func (b box) point(p vec) box {
	b.lo = vec{X: math.Min(b.lo.X, p.X), Y: math.Min(b.lo.Y, p.Y), Z: math.Min(b.lo.Z, p.Z)}
	b.hi = vec{X: math.Max(b.hi.X, p.X), Y: math.Max(b.hi.Y, p.Y), Z: math.Max(b.hi.Z, p.Z)}
	return b
}
func (b box) ray(o, d vec, limit float64) bool {
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

type node struct {
	bounds                  box
	start, end, left, right int
}
type accelerator struct {
	triangles []triangle
	nodes     []node
}

func (a *accelerator) build(start, end int) int {
	b := emptyBox()
	for _, t := range a.triangles[start:end] {
		b = b.point(t.bounds.lo).point(t.bounds.hi)
	}
	index := len(a.nodes)
	a.nodes = append(a.nodes, node{bounds: b, start: start, end: end, left: -1, right: -1})
	if end-start <= 8 {
		return index
	}
	extent := sub(b.hi, b.lo)
	axis := 0
	if extent.Y > extent.X {
		axis = 1
	}
	if component(extent, 2) > component(extent, axis) {
		axis = 2
	}
	sort.SliceStable(a.triangles[start:end], func(i, j int) bool {
		x, y := a.triangles[start+i].bounds, a.triangles[start+j].bounds
		return component(add(x.lo, x.hi), axis) < component(add(y.lo, y.hi), axis)
	})
	middle := (start + end) / 2
	left := a.build(start, middle)
	right := a.build(middle, end)
	a.nodes[index].left = left
	a.nodes[index].right = right
	return index
}

// intersect is double-sided: a back-facing wall still blocks light.
func intersect(t triangle, o, d vec, limit float64) (float64, bool) {
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
func (a *accelerator) hit(o, d vec, limit float64) (int, float64) {
	if len(a.nodes) == 0 {
		return -1, limit
	}
	stack := [64]int{0}
	count := 1
	found := -1
	for count > 0 {
		count--
		n := a.nodes[stack[count]]
		if !n.bounds.ray(o, d, limit) {
			continue
		}
		if n.left >= 0 {
			stack[count] = n.left
			stack[count+1] = n.right
			count += 2
			continue
		}
		for _, t := range a.triangles[n.start:n.end] {
			if distance, ok := intersect(t, o, d, limit); ok {
				limit = distance
				found = t.surface
			}
		}
	}
	return found, limit
}
