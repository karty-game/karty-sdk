package worldlightmap

import "math"

// validateLiquidTiles bounds offline work and proves that an imported loading
// unit preserves its complete geometric coverage, not just its outer bounds.
func validateLiquidTiles(selection *SurfaceSelection) error {
	if len(selection.LiquidTiles) == 0 {
		return nil
	}
	if len(selection.LiquidTiles) > MaxLiquidTiles || selection.Clip == nil {
		return ErrLayout
	}
	whole := *selection.Clip
	area := 0.0
	for index, tile := range selection.LiquidTiles {
		clip := tile.Clip
		if tile.LiquidMaxSubdivisions < 0 || tile.LiquidMaxSubdivisions > 32 ||
			clip[0] >= clip[2] || clip[1] >= clip[3] ||
			clip[0] < whole[0] || clip[1] < whole[1] || clip[2] > whole[2] || clip[3] > whole[3] {
			return ErrLayout
		}
		for _, value := range clip {
			if !finite(value) {
				return ErrLayout
			}
			if selection.LiquidEdgeStep > 0 {
				q := value / selection.LiquidEdgeStep
				if math.Abs(q-math.Round(q)) > 1e-7 {
					return ErrLayout
				}
			}
		}
		for _, other := range selection.LiquidTiles[:index] {
			if max(clip[0], other.Clip[0]) < min(clip[2], other.Clip[2]) &&
				max(clip[1], other.Clip[1]) < min(clip[3], other.Clip[3]) {
				return ErrLayout
			}
		}
		area += (clip[2] - clip[0]) * (clip[3] - clip[1])
	}
	expected := (whole[2] - whole[0]) * (whole[3] - whole[1])
	if math.Abs(area-expected) > max(1, expected)*1e-10 {
		return ErrLayout
	}
	return nil
}
