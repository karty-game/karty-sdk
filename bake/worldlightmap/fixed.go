package worldlightmapbake

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
	"github.com/karty-game/karty-sdk/format/worldterrain"
	"github.com/karty-game/karty-sdk/internal/binwire"
)

type FixedResult struct {
	Result
	Lighting worldlightmap.FixedLighting
}

// BakeFixed resolves the authored fixed sun/sky, selected static lamps and
// geometry once. Runtime time/weather overrides are deliberately absent.
func BakeFixed(
	ctx context.Context,
	document world.Document,
	layout worldlightmap.Layout,
	materials []Material,
	options Options,
) (FixedResult, error) {
	return bakeFixed(ctx, document, layout, materials, options, nil)
}

func (b *Baker) BakeFixed(ctx context.Context, layout worldlightmap.Layout, options Options) (FixedResult, error) {
	if b == nil {
		return FixedResult{}, fmt.Errorf("nil shared baker")
	}
	return bakeFixed(ctx, b.document, layout, b.materials, options, b)
}

func fixedEnvironment(document world.Document) (world.ResolvedEnvironment, error) {
	if document.Environment == nil {
		return world.ResolvedEnvironment{}, nil
	}
	return world.ResolveEnvironment(document.Environment)
}

func fixedRGBMRange(
	document world.Document,
	layout worldlightmap.Layout,
	environment world.ResolvedEnvironment,
	options Options,
) (float64, error) {
	point := 0.0
	if layout.RuntimeBake != nil {
		var err error
		point, err = worldlightmap.OfflineRGBMRange(layout, &document, options.Bounces)
		if err != nil {
			return 0, err
		}
	}
	energy := environment.SunIntensity * max(environment.SunColor.X, environment.SunColor.Y, environment.SunColor.Z)
	for _, color := range [6]vec{environment.AmbientCube.PositiveX, environment.AmbientCube.NegativeX, environment.AmbientCube.PositiveY, environment.AmbientCube.NegativeY, environment.AmbientCube.PositiveZ, environment.AmbientCube.NegativeZ} {
		energy = max(energy, maximum(color)+environment.SunIntensity)
	}
	factor, attenuation := 1.0, 1.0
	for range options.Bounces {
		attenuation *= worldlightmap.MaxDiffuseReflectance
		factor += attenuation
	}
	fill := 0.0
	for _, axis := range probeAxes {
		fill = max(fill, maximum(fixedFill(document.Lighting, axis)))
	}
	return max(1, point+3*energy*factor+fill), nil
}

func bakeFixed(
	ctx context.Context,
	document world.Document,
	layout worldlightmap.Layout,
	materials []Material,
	options Options,
	shared *Baker,
) (FixedResult, error) {
	started := time.Now()
	options.fixed = true
	environment, err := fixedEnvironment(document)
	if err != nil {
		return FixedResult{}, err
	}
	result, err := bake(ctx, document, layout, materials, options, &environment, shared)
	if err != nil {
		return FixedResult{}, err
	}
	inputs, err := prepareEnvironmentInputs(ctx, document, layout, materials, options, environment)
	if err != nil {
		return FixedResult{}, err
	}
	prepared, err := shared.transport(ctx, &document, inputs, true)
	if err != nil {
		return FixedResult{}, err
	}
	s := scene{
		fixedFill:     document.Lighting,
		surfaces:      prepared.surfaces,
		materials:     prepared.materials,
		accel:         prepared.accel,
		surfaceFrames: prepared.surfaceFrames,
		environment:   &environment,
		samples:       initialSamples(inputs.options.Samples),
	}
	lighting := worldlightmap.FixedLighting{
		Schema:            worldlightmap.FixedSchema,
		Producer:          fixedProducer(layout),
		ReflectanceSHA256: inputs.reflectance,
		RGBMRange:         result.RGBMRange,
		Samples:           inputs.options.Samples,
		Bounces:           inputs.options.Bounces,
		Seed:              inputs.options.Seed,
		Denoise:           inputs.options.Denoise,
	}
	lighting.SurfaceSHA256, err = worldlightmap.EnvironmentSurfaceDigest(&document)
	if err != nil {
		return FixedResult{}, err
	}
	lighting.InputSHA256, err = FixedInputDigest(document, layout, materials, options)
	if err != nil {
		return FixedResult{}, err
	}
	for _, id := range inputs.lightIDs {
		for _, light := range document.Lighting.Lights {
			if light.ID != id {
				continue
			}
			s.lights = append(s.lights, preparedLight{light, light.Radius * light.Radius})
			lighting.Lights = append(
				lighting.Lights,
				worldlightmap.CompiledLight{
					ID:       light.ID,
					Kind:     "point",
					Position: light.Position,
					Color:    light.Color,
					Radius:   light.Radius,
				},
			)
		}
	}
	if environment.SunIntensity > 0 {
		lighting.Lights = append(
			lighting.Lights,
			worldlightmap.CompiledLight{
				ID:        "@sun",
				Kind:      "sun",
				Direction: environment.SunDirection,
				Color:     scale(environment.SunColor, environment.SunIntensity*(1-environment.CloudAmount)),
			},
		)
	}
	s.lightIndex.prepare(s.lights)
	var extra counters
	lighting.Terrain, err = s.fixedTerrain(ctx, inputs.options, &extra)
	if err != nil {
		return FixedResult{}, err
	}
	if len(lighting.Terrain) > 0 {
		lighting.TerrainSHA256, err = terrainInputDigest(inputs.options.Terrain)
		if err != nil {
			return FixedResult{}, err
		}
		lighting.TerrainReceiverSHA256, err = terrainInputDigest(inputs.options.TerrainReceivers)
		if err != nil {
			return FixedResult{}, err
		}
	}
	if fixedProducer(layout) != worldlightmap.FixedSpatialProducer {
		lighting.Probes, err = s.fixedProbes(ctx, document, layout, lighting.Lights, inputs.options, &extra)
	}
	if err != nil {
		return FixedResult{}, err
	}
	result.Stats.ShadowRays += extra.shadow
	result.Stats.BounceRays += extra.bounce
	result.Stats.Rays += extra.shadow + extra.bounce
	result.Stats.Duration = time.Since(started)
	return FixedResult{Result: result, Lighting: lighting}, nil
}

func FixedReflectanceDigest(document world.Document, layout worldlightmap.Layout, materials []Material, options Options) (string, error) {
	options.fixed = true
	environment, err := fixedEnvironment(document)
	if err != nil {
		return "", err
	}
	inputs, err := prepareEnvironmentInputs(context.Background(), document, layout, materials, options, environment)
	if err != nil {
		return "", err
	}
	return inputs.reflectance, nil
}

func FixedInputDigest(document world.Document, layout worldlightmap.Layout, materials []Material, options Options) (string, error) {
	options.fixed = true
	environment, err := fixedEnvironment(document)
	if err != nil {
		return "", err
	}
	inputs, err := prepareEnvironmentInputs(context.Background(), document, layout, materials, options, environment)
	if err != nil {
		return "", err
	}
	encoded, err := world.Encode(document)
	if err != nil {
		return "", err
	}
	layoutHash, err := worldlightmap.LayoutDigest(&layout)
	if err != nil {
		return "", err
	}
	w := binwire.NewWriter("KFI1", 2048+MaxShadowTriangles*72)
	digest := sha256.Sum256(encoded)
	w.String(fixedProducer(layout), 128)
	w.String(hex.EncodeToString(digest[:]), 64)
	w.String(layoutHash, 64)
	w.String(inputs.reflectance, 64)
	for _, terrain := range []*worldterrain.Document{inputs.options.Terrain, inputs.options.TerrainReceivers} {
		hash, terrainErr := terrainInputDigest(terrain)
		if terrainErr != nil {
			return "", terrainErr
		}
		w.String(hash, 64)
	}
	w.I32(inputs.options.Samples)
	w.I32(inputs.options.Bounces)
	w.U64(inputs.options.Seed)
	w.String(inputs.options.Denoise, 16)
	w.F64(inputs.rangeValue)
	writeShadowTriangles(w, inputs.options.ShadowTriangles)
	if err := writeVegetationInput(w, inputs.options.Vegetation); err != nil {
		return "", err
	}
	encoded, err = w.Bytes()
	if err != nil {
		return "", err
	}
	digest = sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func fixedProducer(layout worldlightmap.Layout) string {
	if layout.Selection != nil && layout.Selection.Clip != nil {
		return worldlightmap.FixedSpatialProducer
	}
	return worldlightmap.FixedProducer
}
