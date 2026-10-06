package worldlightmapbake

import (
	"image"
	"math"
)

func encodeRGBM(im *image.NRGBA, x, y int, value vec, rangeValue float64) {
	m := math.Min(1, math.Max(1.0/255, math.Ceil(maximum(value)/rangeValue*255)/255))
	index := im.PixOffset(x, y)
	for channel, v := range [3]float64{value.X, value.Y, value.Z} {
		im.Pix[index+channel] = uint8(math.Floor(math.Min(1, math.Max(0, v/(rangeValue*m)))*255 + .5))
	}
	im.Pix[index+3] = uint8(math.Round(m * 255))
}
