package worldlightmapbake

import (
	"bytes"
	"context"
	"image/color"
	"math"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

func environmentRoom(t testing.TB, open bool, barrier float64) (world.Document, worldlightmap.Layout, []Material) {
	t.Helper()
	document, _, materials := room(t, barrier)
	document.Lighting = nil
	document.Environment = &world.Environment{Version: 1, Preset: world.EnvironmentDaylight}
	document.Sectors[0].Sky = open
	for i := range document.Sectors[0].Walls {
		document.Sectors[0].Walls[i].Sky = open
	}
	layout, err := worldlightmap.Compile(document, worldlightmap.Options{TexelsPerUnit: 2, PageSize: 512, Padding: 2})
	if err != nil {
		t.Fatal(err)
	}
	return document, layout, materials
}

func TestEnvironmentBakeSunShadowsSkyOpeningsAndWeather(t *testing.T) {
	t.Parallel()
	document, layout, materials := environmentRoom(t, true, 3)
	options := Options{Samples: 32, Workers: 1, Seed: 17}
	clearSky, err := BakeEnvironment(t.Context(), document, layout, materials, world.EnvironmentSettings{TimeOfDay: 15}, options)
	if err != nil {
		t.Fatal(err)
	}
	overcast, err := BakeEnvironment(t.Context(), document, layout, materials,
		world.EnvironmentSettings{TimeOfDay: 15, CloudAmount: 1}, options)
	if err != nil {
		t.Fatal(err)
	}
	x, y := floorAt(layout, vec{X: .5, Y: 2})
	lit, shaded := decoded(clearSky, x, y), decoded(overcast, x, y)
	if maximum(lit) < maximum(shaded)+.15 || maximum(shaded) < .04 {
		t.Fatalf("sun/weather endpoints or visible sky missing: clear=%+v overcast=%+v", lit, shaded)
	}
	x, y = floorAt(layout, vec{X: 3, Y: 2})
	shadowClear, shadowOvercast := decoded(clearSky, x, y), decoded(overcast, x, y)
	if math.Abs(maximum(shadowClear)-maximum(shadowOvercast)) > .01 {
		t.Fatalf("solid failed to occlude directional sun: %+v / %+v", shadowClear, shadowOvercast)
	}
	closed, closedLayout, materials := environmentRoom(t, false, 0)
	dark, err := BakeEnvironment(t.Context(), closed, closedLayout, materials,
		world.EnvironmentSettings{TimeOfDay: 12}, options)
	if err != nil {
		t.Fatal(err)
	}
	x, y = floorAt(closedLayout, vec{X: 2, Y: 2})
	if maximum(decoded(dark, x, y)) > 1e-8 || dark.Image.NRGBAAt(x, y).A == 0 {
		t.Fatal("closed room leaked sun/sky or lost covered-black pixels")
	}
}

func TestEnvironmentBakeNightWorkersIdentityAndBounds(t *testing.T) {
	t.Parallel()
	document, layout, materials := environmentRoom(t, true, 0)
	settings := world.EnvironmentSettings{TimeOfDay: 12, CloudAmount: .35}
	options := Options{Samples: 8, Bounces: 1, Workers: 1, Seed: 3, Denoise: "low"}
	day, err := BakeEnvironment(t.Context(), document, layout, materials, settings, options)
	if err != nil {
		t.Fatal(err)
	}
	options.Workers = 2
	parallel, err := BakeEnvironment(t.Context(), document, layout, materials, settings, options)
	if err != nil || !bytes.Equal(day.Image.Pix, parallel.Image.Pix) {
		t.Fatalf("worker scheduling changed variant pixels: %v", err)
	}
	key, err := EnvironmentInputDigest(document, layout, materials, settings, options)
	if err != nil {
		t.Fatal(err)
	}
	document.Lighting = &world.Lighting{Version: 1, Ambient: vec{X: .5}, Lights: []world.PointLight{
		{ID: "ignored", Position: vec{X: 1, Y: 1, Z: 1}, Color: vec{X: 1}, Radius: 4},
	}}
	otherKey, err := EnvironmentInputDigest(document, layout, materials, settings, options)
	if err != nil || key != otherKey {
		t.Fatalf("runtime point lights invalidated environment diffuse: %v", err)
	}
	night, err := BakeEnvironment(t.Context(), document, layout, materials,
		world.EnvironmentSettings{TimeOfDay: 0, CloudAmount: .35}, options)
	if err != nil {
		t.Fatal(err)
	}
	x, y := floorAt(layout, vec{X: 2, Y: 2})
	if maximum(decoded(night, x, y)) >= .2*maximum(decoded(day, x, y)) {
		t.Fatal("night retained daytime sun")
	}
	settings.TimeOfDay = 9
	newKey, err := EnvironmentInputDigest(document, layout, materials, settings, options)
	if err != nil || key == newKey {
		t.Fatalf("clock failed to invalidate bake identity: %v", err)
	}
	for _, invalid := range []Options{{Samples: 257}, {Workers: 65}, {Bounces: 5}, {MaxRayDistance: 4}} {
		if _, err := BakeEnvironment(t.Context(), document, layout, materials, settings, invalid); err == nil {
			t.Fatalf("unbounded work or truncated sky ray accepted: %+v", invalid)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if result, err := BakeEnvironment(ctx, document, layout, materials, settings, options); err == nil || result.Image != nil {
		t.Fatal("cancelled bake published a partial variant")
	}
}

func TestEnvironmentBakeGlassTransmitsSunWithoutOpaqueCharts(t *testing.T) {
	t.Parallel()
	document, _, materials := environmentRoom(t, true, 3)
	glass := &document.StaticSolids.Items[0]
	glass.SideMaterial, glass.TopMaterial, glass.BottomMaterial = 3, 3, 3
	materials = append(materials, Material{ID: 3, Albedo: solid(color.NRGBA{R: 200, G: 220, B: 235, A: 255})})
	opacity := .16
	document.Animations = &world.Animations{Version: 1,
		Presets:   []world.AnimationPreset{{Name: "glass", Kind: "liquid", Frequency: 1, Opacity: &opacity}},
		Materials: []world.MaterialAnimation{{Material: 3, Preset: "glass"}}}
	layout, err := worldlightmap.Compile(document, worldlightmap.Options{TexelsPerUnit: 2, PageSize: 512, Padding: 2})
	if err != nil {
		t.Fatal(err)
	}
	settings := world.EnvironmentSettings{TimeOfDay: 15}
	options := Options{Samples: 8, Workers: 1}
	transparent, err := BakeEnvironment(t.Context(), document, layout, materials, settings, options)
	if err != nil {
		t.Fatal(err)
	}
	opacity = 1
	opaque, err := BakeEnvironment(t.Context(), document, layout, materials, settings, options)
	if err != nil {
		t.Fatal(err)
	}
	x, y := floorAt(layout, vec{X: 3, Y: 2})
	if maximum(decoded(transparent, x, y)) < maximum(decoded(opaque, x, y))+.15 {
		t.Fatal("transparent pane occluded sun")
	}
	if transparent.Stats.Triangles != 2 || opaque.Stats.Triangles <= transparent.Stats.Triangles {
		t.Fatalf("glass remained in opaque BVH: %d / %d", transparent.Stats.Triangles, opaque.Stats.Triangles)
	}
	for _, binding := range layout.Bindings {
		if binding.Kind == "solid-top" && binding.Chart >= 0 {
			chart := layout.Charts[binding.Chart]
			if transparent.Image.NRGBAAt(chart.ReceiverRect[0], chart.ReceiverRect[1]).A != 0 {
				t.Fatal("transparent pane received opaque irradiance")
			}
		}
	}
}

func TestEnvironmentSunUsesRotatedChartBasisWithoutChangingNeutralEnergy(t *testing.T) {
	environment, err := world.ResolveEnvironmentSettings(&world.Environment{Version: 1, Preset: world.EnvironmentDaylight},
		world.EnvironmentSettings{TimeOfDay: 15})
	if err != nil {
		t.Fatal(err)
	}
	scene := scene{environment: &environment}
	chart := worldlightmap.Chart{Normal: vec{Z: 1}, Tangent: vec{X: 1}, Bitangent: vec{Y: 1}}
	var counters counters
	original := scene.environmentSun(vec{}, chart, &counters)
	chart.Tangent, chart.Bitangent = chart.Bitangent, scale(chart.Tangent, -1)
	rotated := scene.environmentSun(vec{}, chart, &counters)
	before, after := vec{}, vec{}
	for i := range original {
		before, after = add(before, original[i]), add(after, rotated[i])
	}
	if original == rotated || dot(sub(before, after), sub(before, after)) > 1e-20 {
		t.Fatal("chart rotation lost directional coefficients or changed neutral irradiance")
	}
}
