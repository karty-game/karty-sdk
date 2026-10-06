package worldlightmapbake

import (
	"context"
	"image"
	"math"
	"sync"
	"sync/atomic"

	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

// Float buffers exist only for filtered bakes. At most MaxTexels entries are
// retained (76 bytes each); scratch is 37 bytes per chart texel being processed.
// RNM bases share weights, preserving the directional relationship of the lobes.
type coefficients [9]float32
type filterPixel struct {
	direct, indirect coefficients
	variance         float32 // estimated variance of the sample mean, in linear radiance
}

func packCoefficients(v [3]vec) (out coefficients) {
	for i, c := range v {
		out[i*3], out[i*3+1], out[i*3+2] = float32(c.X), float32(c.Y), float32(c.Z)
	}
	return
}
func (v coefficients) colour() [3]float64 {
	return [3]float64{float64(v[0]+v[3]+v[6]) / 3, float64(v[1]+v[4]+v[7]) / 3, float64(v[2]+v[5]+v[8]) / 3}
}

func (s *scene) denoise(
	ctx context.Context,
	grids []receiverGrid,
	layout worldlightmap.Layout,
	pixels [][]filterPixel,
	output *image.NRGBA,
	rgbmRange float64,
	options Options,
) (uint64, error) {
	var next atomic.Int64
	var rays atomic.Uint64
	var group sync.WaitGroup
	// Each chart has disjoint output, and scratch across all workers is bounded
	// by total receiver storage, regardless of configured worker count.
	for range min(options.Workers, len(grids)) {
		group.Go(func() {
			for {
				index := int(next.Add(1) - 1)
				if index >= len(grids) || ctx.Err() != nil {
					return
				}
				grid, chart := grids[index], layout.Charts[index]
				data := pixels[index]
				edges := make([]uint8, len(data))
				width := grid.rect[2] - grid.rect[0]
				// Short visibility segments prevent smoothing through walls/solid bases
				// even when a large coplanar floor chart covers both sides of an obstacle.
				for y := grid.rect[1]; y < grid.rect[3]; y++ {
					if ctx.Err() != nil {
						return
					}
					for x := grid.rect[0]; x < grid.rect[2]; x++ {
						a := grid.offset(x, y)
						if grid.pixels[a].surface < 0 {
							continue
						}
						for axis, delta := range [2][2]int{{1, 0}, {0, 1}} {
							nx, ny := x+delta[0], y+delta[1]
							if nx >= grid.rect[2] || ny >= grid.rect[3] {
								continue
							}
							b := grid.offset(nx, ny)
							if grid.pixels[b].surface < 0 {
								continue
							}
							p, q := grid.pixels[a].position, grid.pixels[b].position
							direction := sub(q, p)
							distance := math.Sqrt(dot(direction, direction))
							if distance == 0 {
								continue
							}
							e := max(epsilon(p), epsilon(q)) * 4
							blocked := s.accel.blocked(add(p, scale(chart.Normal, e)), scale(direction, 1/distance), distance)
							rays.Add(1)
							if !blocked {
								edges[a] |= 1 << axis
							}
						}
					}
				}
				scratch := make([]coefficients, len(data))
				passes := 2
				if options.Denoise == "medium" {
					passes = 3
				}
				kernel := [5]float64{1, 4, 6, 4, 1}
				for pass := 0; pass < passes; pass++ {
					step := 1 << pass
					for y := 0; y < grid.rect[3]-grid.rect[1]; y++ {
						if ctx.Err() != nil {
							return
						}
						for x := 0; x < width; x++ {
							a := y*width + x
							if grid.pixels[a].surface < 0 {
								continue
							}
							centre := data[a].indirect.colour()
							var sum [9]float64
							weightSum := 0.0
							for ky := -2; ky <= 2; ky++ {
								for kx := -2; kx <= 2; kx++ {
									nx, ny := x+kx*step, y+ky*step
									if nx < 0 || nx >= width || ny < 0 || ny >= grid.rect[3]-grid.rect[1] ||
										!connected(edges, width, x, y, nx, ny) {
										continue
									}
									b := ny*width + nx
									other := data[b].indirect.colour()
									difference := 0.0
									luminance := 0.0
									for c := range 3 {
										difference += (centre[c] - other[c]) * (centre[c] - other[c]) / 3
										luminance += math.Abs(centre[c]) / 3
									}
									// A small relative floor avoids freezing residual noise after early
									// passes. The variance guide stays unfiltered to avoid runaway blur.
									sigma := 3*math.Sqrt(float64(data[a].variance)+float64(data[b].variance)) + .02*luminance + 1e-6
									weight := kernel[kx+2] * kernel[ky+2] * math.Exp(-difference/(sigma*sigma))
									for c := range 9 {
										sum[c] += weight * float64(data[b].indirect[c])
									}
									weightSum += weight
								}
							}
							for c := range 9 {
								scratch[a][c] = float32(sum[c] / weightSum)
							}
						}
					}
					for i := range data {
						data[i].indirect, scratch[i] = scratch[i], data[i].indirect
					}
				}
				w := layout.Pages[0].Width
				for y := grid.rect[1]; y < grid.rect[3]; y++ {
					if ctx.Err() != nil {
						return
					}
					for x := grid.rect[0]; x < grid.rect[2]; x++ {
						a := grid.offset(x, y)
						if grid.pixels[a].surface < 0 {
							continue
						}
						for basis := range 3 {
							i := basis * 3
							value := vec{
								X: float64(data[a].direct[i]) + float64(data[a].indirect[i]),
								Y: float64(data[a].direct[i+1]) + float64(data[a].indirect[i+1]),
								Z: float64(data[a].direct[i+2]) + float64(data[a].indirect[i+2]),
							}
							encodeRGBM(output, x+basis*w, y, value, rgbmRange)
						}
					}
				}
			}
		})
	}
	group.Wait()
	return rays.Load(), ctx.Err()
}

// Walk the sample segment over precomputed adjacent visibility edges. Diagonal
// crossings require both sides of the cell, conservatively keeping holes and
// thin obstacles from being skipped by the wider à-trous passes.
func connected(edges []uint8, width, x, y, nx, ny int) bool {
	dx, dy := absInt(nx-x), absInt(ny-y)
	sx, sy := 1, 1
	if nx < x {
		sx = -1
	}
	if ny < y {
		sy = -1
	}
	err := dx - dy
	for x != nx || y != ny {
		e := 2 * err
		ax, ay := x, y
		if e > -dy {
			err -= dy
			ax += sx
		}
		if e < dx {
			err += dx
			ay += sy
		}
		if ax != x && (!edgeOpen(edges, width, x, y, ax, y) || (ay != y && !edgeOpen(edges, width, x, ay, ax, ay))) {
			return false
		}
		if ay != y && (!edgeOpen(edges, width, x, y, x, ay) || (ax != x && !edgeOpen(edges, width, ax, y, ax, ay))) {
			return false
		}
		x, y = ax, ay
	}
	return true
}
func edgeOpen(edges []uint8, width, x, y, nx, ny int) bool {
	if x != nx {
		return edges[y*width+min(x, nx)]&1 != 0
	}
	return edges[min(y, ny)*width+x]&2 != 0
}
func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
