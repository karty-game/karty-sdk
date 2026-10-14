package worldlightmapbake

import (
	"math"

	"simd"
)

// Each internal node references up to four descendants of the binary SAH tree.
// Bounds live in six contiguous four-value columns, so one ray can test child
// boxes in SIMD without gathering scalar nodes into temporary packets.
type wideNode struct {
	children [4]int32
	count    int32
}
type wideLeaf struct{ start, end int32 }

// Three siblings per level can remain on the stack. SAH uses at most 24 binary
// levels, then balanced median splits; int32 primitive indices add at most 31
// further levels. Collapsing pairs of levels needs at most 3*28+1 entries.
const wideStackSize = 128

func (a *accelerator) prepareWide() {
	q, l := a.countWides(0)
	a.wideNodes = make([]wideNode, 0, q)
	a.wideLeaves = make([]wideLeaf, 0, l)
	a.wideBounds = make([]float64, 0, 24*q+simd.VectorBitSize()/64-1)
	a.wideRoot = a.buildWide(0)
	a.wideBounds = append(a.wideBounds, make([]float64, simd.VectorBitSize()/64-1)...)
	a.nodes = nil
}

// Negative references encode leaves as -1-index; nonnegative references name
// internal nodes. Construction order retains left-to-right child ordering.
func (a *accelerator) buildWide(index int32) int32 {
	n := a.nodes[index]
	if n.left < 0 {
		ref := -int32(len(a.wideLeaves)) - 1
		a.wideLeaves = append(a.wideLeaves, wideLeaf{n.start, n.end})
		return ref
	}
	ref := int32(len(a.wideNodes))
	a.wideNodes = append(a.wideNodes, wideNode{})
	offset := len(a.wideBounds)
	a.wideBounds = append(a.wideBounds, make([]float64, 24)...)
	children, count := a.wideChildren(n)
	a.wideNodes[ref].count = int32(count)
	for i, child := range children[:count] {
		b := a.nodes[child].bounds
		values := [6]float64{b.lo.X, b.hi.X, b.lo.Y, b.hi.Y, b.lo.Z, b.hi.Z}
		for axis, value := range values {
			a.wideBounds[offset+axis*4+i] = value
		}
		a.wideNodes[ref].children[i] = a.buildWide(child)
	}
	return ref
}

// Preserve division, float64 precision and the scalar slab tolerances. Full
// loads can include columns/children beyond the packet; only real lanes are
// reduced. Global tail padding also makes the final column safe at 512 bits.
func wideMask(bounds []float64, count, width int, o, d vec, limit float64) uint8 {
	var result uint8
	for start := 0; start < count; start += width {
		low, high := simd.BroadcastFloat64s(0), simd.BroadcastFloat64s(limit)
		valid := low.Equal(low)
		{
			lo, hi := simd.LoadFloat64s(bounds[start:]), simd.LoadFloat64s(bounds[4+start:])
			origin := simd.BroadcastFloat64s(o.X)
			if math.Abs(d.X) < 1e-15 {
				e := simd.BroadcastFloat64s(1e-9)
				valid = valid.And(origin.GreaterEqual(lo.Sub(e))).And(origin.LessEqual(hi.Add(e)))
			} else {
				direction := simd.BroadcastFloat64s(d.X)
				x, y := lo.Sub(origin).Div(direction), hi.Sub(origin).Div(direction)
				low = low.Max(x.Min(y))
				high = high.Min(x.Max(y))
			}
		}
		{
			lo, hi := simd.LoadFloat64s(bounds[8+start:]), simd.LoadFloat64s(bounds[12+start:])
			origin := simd.BroadcastFloat64s(o.Y)
			if math.Abs(d.Y) < 1e-15 {
				e := simd.BroadcastFloat64s(1e-9)
				valid = valid.And(origin.GreaterEqual(lo.Sub(e))).And(origin.LessEqual(hi.Add(e)))
			} else {
				direction := simd.BroadcastFloat64s(d.Y)
				x, y := lo.Sub(origin).Div(direction), hi.Sub(origin).Div(direction)
				low = low.Max(x.Min(y))
				high = high.Min(x.Max(y))
			}
		}
		{
			lo, hi := simd.LoadFloat64s(bounds[16+start:]), simd.LoadFloat64s(bounds[20+start:])
			origin := simd.BroadcastFloat64s(o.Z)
			if math.Abs(d.Z) < 1e-15 {
				e := simd.BroadcastFloat64s(1e-9)
				valid = valid.And(origin.GreaterEqual(lo.Sub(e))).And(origin.LessEqual(hi.Add(e)))
			} else {
				direction := simd.BroadcastFloat64s(d.Z)
				x, y := lo.Sub(origin).Div(direction), hi.Sub(origin).Div(direction)
				low = low.Max(x.Min(y))
				high = high.Min(x.Max(y))
			}
		}
		valid = valid.And(high.GreaterEqual(low))
		var bits [8]int64
		valid.ToInt64s().StorePart(bits[:min(width, count-start)])
		for i, bit := range bits[:min(width, count-start)] {
			if bit != 0 {
				result |= 1 << (start + i)
			}
		}
	}
	return result
}

func (a *accelerator) countWides(index int32) (int, int) {
	n := a.nodes[index]
	if n.left < 0 {
		return 0, 1
	}
	q, l := 1, 0
	children, count := a.wideChildren(n)
	for _, child := range children[:count] {
		cq, cl := a.countWides(child)
		q += cq
		l += cl
	}
	return q, l
}
func (a *accelerator) wideChildren(n node) ([4]int32, int) {
	var children [4]int32
	count := 0
	for _, child := range [2]int32{n.left, n.right} {
		c := a.nodes[child]
		if c.left >= 0 {
			children[count], children[count+1] = c.left, c.right
			count += 2
		} else {
			children[count] = child
			count++
		}
	}
	return children, count
}
