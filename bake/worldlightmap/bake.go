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
	"github.com/karty-game/karty-sdk/format/worldterrain"
)

// Options bounds both work and scratch. Samples defaults to 16 and Workers to
// GOMAXPROCS (at most 64). Bounces is an explicit 0..4; zero is direct-only.
// Seed zero is valid. Sampling is independent of worker count and scheduling.
type Options struct {
	// Terrain is full offline transport; TerrainReceivers is the chunk crop.
	Terrain          *worldterrain.Document
	TerrainReceivers *worldterrain.Document
	// Vegetation contributes static rest-pose cutouts to this same bake.
	Vegetation                *Vegetation
	fixed                     bool
	Samples, Bounces, Workers int
	Seed                      uint64
	Denoise                   string  // empty/off preserves original pixels; low or medium filters indirect only
	MaxRayDistance            float64 // zero is unbounded; otherwise each transport segment is clipped
	// ShadowTriangles are optional build-only, double-sided opaque absorbing
	// geometry. They never receive atlas space or contribute reflectance.
	// Keep the slice unchanged during the call; at most MaxShadowTriangles.
	ShadowTriangles [][3]world.Vec3
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
	return bake(ctx, document, layout, materials, options, nil, nil)
}

func bake(ctx context.Context, document world.Document, layout worldlightmap.Layout,
	materials []Material, options Options, environment *world.ResolvedEnvironment, shared *Baker) (Result, error) {
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
	rangeValue, surfaces := inputs.rangeValue, inputs.surfaces
	if inputs.receivers != nil {
		surfaces = inputs.receivers
	}
	prepared, err := shared.transport(ctx, &document, inputs, options.Bounces > 0 || environment != nil)
	if err != nil {
		return Result{}, err
	}
	digest, lightIDs := inputs.reflectance, inputs.lightIDs
	s := scene{
		surfaces:       prepared.surfaces,
		accel:          prepared.accel,
		surfaceFrames:  prepared.surfaceFrames,
		materials:      prepared.materials,
		maxRayDistance: options.MaxRayDistance,
		environment:    environment,
	}
	if options.fixed {
		s.fixedFill = document.Lighting
	}
	if options.Bounces > 0 || environment != nil {
		s.samples = initialSamples(options.Samples)
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
	grids, tiles := receiverGrids(layout.Charts)
	for i, surface := range surfaces {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		chartID := layout.Bindings[i].Chart
		if chartID < 0 {
			continue
		}
		for _, polygon := range surface.Polygons {
			for j := 1; j+1 < len(polygon); j++ {
				t := triangle{a: polygon[0], b: polygon[j], c: polygon[j+1], surface: i}
				if !degenerate(t) {
					raster(grids[chartID], w, h, layout.Charts[chartID], t)
				}
			}
		}
	}

	output := image.NewNRGBA(image.Rect(0, 0, 3*w, h))
	stats := Stats{Triangles: prepared.triangles, Workers: options.Workers, Samples: options.Samples, Bounces: options.Bounces}
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
			var local counters
			defer func() { counts[worker] = local }()
			c := &local
			for {
				tileIndex := int(next.Add(1) - 1)
				if tileIndex >= len(tiles) || ctx.Err() != nil {
					return
				}
				tile := tiles[tileIndex]
				grid := grids[tile.chart]
				chart := layout.Charts[tile.chart]
				for y := tile.rect[1]; y < tile.rect[3]; y++ {
					for x := tile.rect[0]; x < tile.rect[2]; x++ {
						index := y*w + x
						r := grid.pixels[grid.offset(x, y)]
						if r.surface < 0 {
							continue
						}
						if x%16 == 0 && ctx.Err() != nil {
							return
						}
						if layout.Selection != nil && layout.Selection.Clip != nil {
							index = terrainSampleIndex(r.position, chart.Normal)
						}
						coeff := s.direct(r.position, chart, c)
						if filtering {
							var indirect [3]vec
							variance := s.indirect(ctx, r.position, chart, s.chartFrames[tile.chart], index, options, c, &indirect)
							lighting[tile.chart][grid.offset(x, y)] = filterPixel{
								direct:   packCoefficients(coeff),
								indirect: packCoefficients(indirect),
								variance: variance,
							}
							continue
						}
						if options.Bounces > 0 || environment != nil {
							s.indirect(ctx, r.position, chart, s.chartFrames[tile.chart], index, options, c, &coeff)
						}
						for basis := range 3 {
							encodeRGBM(output, x+basis*w, y, coeff[basis], rangeValue)
						}
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
