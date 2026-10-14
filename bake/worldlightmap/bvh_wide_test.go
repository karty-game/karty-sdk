package worldlightmapbake

import (
	"math"
	"testing"

	"simd"
)

// Compare slab predicates at grazing endpoints, near-parallel directions and
// coordinate scales beyond the world limit; these errors can cause light leaks.
func TestWideBoxPredicates(t *testing.T) {
	rng := random(8281)
	width := simd.VectorBitSize() / 64
	bounds := make([]float64, 24+width-1)
	for i := 0; i < 75000; i++ {
		scale := math.Pow(10, float64(i%19-9))
		var boxes [4]box
		o := vec{X: (rng.next()*4 - 2) * scale, Y: (rng.next()*4 - 2) * scale, Z: (rng.next()*4 - 2) * scale}
		hi := vec{X: (rng.next()*2 - 1) * scale, Y: (rng.next()*2 - 1) * scale, Z: (rng.next()*2 - 1) * scale}
		d := unit(sub(hi, o))
		if i%9 == 0 {
			d.X = 0
		}
		if i%9 == 1 {
			d.Y = math.SmallestNonzeroFloat64
		}
		if i%9 == 2 {
			d.Z = -1e-16
		}
		limit := scale * rng.next() * 4
		if i%3 == 0 {
			limit = (hi.Z - o.Z) / d.Z
		}
		if i%3 == 1 {
			limit = math.Inf(1)
		}
		switch i % 7 {
		case 0:
			limit = math.Nextafter(limit, math.Inf(1))
		case 1:
			limit = math.Nextafter(limit, math.Inf(-1))
		}
		for j := range boxes {
			lo := sub(hi, vec{X: rng.next() * scale, Y: rng.next() * scale, Z: rng.next() * scale})
			boxes[j] = box{lo: lo, hi: hi}
			values := [6]float64{lo.X, hi.X, lo.Y, hi.Y, lo.Z, hi.Z}
			for axis, value := range values {
				bounds[axis*4+j] = value
			}
		}
		mask := wideMask(bounds, 4, width, o, d, limit)
		for j, b := range boxes {
			got, want := mask&(1<<j) != 0, scalarBoxRay(b, o, d, limit)
			if got != want {
				t.Fatalf("%d/%d: %v origin %v dir %v limit %.17g: %v vs %v", i, j, b, o, d, limit, got, want)
			}
		}
	}
}

func TestWideTailAndFlatBounds(t *testing.T) {
	width := simd.VectorBitSize() / 64
	bounds := make([]float64, 24+width-1)
	boxes := [4]box{
		{lo: vec{Z: 1}, hi: vec{X: 1, Y: 1, Z: 1}},
		{lo: vec{X: 2, Z: 1}, hi: vec{X: 3, Y: 1, Z: 2}},
		{lo: vec{X: -1, Y: -1}, hi: vec{X: 1, Y: 1}},
		{lo: vec{X: 1_000_000, Y: 1_000_000, Z: 1_000_000}, hi: vec{X: 1_000_000, Y: 1_000_000, Z: 1_000_000}},
	}
	for child, b := range boxes {
		for axis, value := range [6]float64{b.lo.X, b.hi.X, b.lo.Y, b.hi.Y, b.lo.Z, b.hi.Z} {
			bounds[axis*4+child] = value
		}
	}
	for i := 24; i < len(bounds); i++ {
		bounds[i] = math.NaN()
	}
	rays := [][2]vec{
		{{X: .5, Y: .5}, {Z: 1}},
		{{X: .5, Y: .5, Z: 1}, {X: 1}},
		{{}, {Y: -1e-16, Z: 1}},
		{{X: math.Nextafter(1, 2), Y: .5}, {Z: 1}},
		{{X: 1_000_000, Y: 1_000_000, Z: 1_000_001}, {Z: -1}},
		{{X: -1, Y: -1, Z: -1}, {X: 1, Y: 1, Z: 1}},
	}
	for count := 1; count <= 4; count++ {
		for _, ray := range rays {
			for _, limit := range []float64{0, 1, math.Nextafter(1, 0), math.Nextafter(1, 2), math.Inf(1)} {
				var want uint8
				for child, b := range boxes[:count] {
					if scalarBoxRay(b, ray[0], ray[1], limit) {
						want |= 1 << child
					}
				}
				if got := wideMask(bounds, count, width, ray[0], ray[1], limit); got != want {
					t.Fatalf("%d boxes, ray %v, limit %g: mask %04b, want %04b", count, ray, limit, got, want)
				}
			}
		}
	}
}

func TestWideStorageAndTraversalStack(t *testing.T) {
	_, packed := rayFixture()
	if packed.nodes != nil || packed.triangles != nil {
		t.Fatal("wide preparation retained binary construction data")
	}
	if cap(packed.wideNodes) != len(packed.wideNodes) || cap(packed.wideLeaves) != len(packed.wideLeaves) ||
		cap(packed.wideBounds) != len(packed.wideBounds) {
		t.Fatal("wide preparation reserved excess retained storage")
	}
	if want := 24*len(packed.wideNodes) + simd.VectorBitSize()/64 - 1; len(packed.wideBounds) != want {
		t.Fatalf("%d bound values, want %d including full-width tail", len(packed.wideBounds), want)
	}
	// Model the deepest possible construction tree: each pair of binary levels
	// leaves three siblings pending while traversal descends into the fourth.
	// Even the int32 index bound, beyond valid world's geometry limits, fits.
	var a accelerator
	var build func(int) int32
	leaf := func() int32 {
		index := int32(len(a.nodes))
		a.nodes = append(a.nodes, node{start: 0, end: 1, left: -1, right: -1})
		return index
	}
	branch := func() int32 {
		index := int32(len(a.nodes))
		a.nodes = append(a.nodes, node{})
		return index
	}
	build = func(levels int) int32 {
		if levels == 0 {
			return leaf()
		}
		root, left, right := branch(), branch(), branch()
		a.nodes[root].left, a.nodes[root].right = left, right
		a.nodes[left].left, a.nodes[left].right = leaf(), leaf()
		a.nodes[right].left, a.nodes[right].right = leaf(), build(levels-1)
		return root
	}
	levels := (bvhSAHDepth + 31 + 1) / 2
	build(levels)
	a.prepareWide()
	stack := [wideStackSize]int32{a.wideRoot}
	count, maximum, leaves := 1, 1, 0
	for count > 0 {
		count--
		ref := stack[count]
		if ref < 0 {
			leaves++
			continue
		}
		n := a.wideNodes[ref]
		for _, child := range n.children[:n.count] {
			if count == len(stack) {
				t.Fatal("adversarial wide traversal exceeded fixed stack")
			}
			stack[count] = child
			count++
		}
		maximum = max(maximum, count)
	}
	if want := 3*levels + 1; maximum != want || leaves != want {
		t.Fatalf("deep traversal peak=%d, leaves=%d; want %d", maximum, leaves, want)
	}
}
