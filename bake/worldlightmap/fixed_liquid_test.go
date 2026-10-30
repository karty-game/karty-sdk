package worldlightmapbake

import (
	"bytes"
	"image/color"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

func fixedLiquidFixture(t *testing.T) (world.Document, worldlightmap.Layout, []Material) {
	t.Helper()
	document, _, materials := environmentRoom(t, true, 3)
	glass := &document.StaticSolids.Items[0]
	glass.SideMaterial, glass.TopMaterial, glass.BottomMaterial = 3, 3, 3
	materials = append(materials, Material{ID: 3, Albedo: solid(color.NRGBA{R: 255, A: 255})})
	document.Animations = &world.Animations{Version: 1,
		Presets:   []world.AnimationPreset{{Name: "water", Kind: "liquid", Frequency: 1, Opacity: new(.76)}},
		Materials: []world.MaterialAnimation{{Material: 3, Preset: "water"}}}
	layout, err := worldlightmap.Compile(document, worldlightmap.Options{TexelsPerUnit: 2, PageSize: 512, Padding: 2})
	if err != nil {
		t.Fatal(err)
	}
	return document, layout, materials
}

func TestFixedLiquidReceivesLightingWithoutOccludingOrReflecting(t *testing.T) {
	t.Parallel()
	document, layout, materials := fixedLiquidFixture(t)
	options := Options{Samples: 16, Bounces: 1, Workers: 1, Seed: 17}
	transparent, err := BakeFixed(t.Context(), document, layout, materials, options)
	if err != nil {
		t.Fatal(err)
	}
	if transparent.Stats.Triangles != 2 {
		t.Fatalf("liquid entered the opaque transport BVH: %d triangles", transparent.Stats.Triangles)
	}
	assertFixedLiquidReceiver(t, layout, transparent.Result)
	materials[len(materials)-1].Albedo = solid(color.NRGBA{B: 255, A: 255})
	otherAlbedo, err := BakeFixed(t.Context(), document, layout, materials, options)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(transparent.Image.Pix, otherAlbedo.Image.Pix) ||
		transparent.ReflectanceSHA256 != otherAlbedo.ReflectanceSHA256 {
		t.Fatal("translucent receiver albedo contributed reflected transport")
	}
	document.Animations.Presets[0].Opacity = new(1.0)
	opaque, err := BakeFixed(t.Context(), document, layout, materials, options)
	if err != nil {
		t.Fatal(err)
	}
	x, y := floorAt(layout, vec{X: 3, Y: 2})
	if maximum(decoded(transparent.Result, x, y)) < maximum(decoded(opaque.Result, x, y))+.15 ||
		opaque.Stats.Triangles <= transparent.Stats.Triangles {
		t.Fatal(
			"translucent surface blocked sunlight or opaque control failed to cast",
			decoded(transparent.Result, x, y),
			decoded(opaque.Result, x, y),
		)
	}
}

func assertFixedLiquidReceiver(t *testing.T, layout worldlightmap.Layout, result Result) {
	t.Helper()
	for _, binding := range layout.Bindings {
		if binding.Kind != "solid-top" || binding.Chart < 0 {
			continue
		}
		chart := layout.Charts[binding.Chart]
		x, y := chart.ReceiverRect[0], chart.ReceiverRect[1]
		if result.Image.NRGBAAt(x, y).A == 0 || maximum(decoded(result, x, y)) < .05 {
			t.Fatal("fixed translucent surface lost covered sun/sky irradiance", decoded(result, x, y))
		}
		return
	}
	t.Fatal("fixture lacks a translucent solid-top receiver")
}

func TestFixedLiquidReceivesSelectedStaticLampAndPreservesBlackCoverage(t *testing.T) {
	t.Parallel()
	document, _, materials := fixedLiquidFixture(t)
	document.Environment = nil
	document.Sectors[0].Sky = false
	for index := range document.Sectors[0].Walls {
		document.Sectors[0].Walls[index].Sky = false
	}
	document.StaticSolids.Items[0].Top.C = 1.5
	document.Lighting = &world.Lighting{Version: world.LightingVersion, Lights: []world.PointLight{
		{ID: "lamp", Position: vec{X: 2, Y: 2, Z: 2.5}, Color: vec{X: .25, Y: .5, Z: 1}, Radius: 20},
	}}
	layout, err := worldlightmap.Compile(document, worldlightmap.Options{TexelsPerUnit: 2, PageSize: 512, Padding: 2, AllStatic: true})
	if err != nil {
		t.Fatal(err)
	}
	options := Options{Samples: 1, Workers: 1}
	lit, err := BakeFixed(t.Context(), document, layout, materials, options)
	if err != nil {
		t.Fatal(err)
	}
	assertFixedLiquidReceiver(t, layout, lit.Result)
	document.Lighting.Lights[0].Color = vec{}
	dark, err := BakeFixed(t.Context(), document, layout, materials, options)
	if err != nil {
		t.Fatal(err)
	}
	for _, binding := range layout.Bindings {
		if binding.Kind != "solid-top" || binding.Chart < 0 {
			continue
		}
		chart := layout.Charts[binding.Chart]
		x, y := chart.ReceiverRect[0], chart.ReceiverRect[1]
		if dark.Image.NRGBAAt(x, y).A == 0 || maximum(decoded(dark.Result, x, y)) != 0 {
			t.Fatal("unlit translucent receiver lost covered-black semantics")
		}
	}
}

func TestFixedLiquidReceiversDoNotChangeSharedLegacyTransport(t *testing.T) {
	t.Parallel()
	document, layout, materials := fixedLiquidFixture(t)
	baker := NewBaker(document, materials)
	options := Options{Samples: 4, Workers: 1, Bounces: 1, Seed: 3}
	settings := world.EnvironmentSettings{TimeOfDay: 15}
	legacy, err := baker.BakeEnvironment(t.Context(), layout, settings, options)
	if err != nil {
		t.Fatal(err)
	}
	prepared := baker.prepared
	fixed, err := baker.BakeFixed(t.Context(), layout, options)
	if err != nil {
		t.Fatal(err)
	}
	assertFixedLiquidReceiver(t, layout, fixed.Result)
	if baker.prepared != prepared {
		t.Fatal("receiver-only geometry changed shared opaque transport")
	}
	again, err := baker.BakeEnvironment(t.Context(), layout, settings, options)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(legacy.Image.Pix, again.Image.Pix) {
		t.Fatal("fixed receiver rasterization mutated legacy environment coverage")
	}
	for _, binding := range layout.Bindings {
		if binding.Kind == "solid-top" && binding.Chart >= 0 {
			chart := layout.Charts[binding.Chart]
			if legacy.Image.NRGBAAt(chart.ReceiverRect[0], chart.ReceiverRect[1]).A != 0 {
				t.Fatal("legacy environment producer began baking translucent receivers")
			}
		}
	}
}
