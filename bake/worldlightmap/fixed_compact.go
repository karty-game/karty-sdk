package worldlightmapbake

import (
	"image"
	"math"

	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

// CompactFixedLighting replaces an entire spatial RNM image only if every
// covered pixel in every chart has exactly the same decoded RGBM coefficients
// as all other covered pixels of that chart. The complete ordinary bake is the
// reference; neither four-corner interpolation nor guessed clear-sky lighting is
// used. Any differing shadow, bounced color or uncovered chart retains the image.
// It preserves all bake input identities. Bind its result with NewFixedLighting.
func CompactFixedLighting(
	layout worldlightmap.Layout,
	pixels *image.NRGBA,
	lighting worldlightmap.FixedLighting,
) (worldlightmap.FixedLighting, bool, error) {
	if layout.Selection == nil || layout.Selection.Clip == nil || lighting.Producer != worldlightmap.FixedSpatialProducer {
		return lighting, false, nil
	}
	if len(layout.Pages) != 1 || len(layout.Charts) == 0 || len(layout.Charts) > worldlightmap.MaxCharts ||
		!validCompactPixels(pixels, layout.Pages[0].Width, layout.Pages[0].Height) ||
		lighting.RGBMRange < 1 || math.IsInf(lighting.RGBMRange, 0) || math.IsNaN(lighting.RGBMRange) {
		return lighting, false, worldlightmap.ErrLayout
	}
	width, height := layout.Pages[0].Width, layout.Pages[0].Height
	constants := make([]worldlightmap.ChartRNM, len(layout.Charts))
	for index, chart := range layout.Charts {
		if chart.ID != index || chart.Page != 0 || chart.Rect[0] < 0 || chart.Rect[1] < 0 ||
			chart.Rect[2] > width || chart.Rect[3] > height || chart.Rect[0] >= chart.Rect[2] || chart.Rect[1] >= chart.Rect[3] {
			return lighting, false, worldlightmap.ErrLayout
		}
		products, covered, uniform := constantChartProducts(pixels, width, chart.Rect)
		if !uniform || !covered {
			return lighting, false, nil
		}
		constants[index].Chart = index
		for coefficient, value := range products {
			constants[index].Coefficients[coefficient] = float32(float64(value) * lighting.RGBMRange / (255 * 255))
		}
	}
	lighting.ConstantRNM = constants
	return lighting, true, nil
}

func constantChartProducts(pixels *image.NRGBA, width int, rect [4]int) (reference [9]uint32, covered, uniform bool) {
	for y := rect[1]; y < rect[3]; y++ {
		for x := rect[0]; x < rect[2]; x++ {
			var values [9]uint32
			present := 0
			for basis := range 3 {
				at := pixels.PixOffset(x+basis*width, y)
				alpha := uint32(pixels.Pix[at+3])
				if alpha != 0 {
					present++
				}
				for channel := range 3 {
					values[basis*3+channel] = uint32(pixels.Pix[at+channel]) * alpha
				}
			}
			if present == 0 {
				continue
			}
			if present != 3 || covered && values != reference {
				return reference, covered, false
			}
			reference, covered = values, true
		}
	}
	return reference, covered, true
}

// Validate caller-owned image buffers before any PixOffset indexing. The
// division form bounds storage without overflowing on hostile Stride values.
func validCompactPixels(pixels *image.NRGBA, width, height int) bool {
	if width < 32 || width > 1024 || width&(width-1) != 0 || height != width || pixels == nil ||
		pixels.Bounds() != image.Rect(0, 0, width*3, height) {
		return false
	}
	rowBytes := width * 12
	return pixels.Stride >= rowBytes && len(pixels.Pix) >= rowBytes &&
		height-1 <= (len(pixels.Pix)-rowBytes)/pixels.Stride
}
