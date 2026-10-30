package worldlightmapbake

import (
	"math"
	"sort"

	"simd"
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

type node struct {
	bounds                  box
	start, end, left, right int32
}
type accelerator struct {
	triangles  []triangle
	nodes      []node // Binary construction data, released after wide packing.
	wideRoot   int32
	wideNodes  []wideNode
	wideBounds []float64
	wideLeaves []wideLeaf
	// Nine contiguous columns: vertex A and the two precomputed triangle edges.
	columns         [9][]float64
	surfaces        []int32
	vegetationStart int
	vegetation      []vegetationReflectance
}

func (a *accelerator) build(start, end int) int {
	b := emptyBox()
	for _, t := range a.triangles[start:end] {
		b = b.point(t.bounds.lo).point(t.bounds.hi)
	}
	index := len(a.nodes)
	a.nodes = append(a.nodes, node{bounds: b, start: int32(start), end: int32(end), left: -1, right: -1})
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
	a.nodes[index].left = int32(left)
	a.nodes[index].right = int32(right)
	return index
}

// prepare retains only the BVH and packed triangle data after construction.
func (a *accelerator) prepare() {
	if len(a.triangles) == 0 {
		return
	}
	a.buildSAH(0, len(a.triangles), 0, make([]triangle, len(a.triangles)))
	for i := range a.nodes {
		a.nodes[i].bounds = a.nodes[i].bounds.conservative()
	}
	// A full vector load may read past a leaf, but only its real lanes are used.
	// The final padding also makes the last leaf safe at any supported width.
	stride := len(a.triangles) + simd.VectorBitSize()/64 - 1
	data := make([]float64, 9*stride)
	for i := range a.columns {
		a.columns[i] = data[i*stride : (i+1)*stride]
	}
	a.surfaces = make([]int32, len(a.triangles))
	for i, t := range a.triangles {
		e1, e2 := sub(t.b, t.a), sub(t.c, t.a)
		values := [9]float64{t.a.X, t.a.Y, t.a.Z, e1.X, e1.Y, e1.Z, e2.X, e2.Y, e2.Z}
		for j, value := range values {
			a.columns[j][i] = value
		}
		a.surfaces[i] = int32(t.surface)
	}
	a.prepareWide()
	a.triangles = nil
}

// intersections tests one ray against a vector of double-sided triangles.
// Keep float64, the scalar producer's operation order, and explicit Mul/Add
// (rather than fused MulAdd) so native and emulated producers agree.
// The caller's slice bounds exclude padding and triangles in the next leaf.
func intersections(columns *[9][]float64, start int, out []float64, o, d vec, limit, minimum float64) (int, bool) {
	ax := simd.LoadFloat64s(columns[0][start:])
	ay := simd.LoadFloat64s(columns[1][start:])
	az := simd.LoadFloat64s(columns[2][start:])
	ex := simd.LoadFloat64s(columns[3][start:])
	ey := simd.LoadFloat64s(columns[4][start:])
	ez := simd.LoadFloat64s(columns[5][start:])
	fx := simd.LoadFloat64s(columns[6][start:])
	fy := simd.LoadFloat64s(columns[7][start:])
	fz := simd.LoadFloat64s(columns[8][start:])
	dx, dy, dz := simd.BroadcastFloat64s(d.X), simd.BroadcastFloat64s(d.Y), simd.BroadcastFloat64s(d.Z)
	px := dy.Mul(fz).Sub(dz.Mul(fy))
	py := dz.Mul(fx).Sub(dx.Mul(fz))
	pz := dx.Mul(fy).Sub(dy.Mul(fx))
	det := ex.Mul(px).Add(ey.Mul(py)).Add(ez.Mul(pz))
	valid := det.Abs().GreaterEqual(simd.BroadcastFloat64s(1e-15))
	lanes := min(len(out), det.Len())
	if !anyLane(valid, lanes) {
		return lanes, false
	}
	sx := simd.BroadcastFloat64s(o.X).Sub(ax)
	sy := simd.BroadcastFloat64s(o.Y).Sub(ay)
	sz := simd.BroadcastFloat64s(o.Z).Sub(az)
	u := sx.Mul(px).Add(sy.Mul(py)).Add(sz.Mul(pz)).Div(det)
	lower, upper := simd.BroadcastFloat64s(-1e-9), simd.BroadcastFloat64s(1+1e-9)
	valid = valid.And(u.GreaterEqual(lower)).And(u.LessEqual(upper))
	if !anyLane(valid, lanes) {
		return lanes, false
	}
	qx := sy.Mul(ez).Sub(sz.Mul(ey))
	qy := sz.Mul(ex).Sub(sx.Mul(ez))
	qz := sx.Mul(ey).Sub(sy.Mul(ex))
	v := dx.Mul(qx).Add(dy.Mul(qy)).Add(dz.Mul(qz)).Div(det)
	valid = valid.And(v.GreaterEqual(lower)).And(u.Add(v).LessEqual(upper))
	if !anyLane(valid, lanes) {
		return lanes, false
	}
	distance := fx.Mul(qx).Add(fy.Mul(qy)).Add(fz.Mul(qz)).Div(det)
	ceiling := simd.BroadcastFloat64s(limit)
	valid = valid.And(distance.Greater(simd.BroadcastFloat64s(minimum))).And(distance.Less(ceiling))
	return distance.IfElse(valid, ceiling).StorePart(out), true
}

// Portable masks have no reduction operation yet. Store only real lanes,
// excluding padding and the next leaf, and reduce without allocating.
func anyLane(mask simd.Mask64s, count int) bool {
	var bits [8]int64
	mask.ToInt64s().StorePart(bits[:count])
	for _, bit := range bits[:count] {
		if bit != 0 {
			return true
		}
	}
	return false
}

func (a *accelerator) hit(o, d vec, limit float64) (int, float64) {
	return trace(a, o, d, limit, false)
}

func (a *accelerator) blocked(o, d vec, limit float64) bool {
	_, distance := trace(a, o, d, limit, true)
	return distance < limit
}

// Nearest hits serve diffuse paths; visibility rays stop at their first blocker.
// SIMD processes contiguous child boxes and triangle leaf arithmetic.
func trace(a *accelerator, o, d vec, limit float64, anyHit bool) (int, float64) {
	maximumDistance := limit
	// Dispatch once per ray rather than once per packet.
	var vector simd.Float64s
	width := vector.Len()
	if len(a.wideLeaves) == 0 {
		return -1, limit
	}
	stack := [wideStackSize]int32{a.wideRoot}
	count, found := 1, -1
	hasHit := false
	minimum := epsilon(o) * .25
	var distances [8]float64 // A leaf has at most eight real triangles.
	for count > 0 {
		count--
		ref := stack[count]
		ceiling := limit
		if hasHit {
			// Include exact equal-distance hits for a stable semantic-surface tie,
			// while retaining the caller's strict upper endpoint.
			ceiling = min(maximumDistance, math.Nextafter(limit, math.Inf(1)))
		}
		if ref >= 0 {
			n := a.wideNodes[ref]
			mask := wideMask(a.wideBounds[24*int(ref):], int(n.count), width, o, d, ceiling)
			for child := 0; child < int(n.count); child++ {
				if mask&(1<<child) != 0 {
					stack[count] = n.children[child]
					count++
				}
			}
			continue
		}
		n := a.wideLeaves[-ref-1]
		for start := int(n.start); start < int(n.end); {
			lanes, possible := intersections(&a.columns, start, distances[:min(width, int(n.end)-start)], o, d, ceiling, minimum)
			if possible {
				for i, distance := range distances[:lanes] {
					if distance >= ceiling {
						continue
					}
					surface := int(a.surfaces[start+i])
					if len(a.vegetation) > 0 && surface >= a.vegetationStart &&
						!a.vegetation[surface-a.vegetationStart].covered(add(o, scale(d, distance))) {
						continue
					}
					if distance < limit || hasHit && distance == limit && surface < found {
						limit, found = distance, surface
						hasHit = true
						if anyHit {
							return found, limit
						}
					}
				}
			}
			start += lanes
		}
	}
	return found, limit
}
