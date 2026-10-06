package worldlightmapbake

import (
	"context"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
	"image"
)

func dilate(ctx context.Context, im *image.NRGBA, w int, layout worldlightmap.Layout) error {
	maximum := 0
	for _, chart := range layout.Charts {
		r := chart.Rect
		maximum = max(maximum, (r[2]-r[0])*(r[3]-r[1]))
	}
	distance := make([]uint8, maximum)
	queue := make([]int, 0, maximum)
	for _, chart := range layout.Charts {
		if err := ctx.Err(); err != nil {
			return err
		}
		queue = queue[:0]
		r := chart.Rect
		stride := r[2] - r[0]
		for y := r[1]; y < r[3]; y++ {
			for x := r[0]; x < r[2]; x++ {
				index := (y-r[1])*stride + x - r[0]
				distance[index] = 255
				if im.Pix[im.PixOffset(x, y)+3] > 0 {
					distance[index] = 0
					queue = append(queue, index)
				}
			}
		}
		for cursor := 0; cursor < len(queue); cursor++ {
			index := queue[cursor]
			if int(distance[index]) >= layout.Padding {
				continue
			}
			x, y := r[0]+index%stride, r[1]+index/stride
			for _, offset := range [4][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
				x1, y1 := x+offset[0], y+offset[1]
				if x1 < r[0] || x1 >= r[2] || y1 < r[1] || y1 >= r[3] {
					continue
				}
				other := (y1-r[1])*stride + x1 - r[0]
				if distance[other] != 255 {
					continue
				}
				distance[other] = distance[index] + 1
				queue = append(queue, other)
				for basis := range 3 {
					from, to := im.PixOffset(x+basis*w, y), im.PixOffset(x1+basis*w, y1)
					copy(im.Pix[to:to+4], im.Pix[from:from+4])
				}
			}
		}
	}
	return nil
}
