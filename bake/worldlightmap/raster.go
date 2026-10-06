package worldlightmapbake

import (
	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
	"math"
)

type receiver struct {
	position vec
	surface  int32
}

type receiverGrid struct {
	rect   [4]int
	pixels []receiver
}

type receiverRow struct{ chart, y int }

func (g receiverGrid) offset(x, y int) int {
	return (y-g.rect[1])*(g.rect[2]-g.rect[0]) + x - g.rect[0]
}

// Layout validation guarantees disjoint bounded rectangles. Keep positions only
// inside receiver rectangles; unused atlas space needs no per-texel scratch.
func receiverGrids(charts []worldlightmap.Chart) ([]receiverGrid, []receiverRow) {
	texels, rowCount := 0, 0
	for _, chart := range charts {
		r := chart.ReceiverRect
		texels += (r[2] - r[0]) * (r[3] - r[1])
		rowCount += r[3] - r[1]
	}
	pixels := make([]receiver, texels)
	for i := range pixels {
		pixels[i].surface = -1
	}
	grids := make([]receiverGrid, len(charts))
	rows := make([]receiverRow, 0, rowCount)
	offset := 0
	for i, chart := range charts {
		r := chart.ReceiverRect
		count := (r[2] - r[0]) * (r[3] - r[1])
		grids[i] = receiverGrid{rect: r, pixels: pixels[offset : offset+count]}
		offset += count
		for y := r[1]; y < r[3]; y++ {
			rows = append(rows, receiverRow{chart: i, y: y})
		}
	}
	return grids, rows
}

func raster(grid receiverGrid, w, h int, chart worldlightmap.Chart, t triangle) {
	uv := func(p vec) world.Vec2 {
		return world.Vec2{X: float64(w) * (chart.UPlane[0]*p.X + chart.UPlane[1]*p.Y + chart.UPlane[2]*p.Z + chart.UPlane[3]), Y: float64(h) * (chart.VPlane[0]*p.X + chart.VPlane[1]*p.Y + chart.VPlane[2]*p.Z + chart.VPlane[3])}
	}
	a, b, c := uv(t.a), uv(t.b), uv(t.c)
	den := (b.Y-c.Y)*(a.X-c.X) + (c.X-b.X)*(a.Y-c.Y)
	if math.Abs(den) < 1e-15 {
		return
	}
	r := chart.ReceiverRect
	// Chart vertices intentionally lie on texel centres. Include their boundary
	// samples despite tiny affine evaluation roundoff; barycentrics still test
	// actual polygon coverage and the receiver rectangle remains authoritative.
	x0, x1 := max(r[0], int(math.Ceil(math.Min(a.X, math.Min(b.X, c.X))-.5-1e-8))), min(r[2]-1, int(math.Floor(math.Max(a.X, math.Max(b.X, c.X))-.5+1e-8)))
	y0, y1 := max(r[1], int(math.Ceil(math.Min(a.Y, math.Min(b.Y, c.Y))-.5-1e-8))), min(r[3]-1, int(math.Floor(math.Max(a.Y, math.Max(b.Y, c.Y))-.5+1e-8)))
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			u := ((b.Y-c.Y)*(float64(x)+.5-c.X) + (c.X-b.X)*(float64(y)+.5-c.Y)) / den
			v := ((c.Y-a.Y)*(float64(x)+.5-c.X) + (a.X-c.X)*(float64(y)+.5-c.Y)) / den
			if u < -1e-9 || v < -1e-9 || u+v > 1+1e-9 {
				continue
			}
			index := grid.offset(x, y)
			if grid.pixels[index].surface >= 0 {
				continue
			}
			grid.pixels[index] = receiver{add(add(scale(t.a, u), scale(t.b, v)), scale(t.c, 1-u-v)), int32(t.surface)}
		}
	}
}
