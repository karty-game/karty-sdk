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

func TestFixedSpatialTilesShareBoundarySamplingAndDoNotBakeProbes(t *testing.T) {
	for _, denoise := range []string{"", "low", "medium"} {
		t.Run("denoise="+denoise, func(t *testing.T) { testFixedSpatialBoundary(t, denoise) })
	}
}

func testFixedSpatialBoundary(t *testing.T, denoise string) {
	document, _, materials := room(t, 0)
	document.Environment = &world.Environment{Version: 1, Preset: world.EnvironmentDaylight}
	document.Sectors[0].Sky = true
	binding := worldlightmap.Binding{Kind: "sector-floor", Index: 0, Edge: -1, Chart: -1}
	// Room helper spans 0..4. The common x=2 edge must use the same sample
	// sequence even when the two charts have different atlas offsets.
	var boundary []byte
	for i, tile := range [][4]float64{{0, 0, 2, 4}, {2, 0, 4, 4}} {
		layout, err := worldlightmap.CompileSelectedReceivers(
			document,
			worldlightmap.Options{PageSize: 32, TexelsPerUnit: 2, AllStatic: true},
			[]int{0},
			nil,
			worldlightmap.SurfaceSelection{Bindings: []worldlightmap.Binding{binding}, Clip: &tile},
		)
		if err != nil {
			t.Fatal(err)
		}
		result, err := BakeFixed(
			context.Background(),
			document,
			layout,
			materials,
			Options{Samples: 16, Bounces: 1, Workers: 1, Denoise: denoise},
		)
		if err != nil {
			t.Fatal(err)
		}
		if result.Lighting.Producer != worldlightmap.FixedSpatialProducer || len(result.Lighting.Probes) != 0 {
			t.Fatal("spatial child duplicated actor probes")
		}
		chart := layout.Charts[0]
		edge := []byte{}
		for y := range 9 {
			p := world.Vec3{X: 2, Y: float64(y) / 2}
			u := chart.UPlane[0]*p.X + chart.UPlane[1]*p.Y + chart.UPlane[3]
			v := chart.VPlane[0]*p.X + chart.VPlane[1]*p.Y + chart.VPlane[3]
			x, iy := int(math.Round(u*32-.5)), int(math.Round(v*32-.5))
			for basis := range 3 {
				at := result.Image.PixOffset(x+basis*32, iy)
				edge = append(edge, result.Image.Pix[at:at+4]...)
			}
		}
		if i == 0 {
			boundary = edge
		} else if !bytes.Equal(boundary, edge) {
			t.Fatal("tile boundary lighting/sample sequence differs")
		}
		image, _, err := qoi.Encode(result.Image, qoi.Options{Channels: qoi.ChannelsRGBA, Colorspace: qoi.ColorspaceLinear})
		if err != nil {
			t.Fatal(err)
		}
		fixed, err := worldlightmap.NewFixedLighting(layout, &document, image, result.Lighting)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := worldlightmap.EncodeFixedLighting(fixed)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := worldlightmap.DecodeFixedLightingArtifact(encoded, image, layout); err != nil {
			t.Fatal(err)
		}
		fixed.Producer = worldlightmap.FixedProducer
		if err := worldlightmap.ValidateFixedLighting(fixed, layout, &document); err == nil {
			t.Fatal("spatial child accepted legacy probe semantics")
		}
	}
}

func TestFixedSpatialPreservesCoveredBlackAndUncoveredPixels(t *testing.T) {
	document, _, materials := room(t, 0)
	document.Lighting.Lights[0].Color = world.Vec3{}
	tile := [4]float64{0, 0, 2, 4}
	binding := worldlightmap.Binding{Kind: "sector-floor", Index: 0, Edge: -1, Chart: -1}
	layout, err := worldlightmap.CompileSelectedReceivers(
		document,
		worldlightmap.Options{PageSize: 32, TexelsPerUnit: 2, AllStatic: true},
		[]int{0},
		nil,
		worldlightmap.SurfaceSelection{Bindings: []worldlightmap.Binding{binding}, Clip: &tile},
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := BakeFixed(context.Background(), document, layout, materials, Options{Samples: 1, Workers: 1})
	if err != nil {
		t.Fatal(err)
	}
	c := layout.Charts[0]
	x, y := c.ReceiverRect[0]+1, c.ReceiverRect[1]+1
	at := result.Image.PixOffset(x, y)
	if !bytes.Equal(result.Image.Pix[at:at+4], []byte{0, 0, 0, 1}) {
		t.Fatal("covered black lost validity", result.Image.Pix[at:at+4])
	}
	at = result.Image.PixOffset(31, 31)
	if !bytes.Equal(result.Image.Pix[at:at+4], []byte{0, 0, 0, 0}) {
		t.Fatal("uncovered atlas pixel became covered")
	}
}
