package worldlightmapbake

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/karty-game/karty-sdk/codec/qoi"
	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

func TestThousandStaticSourcesKeepThreeFixedCoefficientTiles(t *testing.T) {
	document, singleLayout, materials := room(t, 0)
	single, err := Bake(context.Background(), document, singleLayout, materials, Options{Workers: 1, Samples: 1})
	if err != nil {
		t.Fatal(err)
	}
	source := document.Lighting.Lights[0]
	document.Lighting.Lights = make([]world.PointLight, 1000)
	for i := range document.Lighting.Lights {
		light := source
		light.ID = fmt.Sprintf("%04d-", i) + strings.Repeat("s", 120)
		light.Color = scale(light.Color, .001)
		document.Lighting.Lights[i] = light
	}
	encodedWorld, err := world.Encode(document)
	if err != nil {
		t.Fatal(err)
	}
	document, err = world.Decode(encodedWorld)
	if err != nil || len(document.Lighting.Lights) != 1000 || world.MaxRuntimeLights != 50 {
		t.Fatal("authored and runtime budgets are coupled", err)
	}
	layout, err := worldlightmap.Compile(document, worldlightmap.Options{
		AllStatic: true, PageSize: 512, TexelsPerUnit: 1, Padding: 2, ShadowSize: 32,
	})
	if err != nil || len(layout.RuntimeBake.LightIDs) != 1000 || len(layout.Pages) != 1 {
		t.Fatal("full static source recipe failed", err)
	}
	result, err := Bake(context.Background(), document, layout, materials, Options{Workers: 1, Samples: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.Image.Bounds() != single.Image.Bounds() || result.Image.Rect.Dx() != 3*layout.Pages[0].Width ||
		result.Stats.ReceiverTexels != single.Stats.ReceiverTexels {
		t.Fatal("source count changed texture dimensions or receiver allocation")
	}
	x, y := floorAt(layout, vec{X: 2, Y: 2})
	got, want := decoded(result, x, y), decoded(single, x, y)
	if math.Abs(got.X-want.X) > .02 || math.Abs(got.Y-want.Y) > .02 || math.Abs(got.Z-want.Z) > .02 {
		t.Fatalf("1000 dim lamps lost combined energy: %v versus %v", got, want)
	}
	image, _, err := qoi.Encode(result.Image, qoi.Options{Channels: qoi.ChannelsRGBA, Colorspace: qoi.ColorspaceLinear})
	if err != nil {
		t.Fatal(err)
	}
	pair, err := worldlightmap.NewOfflinePrebake(layout, &document, image, worldlightmap.OfflineBakeInputs{
		Samples: 1, ReflectanceSHA256: result.ReflectanceSHA256, RGBMRange: result.RGBMRange,
	})
	if err != nil {
		t.Fatal("source identity scratch retained 16 KiB manifest limit", err)
	}
	manifest, err := worldlightmap.EncodePrebake(pair, layout, &document)
	if err != nil || len(manifest) > worldlightmap.MaxPrebakeManifestSize {
		t.Fatal("fixed manifest grew with full source records", err)
	}
	if _, err := worldlightmap.DecodePrebake(manifest, image, layout, &document); err != nil {
		t.Fatal("1000 source integrity round trip", err)
	}
	document.Lighting.Lights[999].Position.X += .1
	if err := pair.Validate(layout, &document); err == nil {
		t.Fatal("final baked source edit omitted from manifest identity")
	}
}
