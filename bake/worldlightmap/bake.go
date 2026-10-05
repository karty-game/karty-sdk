// Package worldlightmapbake implements deterministic CPU direct and diffuse
// bounced RNM lightmaps. It has no graphics or private engine dependencies.
package worldlightmapbake

import (
	"context"
	"fmt"
	"image"
	"math"
	"runtime"
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
}
type Stats struct {
	ReceiverTexels, Triangles, Workers, Samples, Bounces int
	Rays, ShadowRays, BounceRays                         uint64
	Duration                                             time.Duration
}
type Result struct {
	Image             *image.NRGBA
	RGBMRange         float64
	ReflectanceSHA256 string
	Stats             Stats
}
type receiver struct {
	position vec
	surface  int32
}
type scene struct {
	surfaces  []worldlightmap.Surface
	materials []reflectance
	accel     accelerator
	lights    []world.PointLight
}
type counters struct{ shadow, bounce uint64 }

// Bake returns a complete straight-alpha RGBM atlas with three horizontal
// coefficient tiles. Geometric direct lighting retains the existing neutral
// Half-Lambert response. Reflected transport uses cosine-weighted Lambertian
// sampling and sRGB albedo decoded to linear reflectance bounded to .95.
// Ambient and material AO are omitted and remain live runtime terms.
func Bake(ctx context.Context, document world.Document, layout worldlightmap.Layout, materials []Material, options Options) (Result, error) {
	start := time.Now()
	if ctx == nil {
		return Result{}, fmt.Errorf("nil bake context")
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if options.Samples == 0 {
		options.Samples = 16
	}
	if options.Workers == 0 {
		options.Workers = min(64, runtime.GOMAXPROCS(0))
	}
	if options.Samples < 1 || options.Samples > worldlightmap.MaxOfflineSamples || options.Workers < 1 || options.Workers > 64 || options.Bounces < 0 || options.Bounces > worldlightmap.MaxOfflineBounces {
		return Result{}, fmt.Errorf("offline bake requires samples 1..256, workers 1..64 and bounces 0..4")
	}
	if err := worldlightmap.Validate(&layout, &document); err != nil {
		return Result{}, err
	}
	if layout.RuntimeBake == nil || layout.RuntimeBake.Encoding != worldlightmap.DirectRNMEncoding {
		return Result{}, fmt.Errorf("offline baking requires a direct-rnm3 runtime recipe")
	}
	digest, err := ReflectanceDigest(document, materials)
	if err != nil {
		return Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	images, err := materialImages(materials)
	if err != nil {
		return Result{}, err
	}
	surfaces, err := worldlightmap.Surfaces(&document)
	if err != nil {
		return Result{}, err
	}
	s := scene{surfaces: surfaces, materials: make([]reflectance, len(surfaces))}
	for _, id := range layout.RuntimeBake.LightIDs {
		for _, light := range document.Lighting.Lights {
			if light.ID == id {
				s.lights = append(s.lights, light)
				break
			}
		}
	}
	w, h := layout.Pages[0].Width, layout.Pages[0].Height
	receivers := make([]receiver, w*h)
	for i := range receivers {
		receivers[i].surface = -1
	}
	for i, surface := range surfaces {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		id, uv, origin, tangent, cap := materialOf(&document, surface.Binding)
		s.materials[i] = reflectance{images[id], uv, origin, tangent, cap}
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
					raster(receivers, w, h, layout.Charts[chartID], t)
				}
			}
		}
	}
	if len(s.accel.triangles) > 0 {
		s.accel.build(0, len(s.accel.triangles))
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	rangeValue, err := worldlightmap.OfflineRGBMRange(layout, &document, options.Bounces)
	if err != nil {
		return Result{}, err
	}
	output := image.NewNRGBA(image.Rect(0, 0, 3*w, h))
	stats := Stats{Triangles: len(s.accel.triangles), Workers: options.Workers, Samples: options.Samples, Bounces: options.Bounces}
	for _, r := range receivers {
		if r.surface >= 0 {
			stats.ReceiverTexels++
		}
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
				y := int(next.Add(1) - 1)
				if y >= h || ctx.Err() != nil {
					return
				}
				for x := range w {
					index := y*w + x
					r := receivers[index]
					if r.surface < 0 {
						continue
					}
					if x%16 == 0 && ctx.Err() != nil {
						return
					}
					chart := layout.Charts[layout.Bindings[r.surface].Chart]
					coeff := s.direct(r.position, chart, c)
					if options.Bounces > 0 {
						s.indirect(ctx, r.position, chart, index, options, c, &coeff)
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
	stats.Rays = stats.ShadowRays + stats.BounceRays
	if err := dilate(ctx, output, w, h, layout); err != nil {
		return Result{}, err
	}
	stats.Duration = time.Since(start)
	return Result{Image: output, RGBMRange: rangeValue, ReflectanceSHA256: digest, Stats: stats}, nil
}

func raster(receivers []receiver, w, h int, chart worldlightmap.Chart, t triangle) {
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
			index := y*w + x
			if receivers[index].surface >= 0 {
				continue
			}
			receivers[index] = receiver{add(add(scale(t.a, u), scale(t.b, v)), scale(t.c, 1-u-v)), int32(t.surface)}
		}
	}
}

func (s *scene) visible(p, n, d vec, distance float64, c *counters) bool {
	c.shadow++
	e := epsilon(p)
	hit, _ := s.accel.hit(add(p, scale(n, e)), d, distance-e)
	return hit < 0
}
func (s *scene) direct(p vec, chart worldlightmap.Chart, c *counters) [3]vec {
	var result [3]vec
	for _, l := range s.lights {
		delta := sub(l.Position, p)
		distance := math.Sqrt(dot(delta, delta))
		if distance < epsilon(p) || distance >= l.Radius {
			continue
		}
		d := scale(delta, 1/distance)
		cosine := dot(chart.Normal, d)
		if cosine <= 0 || !s.visible(p, chart.Normal, d, distance, c) {
			continue
		}
		local := vec{X: dot(chart.Tangent, d), Y: dot(chart.Bitangent, d), Z: cosine}
		var weights [3]float64
		sum := 0.0
		for i, basis := range rnmBasis {
			weights[i] = halfLambert(dot(basis, local))
			sum += weights[i]
		}
		attenuation := math.Pow(1-distance*distance/(l.Radius*l.Radius), 2) * 3 * halfLambert(cosine)
		for i := range 3 {
			result[i] = add(result[i], scale(l.Color, attenuation*weights[i]/sum))
		}
	}
	return result
}
func (s *scene) lambertDirect(p, n vec, c *counters) vec {
	result := vec{}
	for _, l := range s.lights {
		delta := sub(l.Position, p)
		distance := math.Sqrt(dot(delta, delta))
		if distance < epsilon(p) || distance >= l.Radius {
			continue
		}
		d := scale(delta, 1/distance)
		cosine := dot(n, d)
		if cosine <= 0 || !s.visible(p, n, d, distance, c) {
			continue
		}
		result = add(result, scale(l.Color, math.Pow(1-distance*distance/(l.Radius*l.Radius), 2)*cosine))
	}
	return result
}
func (s *scene) indirect(ctx context.Context, p vec, chart worldlightmap.Chart, index int, options Options, c *counters, result *[3]vec) {
	rotation := random(options.Seed ^ uint64(index)*0xd6e8feb86659fd93)
	angleOffset := rotation.next()
	for sample := range options.Samples {
		if sample%16 == 0 && ctx.Err() != nil {
			return
		}
		rng := random(options.Seed ^ uint64(index)*0xd6e8feb86659fd93 ^ uint64(sample+1)*0xa0761d6478bd642f)
		// Stratified polar radius plus a deterministic irrational-angle sequence.
		d := hemisphere(chart.Normal, (float64(sample)+.5)/float64(options.Samples), math.Mod(angleOffset+float64(sample)*.6180339887498949, 1))
		local := vec{X: dot(chart.Tangent, d), Y: dot(chart.Bitangent, d), Z: dot(chart.Normal, d)}
		var weights [3]float64
		sum := 0.0
		for i, basis := range rnmBasis {
			weights[i] = math.Pow(math.Max(0, dot(basis, local)), 2)
			sum += weights[i]
		}
		origin := add(p, scale(chart.Normal, epsilon(p)))
		throughput := vec{X: 1, Y: 1, Z: 1}
		incoming := vec{}
		for bounce := range options.Bounces {
			c.bounce++
			hit, distance := s.accel.hit(origin, d, math.Inf(1))
			if hit < 0 {
				break
			}
			n := s.surfaces[hit].Normal
			if dot(n, d) >= 0 {
				break
			}
			position := add(origin, scale(d, distance))
			throughput = mul(throughput, s.materials[hit].sample(position))
			incoming = add(incoming, mul(throughput, s.lambertDirect(position, n, c)))
			if bounce+1 < options.Bounces {
				d = hemisphere(n, rng.next(), rng.next())
				origin = add(position, scale(n, epsilon(position)))
			}
		}
		for i := range 3 {
			result[i] = add(result[i], scale(incoming, 3*weights[i]/(sum*float64(options.Samples))))
		}
	}
}
func encodeRGBM(im *image.NRGBA, x, y int, value vec, rangeValue float64) {
	m := math.Min(1, math.Max(1.0/255, math.Ceil(maximum(value)/rangeValue*255)/255))
	index := im.PixOffset(x, y)
	for channel, v := range [3]float64{value.X, value.Y, value.Z} {
		im.Pix[index+channel] = uint8(math.Floor(math.Min(1, math.Max(0, v/(rangeValue*m)))*255 + .5))
	}
	im.Pix[index+3] = uint8(math.Round(m * 255))
}
func dilate(ctx context.Context, im *image.NRGBA, w, h int, layout worldlightmap.Layout) error {
	distance := make([]uint8, w*h)
	queue := make([]int, 0, w*h)
	for _, chart := range layout.Charts {
		if err := ctx.Err(); err != nil {
			return err
		}
		queue = queue[:0]
		r := chart.Rect
		for y := r[1]; y < r[3]; y++ {
			for x := r[0]; x < r[2]; x++ {
				index := y*w + x
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
			x, y := index%w, index/w
			for _, offset := range [4][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
				x1, y1 := x+offset[0], y+offset[1]
				if x1 < r[0] || x1 >= r[2] || y1 < r[1] || y1 >= r[3] {
					continue
				}
				other := y1*w + x1
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
