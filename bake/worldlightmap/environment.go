package worldlightmapbake

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"slices"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
	"github.com/karty-game/karty-sdk/internal/binwire"
)

const EnvironmentProducer = worldlightmap.EnvironmentPrebakeProducer

// BakeEnvironment produces only static directional sun and sky diffuse. Point
// lights, authored ambient, specular and actors remain runtime contributions.
// Every variant uses the caller's same receiver layout. Sky illumination traces
// actual openings; ordinary walls and solids block both sun and sky rays.
// CloudAmount linearly attenuates the sun between clear and overcast endpoints.
// MaxRayDistance must be zero: a truncated ray cannot establish sky visibility.
func BakeEnvironment(ctx context.Context, document world.Document, layout worldlightmap.Layout,
	materials []Material, settings world.EnvironmentSettings, options Options) (Result, error) {
	environment, err := world.ResolveEnvironmentSettings(document.Environment, settings)
	if err != nil {
		return Result{}, err
	}
	return bake(ctx, document, layout, materials, options, &environment, nil)
}

func prepareEnvironmentInputs(ctx context.Context, document world.Document, layout worldlightmap.Layout,
	materials []Material, options Options, environment world.ResolvedEnvironment) (bakeInputs, error) {
	var err error
	options, err = normalizeOptions(options)
	if err != nil || options.MaxRayDistance != 0 {
		return bakeInputs{}, fmt.Errorf("environment bake requires samples 1..256, workers 1..64, bounces 0..4 and unbounded rays")
	}
	if _, err := worldlightmap.OfflineDenoiseProducer(options.Denoise); err != nil {
		return bakeInputs{}, err
	}
	if layout.RuntimeBake != nil && !options.fixed {
		return bakeInputs{}, fmt.Errorf("environment variants require a layout without a point-light recipe")
	}
	if err := worldlightmap.Validate(&layout, &document); err != nil {
		return bakeInputs{}, err
	}
	selection := layout.Receivers
	if selection == nil {
		selection = &worldlightmap.ReceiverSelection{Sectors: make([]int, len(document.Sectors))}
		for i := range selection.Sectors {
			selection.Sectors[i] = i
		}
	}
	sectors, solids, err := worldlightmap.ReceiverScope(&document, selection)
	if err != nil {
		return bakeInputs{}, err
	}
	images, err := materialImages(materials)
	if err != nil {
		return bakeInputs{}, err
	}
	if err := validateVegetationForBake(&document, options, images); err != nil {
		return bakeInputs{}, err
	}
	surfaces, err := worldlightmap.Surfaces(&document)
	if err != nil {
		return bakeInputs{}, err
	}
	transparent := transparentEnvironmentMaterials(&document)
	applyReceiverScope(surfaces, sectors, solids)
	if err := prepareFixedTerrain(document, options, surfaces); err != nil {
		return bakeInputs{}, err
	}
	var receivers []worldlightmap.Surface
	if options.fixed && (len(transparent) > 0 || layout.Selection != nil) {
		// Copy headers only: removing transport polygons below must not remove
		// fixed water/glass receivers. Geometry backing stays immutable/shared.
		receivers = slices.Clone(surfaces)
	}
	if layout.Selection != nil {
		if err := worldlightmap.ApplySurfaceSelection(receivers, layout.Selection); err != nil {
			return bakeInputs{}, err
		}
	}
	for i := range surfaces {
		binding := surfaces[i].Binding
		material, _, _, _, _ := materialOf(&document, binding)
		if transparent[material] {
			surfaces[i].Polygons = nil
		}
	}
	digest, err := reflectanceDigest(ctx, &document, surfaces, images)
	if err != nil {
		return bakeInputs{}, err
	}
	digest, err = vegetationReflectanceDigest(digest, options.Vegetation)
	if err != nil {
		return bakeInputs{}, err
	}
	if options.Terrain != nil {
		terrainDigest, terrainErr := terrainInputDigest(options.Terrain)
		if terrainErr != nil {
			return bakeInputs{}, terrainErr
		}
		hash := sha256.Sum256([]byte("fixed-terrain-reflectance@1:" + digest + ":" + terrainDigest))
		digest = hex.EncodeToString(hash[:])
	}
	var rangeValue float64
	var lightIDs []string
	if options.fixed {
		if layout.RuntimeBake != nil {
			if layout.RuntimeBake.Encoding != worldlightmap.DirectRNMEncoding {
				return bakeInputs{}, worldlightmap.ErrLayout
			}
			lightIDs = layout.RuntimeBake.LightIDs
		}
		rangeValue, err = fixedRGBMRange(document, layout, environment, options)
	} else {
		rangeValue, err = worldlightmap.EnvironmentRGBMRange(
			document.Environment,
			world.EnvironmentSettings{TimeOfDay: environment.TimeOfDay, CloudAmount: environment.CloudAmount},
			options.Bounces,
		)
	}
	if err != nil {
		return bakeInputs{}, err
	}
	return bakeInputs{options: options, rangeValue: rangeValue, images: images,
		surfaces: surfaces, receivers: receivers, lightIDs: lightIDs, reflectance: digest}, nil
}

// EnvironmentReflectanceDigest returns the scoped transport reflectance shared
// by every clock/weather variant of this receiver layout.
func EnvironmentReflectanceDigest(document world.Document, layout worldlightmap.Layout, materials []Material) (string, error) {
	environment, err := world.ResolveEnvironmentSettings(document.Environment, world.EnvironmentSettings{TimeOfDay: 15})
	if err != nil {
		return "", err
	}
	inputs, err := prepareEnvironmentInputs(context.Background(), document, layout, materials, Options{Samples: 1, Workers: 1}, environment)
	if err != nil {
		return "", err
	}
	return inputs.reflectance, nil
}

func transparentEnvironmentMaterials(document *world.Document) map[uint32]bool {
	result := make(map[uint32]bool)
	if document.Animations == nil {
		return result
	}
	presets := make(map[string]bool)
	for _, preset := range document.Animations.Presets {
		presets[preset.Name] = preset.Kind == "liquid" && preset.Opacity != nil && *preset.Opacity < 1
	}
	for _, binding := range document.Animations.Materials {
		if presets[binding.Preset] {
			result[binding.Material] = true
		}
	}
	return result
}

// EnvironmentInputDigest binds the complete static transport, original used
// albedo pixels, fixed chart coordinates, sampling and resolved time/weather.
// Workers, actors and authored point lights do not affect these pixels.
func EnvironmentInputDigest(document world.Document, layout worldlightmap.Layout, materials []Material,
	settings world.EnvironmentSettings, options Options) (string, error) {
	environment, err := world.ResolveEnvironmentSettings(document.Environment, settings)
	if err != nil {
		return "", err
	}
	inputs, err := prepareEnvironmentInputs(context.Background(), document, layout, materials, options, environment)
	if err != nil {
		return "", err
	}
	surfaceDigest, err := worldlightmap.EnvironmentSurfaceDigest(&document)
	if err != nil {
		return "", err
	}
	layoutDigest, err := worldlightmap.LayoutDigest(&layout)
	if err != nil {
		return "", err
	}
	w := binwire.NewWriter("KEI1", 2048+MaxShadowTriangles*72)
	w.String(EnvironmentProducer, 128)
	w.String(surfaceDigest, 64)
	w.String(layoutDigest, 64)
	w.String(inputs.reflectance, 64)
	w.I32(inputs.options.Samples)
	w.I32(inputs.options.Bounces)
	w.U64(inputs.options.Seed)
	w.String(inputs.options.Denoise, 16)
	w.F64(settings.TimeOfDay)
	w.F64(settings.CloudAmount)
	w.F64(inputs.rangeValue)
	for _, value := range [...]vec{environment.SunDirection, environment.SunColor, environment.Ambient} {
		writeInputVector(w, value)
	}
	w.F64(environment.SunIntensity)
	writeShadowTriangles(w, inputs.options.ShadowTriangles)
	if err := writeVegetationInput(w, inputs.options.Vegetation); err != nil {
		return "", err
	}
	encoded, err := w.Bytes()
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func (s *scene) environmentSun(p vec, chart worldlightmap.Chart, c *counters) [3]vec {
	var result [3]vec
	e := s.environment
	cosine := dot(chart.Normal, e.SunDirection)
	if e.SunIntensity == 0 || e.CloudAmount == 1 || cosine <= 0 || !s.visible(p, chart.Normal, e.SunDirection, math.Inf(1), c) {
		return result
	}
	local := vec{X: dot(chart.Tangent, e.SunDirection), Y: dot(chart.Bitangent, e.SunDirection), Z: cosine}
	var weights [3]float64
	sum := 0.0
	for i, basis := range rnmBasis {
		weights[i] = halfLambert(dot(basis, local))
		sum += weights[i]
	}
	for i := range result {
		result[i] = scale(e.SunColor, e.SunIntensity*(1-e.CloudAmount)*3*halfLambert(cosine)*weights[i]/sum)
	}
	return result
}

func (s *scene) skyRadiance(direction vec) vec {
	cube := s.environment.AmbientCube
	x, y, z := cube.PositiveX, cube.PositiveY, cube.PositiveZ
	if direction.X < 0 {
		x = cube.NegativeX
	}
	if direction.Y < 0 {
		y = cube.NegativeY
	}
	if direction.Z < 0 {
		z = cube.NegativeZ
	}
	return add(add(scale(x, square(direction.X)), scale(y, square(direction.Y))), scale(z, square(direction.Z)))
}

func (s *scene) environmentIndirect(ctx context.Context, p vec, chart worldlightmap.Chart, sampling sampleFrame,
	index int, options Options, c *counters, result *[3]vec) float32 {
	mean, moment := 0.0, 0.0
	seed := options.Seed ^ uint64(index)*0xd6e8feb86659fd93
	rngRotation := random(seed)
	angleOffset := rngRotation.next()
	for sample, precomputed := range s.samples {
		if sample%16 == 0 && ctx.Err() != nil {
			return 0
		}
		rng := random(seed ^ precomputed.seed)
		direction := sampling.direction(chart.Normal, precomputed.radius, precomputed.height, math.Mod(angleOffset+precomputed.angle, 1))
		local := vec{X: dot(chart.Tangent, direction), Y: dot(chart.Bitangent, direction), Z: dot(chart.Normal, direction)}
		var weights [3]float64
		sum := 0.0
		for i, basis := range rnmBasis {
			weights[i] = square(max(0, dot(basis, local)))
			sum += weights[i]
		}
		origin := add(p, scale(chart.Normal, epsilon(p)))
		throughput, incoming := vec{X: 1, Y: 1, Z: 1}, vec{}
		for bounce := 0; bounce <= options.Bounces; bounce++ {
			c.bounce++
			hit, distance := s.accel.hit(origin, direction, math.Inf(1))
			if hit < 0 {
				// A negative surface with finite distance is an absorbing
				// shadow proxy, not an escape to the sky.
				if math.IsInf(distance, 1) {
					incoming = add(incoming, mul(throughput, s.skyRadiance(direction)))
				}
				break
			}
			if bounce == options.Bounces {
				break
			}
			normal := s.surfaces[hit].Normal
			if s.materials[hit].vegetation != nil && dot(normal, direction) >= 0 {
				normal = scale(normal, -1)
			}
			if dot(normal, direction) >= 0 {
				break
			}
			position := add(origin, scale(direction, distance))
			throughput = mul(throughput, s.materials[hit].sample(position))
			if maximum(throughput) == 0 {
				break
			}
			incoming = add(incoming, mul(throughput, s.lambertDirect(position, normal, c)))
			direction = s.surfaceFrames[hit].hemisphere(normal, rng.next(), rng.next())
			origin = add(position, scale(normal, epsilon(position)))
		}
		value := .2126*incoming.X + .7152*incoming.Y + .0722*incoming.Z
		delta := value - mean
		mean += delta / float64(sample+1)
		moment += delta * (value - mean)
		for i := range result {
			result[i] = add(result[i], scale(incoming, 3*weights[i]/(sum*float64(options.Samples))))
		}
	}
	if options.Samples > 1 {
		return float32(moment / float64(options.Samples*(options.Samples-1)))
	}
	return float32(mean * mean)
}
