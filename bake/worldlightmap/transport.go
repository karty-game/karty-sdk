package worldlightmapbake

import (
	"context"
	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
	"math"
)

type scene struct {
	surfaces      []worldlightmap.Surface
	materials     []reflectance
	accel         accelerator
	lights        []preparedLight
	surfaceFrames []sampleFrame
	chartFrames   []sampleFrame
	samples       []initialSample
}
type preparedLight struct {
	world.PointLight
	radiusSquared float64
}

type counters struct{ shadow, bounce uint64 }

func (s *scene) visible(p, n, d vec, distance float64, c *counters) bool {
	c.shadow++
	e := epsilon(p)
	return !s.accel.blocked(add(p, scale(n, e)), d, distance-e)
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
		attenuation := square(1-distance*distance/l.radiusSquared) * 3 * halfLambert(cosine)
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
		result = add(result, scale(l.Color, square(1-distance*distance/l.radiusSquared)*cosine))
	}
	return result
}
func (s *scene) indirect(ctx context.Context, p vec, chart worldlightmap.Chart, sampling sampleFrame, index int, options Options, c *counters, result *[3]vec) float32 {
	mean, moment := 0.0, 0.0
	measure := options.Denoise != "" && options.Denoise != "off"
	seed := options.Seed ^ uint64(index)*0xd6e8feb86659fd93
	rotation := random(seed)
	angleOffset := rotation.next()
	initialOrigin := add(p, scale(chart.Normal, epsilon(p)))
	for sample, precomputed := range s.samples {
		if sample%16 == 0 && ctx.Err() != nil {
			return 0
		}
		rng := random(seed ^ precomputed.seed)
		// Stratified polar radius plus a deterministic irrational-angle sequence.
		d := sampling.direction(chart.Normal, precomputed.radius, precomputed.height, math.Mod(angleOffset+precomputed.angle, 1))
		local := vec{X: dot(chart.Tangent, d), Y: dot(chart.Bitangent, d), Z: dot(chart.Normal, d)}
		var weights [3]float64
		sum := 0.0
		for i, basis := range rnmBasis {
			weights[i] = square(math.Max(0, dot(basis, local)))
			sum += weights[i]
		}
		origin := initialOrigin
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
				d = s.surfaceFrames[hit].hemisphere(n, rng.next(), rng.next())
				origin = add(position, scale(n, epsilon(position)))
			}
		}
		if measure {
			value := .2126*incoming.X + .7152*incoming.Y + .0722*incoming.Z
			delta := value - mean
			mean += delta / float64(sample+1)
			moment += delta * (value - mean)
		}
		for i := range 3 {
			result[i] = add(result[i], scale(incoming, 3*weights[i]/(sum*float64(options.Samples))))
		}
	}
	if options.Samples > 1 {
		return float32(moment / float64(options.Samples*(options.Samples-1)))
	}
	return float32(mean * mean)
}
