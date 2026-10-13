// Package worldlightmapbake implements deterministic CPU direct and diffuse
// bounced RNM lightmaps. It has no graphics or private engine dependencies.
package worldlightmapbake

import (
	"context"
	"fmt"
	"image"
	"sync"
	"sync/atomic"
	"time"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

// Options bounds both work and scratch. Samples defaults to 16 and Workers to
// GOMAXPROCS (at most 64). Bounces is an explicit 0..4; zero is direct-only.
// Seed zero is valid. Sampling is independent of worker count and scheduling.
type Options struct {
	Samples, Bounces, Workers int
	Seed                      uint64
	Denoise                   string  // empty/off preserves original pixels; low or medium filters indirect only
	MaxRayDistance            float64 // zero is unbounded; otherwise each transport segment is clipped
}
type Stats struct {
	ReceiverTexels, Triangles, Workers, Samples, Bounces int
	Rays, ShadowRays, BounceRays                         uint64
	Duration                                             time.Duration
	DenoiseDuration                                      time.Duration
	DenoiseRays                                          uint64
	Denoise                                              string
}
type Result struct {
	Image             *image.NRGBA
	RGBMRange         float64
	ReflectanceSHA256 string
	Stats             Stats
}

// Bake returns a complete straight-alpha RGBM atlas with three horizontal
// coefficient tiles. Geometric direct lighting retains the existing neutral
// Half-Lambert response. Reflected transport uses cosine-weighted Lambertian
// sampling and sRGB albedo decoded to linear reflectance bounded to .95.
// Ambient and material AO are omitted and remain live runtime terms.
func Bake(
	ctx context.Context,
	document world.Document,
	layout worldlightmap.Layout,
	materials []Material,
	options Options,
) (Result, error) {
	return bake(ctx, document, layout, materials, options, nil)
}

func bake(ctx context.Context, document world.Document, layout worldlightmap.Layout,
	materials []Material, options Options, environment *world.ResolvedEnvironment) (Result, error) {
	start := time.Now()
	if ctx == nil {
		return Result{}, fmt.Errorf("nil bake context")
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	var inputs bakeInputs
	var err error
	if environment == nil {
		inputs, err = prepareBakeInputs(ctx, document, layout, materials, options)
	} else {
		inputs, err = prepareEnvironmentInputs(ctx, document, layout, materials, options, *environment)
	}
	if err != nil {
		return Result{}, err
	}
	options = inputs.options
	rangeValue, images, surfaces := inputs.rangeValue, inputs.images, inputs.surfaces
	digest, lightIDs := inputs.reflectance, inputs.lightIDs
	s := scene{
		surfaces:       surfaces,
		materials:      make([]reflectance, len(surfaces)),
		maxRayDistance: options.MaxRayDistance,
		environment:    environment,
	}
	if options.Bounces > 0 || environment != nil {
		s.samples = initialSamples(options.Samples)
		s.surfaceFrames = make([]sampleFrame, len(surfaces))
		for i, surface := range surfaces {
			s.surfaceFrames[i] = newSampleFrame(surface.Normal)
		}
		s.chartFrames = make([]sampleFrame, len(layout.Charts))
		for i, chart := range layout.Charts {
			s.chartFrames[i] = newSampleFrame(chart.Normal)
		}
	}
	sources := make(map[string]world.PointLight)
	if document.Lighting != nil {
		for _, light := range document.Lighting.Lights {
			sources[light.ID] = light
		}
	}
	for _, id := range lightIDs {
		light := sources[id]
		s.lights = append(s.lights, preparedLight{light, light.Radius * light.Radius})
	}
	s.lightIndex.prepare(s.lights)
	w, h := layout.Pages[0].Width, layout.Pages[0].Height
	grids, rows := receiverGrids(layout.Charts)
	albedos := make(map[uint32]*image.NRGBA)
	for i, surface := range surfaces {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		id, uv, origin, tangent, reflectanceCap := materialOf(&document, surface.Binding)
		r := reflectance{mapping: uv, origin: origin, tangent: tangent, cap: reflectanceCap}
		if options.Bounces > 0 && len(surface.Polygons) > 0 {
			pixels, ok := albedos[id]
			if !ok {
				pixels, err = prepareAlbedo(ctx, images[id])
				if err != nil {
					return Result{}, err
				}
				albedos[id] = pixels
			}
			r.image = pixels
			r.width, r.height = float64(pixels.Rect.Dx()), float64(pixels.Rect.Dy())
		}
		s.materials[i] = r
		chartID := layout.Bindings[i].Chart
		for _, polygon := range surface.Polygons {
			for j := 1; j+1 < len(polygon); j++ {
				t := triangle{a: polygon[0], b: polygon[j], c: polygon[j+1], surface: i}
				if dot(cross(sub(t.b, t.a), sub(t.c, t.a)), cross(sub(t.b, t.a), sub(t.c, t.a))) < 1e-28 {
					continue
				}
				t.bounds = emptyBox().point(t.a).point(t.b).point(t.c)
				s.accel.triangles = append(s.accel.triangles, t)
				if chartID >= 0 {
					raster(grids[chartID], w, h, layout.Charts[chartID], t)
				}
			}
		}
	}
	triangleCount := len(s.accel.triangles)
	s.accel.prepare()
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	output := image.NewNRGBA(image.Rect(0, 0, 3*w, h))
	stats := Stats{Triangles: triangleCount, Workers: options.Workers, Samples: options.Samples, Bounces: options.Bounces}
	for _, grid := range grids {
		for _, r := range grid.pixels {
			if r.surface >= 0 {
				stats.ReceiverTexels++
			}
		}
	}
	filtering := (options.Bounces > 0 || environment != nil) && options.Denoise != "" && options.Denoise != "off"
	var lighting [][]filterPixel
	if filtering {
		lighting = make([][]filterPixel, len(grids))
		for i, grid := range grids {
			lighting[i] = make([]filterPixel, len(grid.pixels))
		}
	}
	stats.Denoise = options.Denoise
	if stats.Denoise == "" {
		stats.Denoise = "off"
	}
	var next atomic.Int64
	var group sync.WaitGroup
	counts := make([]counters, options.Workers)
	for worker := range options.Workers {
		group.Add(1)
		go func(worker int) {
			defer group.Done()
			c := &counts[worker]
			for {
				rowIndex := int(next.Add(1) - 1)
				if rowIndex >= len(rows) || ctx.Err() != nil {
					return
				}
				row := rows[rowIndex]
				grid := grids[row.chart]
				y := row.y
				chart := layout.Charts[row.chart]
				for x := grid.rect[0]; x < grid.rect[2]; x++ {
					index := y*w + x
					r := grid.pixels[grid.offset(x, y)]
					if r.surface < 0 {
						continue
					}
					if x%16 == 0 && ctx.Err() != nil {
						return
					}
					coeff := s.direct(r.position, chart, c)
					if filtering {
						var indirect [3]vec
						variance := s.indirect(ctx, r.position, chart, s.chartFrames[row.chart], index, options, c, &indirect)
						lighting[row.chart][grid.offset(x, y)] = filterPixel{
							direct:   packCoefficients(coeff),
							indirect: packCoefficients(indirect),
							variance: variance,
						}
						continue
					}
					if options.Bounces > 0 || environment != nil {
						s.indirect(ctx, r.position, chart, s.chartFrames[row.chart], index, options, c, &coeff)
					}
					for basis := range 3 {
						encodeRGBM(output, x+basis*w, y, coeff[basis], rangeValue)
					}
				}
			}
		}(worker)
	}
	group.Wait()
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	for _, c := range counts {
		stats.ShadowRays += c.shadow
		stats.BounceRays += c.bounce
	}
	if filtering {
		filterStart := time.Now()
		rays, err := s.denoise(ctx, grids, layout, lighting, output, rangeValue, options)
		if err != nil {
			return Result{}, err
		}
		stats.DenoiseRays = rays
		stats.DenoiseDuration = time.Since(filterStart)
	}
	stats.Rays = stats.ShadowRays + stats.BounceRays + stats.DenoiseRays
	if err := dilate(ctx, output, w, layout); err != nil {
		return Result{}, err
	}
	stats.Duration = time.Since(start)
	return Result{Image: output, RGBMRange: rangeValue, ReflectanceSHA256: digest, Stats: stats}, nil
}
