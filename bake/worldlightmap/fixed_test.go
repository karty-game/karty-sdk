package worldlightmapbake

import (
	"bytes"
	"context"
	"math"
	"testing"

	"github.com/karty-game/karty-sdk/codec/qoi"
	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

func TestFixedCombinedStaticSourcesAndArtifact(t *testing.T) {
	document, _, materials := room(t, 0)
	document.Environment = &world.Environment{Version: 1, Preset: world.EnvironmentDaylight}
	document.Sectors[0].Sky = true
	var err error
	layout, err := worldlightmap.Compile(document, worldlightmap.Options{PageSize: 512, TexelsPerUnit: 1, AllStatic: true})
	if err != nil {
		t.Fatal(err)
	}
	options := Options{Samples: 2, Bounces: 1, Workers: 1}
	result, err := BakeFixed(context.Background(), document, layout, materials, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Lighting.Lights) != 2 || len(result.Lighting.Probes) != 15 {
		t.Fatalf("missing fixed lamps/sun/probes: %+v", result.Lighting)
	}
	image, _, err := qoi.Encode(result.Image, qoi.Options{Channels: qoi.ChannelsRGBA, Colorspace: qoi.ColorspaceLinear})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := worldlightmap.NewFixedLighting(layout, &document, image, result.Lighting)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := worldlightmap.EncodeFixedLighting(manifest)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := worldlightmap.DecodeFixedLightingArtifact(encoded, image, layout)
	if err != nil {
		t.Fatal(err)
	}
	again, err := worldlightmap.EncodeFixedLighting(decoded)
	if err != nil || !bytes.Equal(encoded, again) {
		t.Fatal("noncanonical fixed round trip", err)
	}
	for _, broken := range [][]byte{encoded[:len(encoded)-1], append(bytes.Clone(encoded), 0)} {
		if _, err := worldlightmap.DecodeFixedLighting(broken); err == nil {
			t.Fatal("accepted malformed fixed wire")
		}
	}
	options.Workers = 2
	second, err := BakeFixed(context.Background(), document, layout, materials, options)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(result.Image.Pix, second.Image.Pix) || result.Lighting.InputSHA256 != second.Lighting.InputSHA256 {
		t.Fatal("worker count changed fixed pixels/identity")
	}
}

func TestFixedProbeNominatedDirectNotInAmbient(t *testing.T) {
	document, layout, materials := room(t, 0)
	result, err := BakeFixed(context.Background(), document, layout, materials, Options{Samples: 1, Workers: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, probe := range result.Lighting.Probes {
		if probe.Direct[0].Light != 0 || probe.Direct[0].Visibility != 1 {
			t.Fatal("fixed lamp not selected")
		}
		for _, color := range probe.Ambient {
			if color != (world.Vec3{}) {
				t.Fatal("nominated direct lamp duplicated in zero-bounce ambient", color)
			}
		}
	}
}

func TestFixedAuthoredFillIsBakedOnceIntoSurfacesAndProbes(t *testing.T) {
	document, layout, materials := room(t, 0)
	document.Lighting.Lights[0].Color = world.Vec3{}
	options := Options{Samples: 1, Workers: 1}
	baseline, err := BakeFixed(context.Background(), document, layout, materials, options)
	if err != nil {
		t.Fatal(err)
	}
	fill := world.Vec3{X: .125, Y: .25, Z: .375}
	document.Lighting.Ambient = fill
	result, err := BakeFixed(context.Background(), document, layout, materials, options)
	if err != nil {
		t.Fatal(err)
	}
	x, y := floorAt(layout, world.Vec3{X: 2, Y: 2})
	got := decoded(result.Result, x, y)
	if math.Abs(got.X-fill.X) > .01 || math.Abs(got.Y-fill.Y) > .01 || math.Abs(got.Z-fill.Z) > .01 {
		t.Fatal("surface omitted or duplicated authored fill", got)
	}
	for _, probe := range result.Lighting.Probes {
		for _, color := range probe.Ambient {
			if color != fill {
				t.Fatal("probe omitted or duplicated authored fill", color)
			}
		}
	}
	if result.RGBMRange <= baseline.RGBMRange || result.Lighting.InputSHA256 == baseline.Lighting.InputSHA256 {
		t.Fatal("fill missing from range or provenance")
	}
	document.Lighting.AmbientCube = &world.AmbientCube{PositiveX: world.Vec3{X: .5}, NegativeZ: world.Vec3{Z: .25}}
	result, err = BakeFixed(context.Background(), document, layout, materials, options)
	if err != nil {
		t.Fatal(err)
	}
	for _, probe := range result.Lighting.Probes {
		if probe.Ambient[0] != (world.Vec3{X: .5}) || probe.Ambient[5] != (world.Vec3{Z: .25}) || probe.Ambient[4] != (world.Vec3{}) {
			t.Fatal("directional fill did not replace constant fill", probe.Ambient)
		}
	}
}
