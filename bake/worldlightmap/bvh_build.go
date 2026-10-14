package worldlightmapbake

import "math"

const (
	bvhBins = 16
	// Median splits below this depth keep the fixed traversal stack bounded
	// even for adversarial surface-area distributions.
	bvhSAHDepth = 24
)

type bvhBin struct {
	bounds box
	count  int
}

func (b box) area() float64 {
	d := sub(b.hi, b.lo)
	return d.X*d.Y + d.X*d.Z + d.Y*d.Z
}

func mergeBounds(a, b box) box { return a.point(b.lo).point(b.hi) }

// The triangle kernel admits barycentrics just outside an edge. A tight SAH
// box must include that existing tolerance rather than depending on a large
// median leaf's accidental union with neighbouring geometry.
func (b box) conservative() box {
	d := scale(sub(b.hi, b.lo), 3e-9)
	e := epsilon(b.lo) + epsilon(b.hi)
	d = add(d, vec{X: e, Y: e, Z: e})
	return box{lo: sub(b.lo, d), hi: add(b.hi, d)}
}

func triangleBin(t triangle, axis int, low, span float64) int {
	centre := component(t.bounds.lo, axis) + component(t.bounds.hi, axis)
	return min(bvhBins-1, max(0, int((centre-low)/span*bvhBins)))
}

// Binned surface-area splits reduce overlap without changing triangle math.
// Stable partitioning uses one temporary construction buffer, released after
// packing. Degenerate/too-deep partitions use the balanced median builder.
func (a *accelerator) buildSAH(start, end, depth int, scratch []triangle) int {
	if end-start <= 2 || depth >= bvhSAHDepth {
		return a.build(start, end)
	}
	// Split small leaves that span surfaces, but keep a surface's triangle fan
	// together. Unconditionally shrinking every leaf adds build/node cost on
	// dense solid geometry without improving its full bake time.
	if end-start <= 8 {
		surface := a.triangles[start].surface
		sameSurface := true
		for _, t := range a.triangles[start+1 : end] {
			if t.surface != surface {
				sameSurface = false
				break
			}
		}
		if sameSurface {
			return a.build(start, end)
		}
	}
	bounds := emptyBox()
	centres := emptyBox()
	for _, t := range a.triangles[start:end] {
		bounds = mergeBounds(bounds, t.bounds)
		centres = centres.point(add(t.bounds.lo, t.bounds.hi))
	}
	best, axis, split := math.Inf(1), -1, 0
	for candidate := range 3 {
		low := component(centres.lo, candidate)
		span := component(centres.hi, candidate) - low
		if span <= 0 {
			continue
		}
		var bins [bvhBins]bvhBin
		for _, t := range a.triangles[start:end] {
			bin := &bins[triangleBin(t, candidate, low, span)]
			if bin.count == 0 {
				bin.bounds = t.bounds
			} else {
				bin.bounds = mergeBounds(bin.bounds, t.bounds)
			}
			bin.count++
		}
		var right [bvhBins]bvhBin
		accumulated := bvhBin{bounds: emptyBox()}
		for i := bvhBins - 1; i >= 0; i-- {
			if bins[i].count > 0 {
				accumulated.bounds = mergeBounds(accumulated.bounds, bins[i].bounds)
				accumulated.count += bins[i].count
			}
			right[i] = accumulated
		}
		left := bvhBin{bounds: emptyBox()}
		for i := 0; i < bvhBins-1; i++ {
			if bins[i].count > 0 {
				left.bounds = mergeBounds(left.bounds, bins[i].bounds)
				left.count += bins[i].count
			}
			r := right[i+1]
			if left.count == 0 || r.count == 0 {
				continue
			}
			cost := left.bounds.area()*float64(left.count) + r.bounds.area()*float64(r.count)
			if cost < best {
				best, axis, split = cost, candidate, i
			}
		}
	}
	if axis < 0 {
		return a.build(start, end)
	}
	low := component(centres.lo, axis)
	span := component(centres.hi, axis) - low
	middle := start
	for _, t := range a.triangles[start:end] {
		if triangleBin(t, axis, low, span) <= split {
			scratch[middle] = t
			middle++
		}
	}
	position := middle
	for _, t := range a.triangles[start:end] {
		if triangleBin(t, axis, low, span) > split {
			scratch[position] = t
			position++
		}
	}
	copy(a.triangles[start:end], scratch[start:end])
	index := len(a.nodes)
	a.nodes = append(a.nodes, node{bounds: bounds, start: int32(start), end: int32(end), left: -1, right: -1})
	left := a.buildSAH(start, middle, depth+1, scratch)
	right := a.buildSAH(middle, end, depth+1, scratch)
	a.nodes[index].left, a.nodes[index].right = int32(left), int32(right)
	return index
}
