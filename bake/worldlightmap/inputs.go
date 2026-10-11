package worldlightmapbake

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"runtime"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
	"github.com/karty-game/karty-sdk/internal/binwire"
)

type bakeInputs struct {
	options     Options
	rangeValue  float64
	images      map[uint32]image.Image
	surfaces    []worldlightmap.Surface
	lightIDs    []string
	reflectance string
}

func prepareBakeInputs(
	ctx context.Context,
	document world.Document,
	layout worldlightmap.Layout,
	materials []Material,
	options Options,
) (bakeInputs, error) {
	if options.Samples == 0 {
		options.Samples = 16
	}
	if options.Workers == 0 {
		options.Workers = min(64, runtime.GOMAXPROCS(0))
	}
	if options.Samples < 1 || options.Samples > worldlightmap.MaxOfflineSamples || options.Workers < 1 || options.Workers > 64 ||
		options.Bounces < 0 || options.Bounces > worldlightmap.MaxOfflineBounces {
		return bakeInputs{}, fmt.Errorf("offline bake requires samples 1..256, workers 1..64 and bounces 0..4")
	}
	if _, err := worldlightmap.OfflineProducer(options.Denoise, options.MaxRayDistance); err != nil {
		return bakeInputs{}, err
	}
	// Validate the complete world, recipe and encoded layout before bake scratch.
	rangeValue, err := worldlightmap.OfflineRGBMRangeBounded(layout, &document, options.Bounces, options.MaxRayDistance)
	if err != nil {
		return bakeInputs{}, fmt.Errorf("offline bake layout: %w", err)
	}
	images, err := materialImages(materials)
	if err != nil {
		return bakeInputs{}, err
	}
	surfaces, err := worldlightmap.Surfaces(&document)
	if err != nil {
		return bakeInputs{}, err
	}
	lightIDs := layout.RuntimeBake.LightIDs
	if options.MaxRayDistance > 0 {
		scope, scopeErr := worldlightmap.BoundedInfluenceScope(&document, layout, options.Bounces, options.MaxRayDistance)
		if scopeErr != nil {
			return bakeInputs{}, scopeErr
		}
		applyInfluenceScope(surfaces, scope)
		lightIDs = scope.LightIDs
	} else if layout.Receivers != nil {
		sectors, solids, scopeErr := worldlightmap.ReceiverScope(&document, layout.Receivers)
		if scopeErr != nil {
			return bakeInputs{}, scopeErr
		}
		for i := range surfaces {
			binding := surfaces[i].Binding
			included := false
			if binding.Kind == "sector-floor" || binding.Kind == "sector-ceiling" || binding.Kind == "sector-wall" {
				included = sectors[binding.Index]
			} else {
				included = solids[binding.Index]
			}
			if !included {
				surfaces[i].Polygons = nil
			}
		}
	}
	digest, err := reflectanceDigest(ctx, &document, surfaces, images)
	if err != nil {
		return bakeInputs{}, err
	}
	return bakeInputs{options, rangeValue, images, surfaces, lightIDs, digest}, nil
}

func applyInfluenceScope(surfaces []worldlightmap.Surface, scope worldlightmap.InfluenceScope) {
	next := 0
	for i := range surfaces {
		if next < len(scope.SurfaceIndices) && scope.SurfaceIndices[next] == i {
			next++
		} else {
			surfaces[i].Polygons = nil
		}
	}
}

// ReflectanceDigestForLayout returns exactly Bake's current reflectance
// provenance, including ordinary-component and bounded transport selection.
// It is suitable for making a fresh manifest around verified cached pixels.
func ReflectanceDigestForLayout(
	document world.Document,
	layout worldlightmap.Layout,
	materials []Material,
	options Options,
) (string, error) {
	inputs, err := prepareBakeInputs(context.Background(), document, layout, materials, options)
	if err != nil {
		return "", err
	}
	return inputs.reflectance, nil
}

// BakeInputDigest identifies the present inputs that can affect atlas pixels.
// The producer, transport options, local source/range selection, actual derived
// polygons, chart packing, UV assignments and decoded albedo pixels are bound.
// Global world/layout digests, unrelated bindings, actors and unused materials
// are omitted. A cache hit still requires fresh whole-world artifact validation
// and a new manifest; this digest never authorizes mounting an old artifact.
func BakeInputDigest(
	document world.Document,
	layout worldlightmap.Layout,
	materials []Material,
	options Options,
) (string, error) {
	ctx := context.Background()
	inputs, err := prepareBakeInputs(ctx, document, layout, materials, options)
	if err != nil {
		return "", err
	}
	pixelReflectance, err := reflectanceDigestWithBindings(ctx, &document, inputs.surfaces, inputs.images, true)
	if err != nil {
		return "", err
	}
	producer, err := worldlightmap.OfflineProducer(inputs.options.Denoise, inputs.options.MaxRayDistance)
	if err != nil {
		return "", err
	}
	w := binwire.NewWriter("KBI3", 32*1024*1024)
	w.String(producer, 128)
	w.I32(inputs.options.Samples)
	w.I32(inputs.options.Bounces)
	w.U64(inputs.options.Seed)
	w.F64(inputs.options.MaxRayDistance)
	w.F64(inputs.rangeValue)
	w.String(pixelReflectance, 64)
	w.F64(layout.TexelsPerUnit)
	w.I32(layout.Padding)
	w.Count(len(layout.Pages), false, 1)
	for _, page := range layout.Pages {
		w.I32(page.Width)
		w.I32(page.Height)
	}
	w.Count(len(layout.Charts), false, worldlightmap.MaxCharts)
	for _, chart := range layout.Charts {
		w.I32(chart.ID)
		w.I32(chart.Page)
		for _, rectangle := range [2][4]int{chart.Rect, chart.ReceiverRect} {
			for _, value := range rectangle {
				w.I32(value)
			}
		}
		for _, value := range [3]world.Vec3{chart.Normal, chart.Tangent, chart.Bitangent} {
			writeInputVector(w, value)
		}
		for _, plane := range [2][4]float64{chart.UPlane, chart.VPlane} {
			for _, value := range plane {
				w.F64(value)
			}
		}
	}
	count := 0
	for _, surface := range inputs.surfaces {
		if len(surface.Polygons) > 0 {
			count++
		}
	}
	w.Count(count, false, 76288)
	for i, surface := range inputs.surfaces {
		if len(surface.Polygons) == 0 {
			continue
		}
		// Local discovery order affects deterministic acceleration ties; global
		// array indices do not change transport and are freshly rebound on reuse.
		w.String(surface.Binding.Kind, 128)
		w.I32(surface.Binding.Edge)
		w.I32(layout.Bindings[i].Chart)
		writeInputVector(w, surface.Normal)
		w.Count(len(surface.Polygons), false, 128)
		for _, polygon := range surface.Polygons {
			w.Count(len(polygon), false, world.MaxWallsPerSector)
			for _, point := range polygon {
				writeInputVector(w, point)
			}
		}
	}
	sources := make(map[string]world.PointLight, len(document.Lighting.Lights))
	for _, light := range document.Lighting.Lights {
		sources[light.ID] = light
	}
	w.Count(len(inputs.lightIDs), false, worldlightmap.MaxBakeLights)
	for _, id := range inputs.lightIDs {
		light := sources[id]
		writeInputVector(w, light.Position)
		writeInputVector(w, light.Color)
		w.F64(light.Radius)
	}
	encoded, err := w.Bytes()
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func writeInputVector(w *binwire.Writer, value world.Vec3) {
	w.F64(value.X)
	w.F64(value.Y)
	w.F64(value.Z)
}
