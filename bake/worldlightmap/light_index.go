package worldlightmapbake

import (
	"math"
	"math/bits"
	"slices"

	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

// Build-only influence BVH. A query returns candidate sources in the original
// recipe order, retaining deterministic floating-point accumulation and pixels.
// Small lists use the original linear walk rather than paying traversal cost.
type lightIndex struct {
	count  int
	order  []int
	bounds []box
	nodes  []node
}

func (index *lightIndex) prepare(lights []preparedLight) {
	index.count = len(lights)
	if len(lights) <= 32 {
		return
	}
	index.order = make([]int, len(lights))
	index.bounds = make([]box, len(lights))
	for i, light := range lights {
		index.order[i] = i
		lo, hi := sub(
			light.Position,
			vec{X: light.Radius, Y: light.Radius, Z: light.Radius},
		), add(
			light.Position,
			vec{X: light.Radius, Y: light.Radius, Z: light.Radius},
		)
		// Outward rounding keeps boundary candidates conservative.
		index.bounds[i] = box{
			lo: vec{X: math.Nextafter(lo.X, math.Inf(-1)), Y: math.Nextafter(lo.Y, math.Inf(-1)), Z: math.Nextafter(lo.Z, math.Inf(-1))},
			hi: vec{X: math.Nextafter(hi.X, math.Inf(1)), Y: math.Nextafter(hi.Y, math.Inf(1)), Z: math.Nextafter(hi.Z, math.Inf(1))},
		}
	}
	index.build(0, len(index.order))
}

func (index *lightIndex) build(start, end int) int {
	bounds := emptyBox()
	for _, source := range index.order[start:end] {
		bounds = bounds.point(index.bounds[source].lo).point(index.bounds[source].hi)
	}
	current := len(index.nodes)
	index.nodes = append(index.nodes, node{bounds: bounds, start: start, end: end, left: -1, right: -1})
	if end-start <= 8 {
		return current
	}
	extent := sub(bounds.hi, bounds.lo)
	axis := 0
	if extent.Y > extent.X {
		axis = 1
	}
	if extent.Z > component(extent, axis) {
		axis = 2
	}
	slices.SortFunc(index.order[start:end], func(a, b int) int {
		x, y := index.bounds[a], index.bounds[b]
		xCenter := component(x.lo, axis) + component(x.hi, axis)
		yCenter := component(y.lo, axis) + component(y.hi, axis)
		if xCenter < yCenter {
			return -1
		}
		if xCenter > yCenter {
			return 1
		}
		return a - b
	})
	middle := (start + end) / 2
	left, right := index.build(start, middle), index.build(middle, end)
	index.nodes[current].left, index.nodes[current].right = left, right
	return current
}

type lightIterator struct {
	candidates               [worldlightmap.MaxBakeLights / 64]uint64
	linear, end, word, words int
}

func (index *lightIndex) query(position vec, sourceCount int) lightIterator {
	if len(index.nodes) == 0 {
		return lightIterator{end: sourceCount}
	}
	result := lightIterator{linear: -1, words: (index.count + 63) / 64}
	stack := [64]int{0}
	count := 1
	for count > 0 {
		count--
		node := index.nodes[stack[count]]
		if !containsLightPoint(node.bounds, position) {
			continue
		}
		if node.left >= 0 {
			stack[count], stack[count+1] = node.left, node.right
			count += 2
			continue
		}
		for _, source := range index.order[node.start:node.end] {
			if containsLightPoint(index.bounds[source], position) {
				result.candidates[source/64] |= uint64(1) << (source % 64)
			}
		}
	}
	return result
}

func (iterator *lightIterator) next() int {
	if iterator.linear >= 0 {
		if iterator.linear >= iterator.end {
			return -1
		}
		source := iterator.linear
		iterator.linear++
		return source
	}
	for iterator.word < iterator.words {
		mask := iterator.candidates[iterator.word]
		if mask != 0 {
			iterator.candidates[iterator.word] = mask & (mask - 1)
			return iterator.word*64 + bits.TrailingZeros64(mask)
		}
		iterator.word++
	}
	return -1
}

func containsLightPoint(bounds box, point vec) bool {
	return point.X >= bounds.lo.X && point.X <= bounds.hi.X && point.Y >= bounds.lo.Y && point.Y <= bounds.hi.Y && point.Z >= bounds.lo.Z &&
		point.Z <= bounds.hi.Z
}
