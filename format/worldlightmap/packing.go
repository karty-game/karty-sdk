package worldlightmap

import (
	"fmt"
	"slices"
)

// Free rectangles may overlap each other but never any placed chart. Explicit
// bounds keep the two free-rectangle buffers within two MiB on 64-bit targets.
const maxPackingFreeRects = MaxCharts * 4

type packingPosition struct {
	x, y    int
	rotated bool
}

func packCompactRects(bounds []chartBounds, original []int, options Options) ([]packingPosition, error) {
	order := slices.Clone(original)
	slices.SortStableFunc(order, func(a, b int) int {
		aa := (bounds[a].width + 2*options.Padding) * (bounds[a].height + 2*options.Padding)
		bb := (bounds[b].width + 2*options.Padding) * (bounds[b].height + 2*options.Padding)
		return bb - aa
	})
	limit := min(maxPackingFreeRects, len(bounds)*4+1)
	free := make([][4]int, 1, limit)
	free[0] = [4]int{0, 0, options.PageSize, options.PageSize}
	scratch := make([][4]int, 0, limit)
	positions := make([]packingPosition, len(bounds))
	for _, chart := range order {
		width, height := bounds[chart].width+2*options.Padding, bounds[chart].height+2*options.Padding
		chosen, shortSide, longSide := -1, options.PageSize+1, options.PageSize+1
		rotated := false
		for i, rect := range free {
			w, h := rect[2]-rect[0], rect[3]-rect[1]
			for turn := range 2 {
				cw, ch := width, height
				if turn == 1 {
					cw, ch = height, width
				}
				if cw > w || ch > h {
					continue
				}
				a, b := min(w-cw, h-ch), max(w-cw, h-ch)
				if a < shortSide ||
					a == shortSide &&
						(b < longSide || b == longSide && (chosen < 0 || rect[1] < free[chosen][1] || rect[1] == free[chosen][1] && rect[0] < free[chosen][0])) {
					chosen, shortSide, longSide, rotated = i, a, b, turn == 1
				}
			}
		}
		if chosen < 0 {
			return nil, fmt.Errorf("compact one-page lightmap budget exhausted: %w: %w", ErrPageCapacity, ErrLayout)
		}
		x, y := free[chosen][0], free[chosen][1]
		positions[chart] = packingPosition{x, y, rotated}
		if rotated {
			width, height = height, width
		}
		placed := [4]int{x, y, x + width, y + height}
		next := scratch[:0]
		for _, rect := range free {
			if len(next)+4 > limit {
				return nil, fmt.Errorf("compact packing scratch bound: %w: %w", ErrPageCapacity, ErrLayout)
			}
			if rect[0] >= placed[2] || rect[2] <= placed[0] || rect[1] >= placed[3] || rect[3] <= placed[1] {
				next = append(next, rect)
				continue
			}
			if placed[0] > rect[0] {
				next = append(next, [4]int{rect[0], rect[1], placed[0], rect[3]})
			}
			if placed[2] < rect[2] {
				next = append(next, [4]int{placed[2], rect[1], rect[2], rect[3]})
			}
			if placed[1] > rect[1] {
				next = append(next, [4]int{rect[0], rect[1], rect[2], placed[1]})
			}
			if placed[3] < rect[3] {
				next = append(next, [4]int{rect[0], placed[3], rect[2], rect[3]})
			}
			if len(next) > limit {
				return nil, fmt.Errorf("compact packing scratch bound: %w: %w", ErrPageCapacity, ErrLayout)
			}
		}
		free = free[:0]
		for i, rect := range next {
			contained := false
			for j, other := range next {
				if i != j && other[0] <= rect[0] && other[1] <= rect[1] && other[2] >= rect[2] && other[3] >= rect[3] &&
					(other != rect || j < i) {
					contained = true
					break
				}
			}
			if !contained {
				free = append(free, rect)
			}
		}
	}
	return positions, nil
}
