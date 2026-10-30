package worldlightmapbake

import (
	"context"
	"image"
	"image/color"
	"reflect"
	"testing"

	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

func compactFixture(t *testing.T) (worldlightmap.Layout, FixedResult) {
	t.Helper()
	document, _, materials := room(t, 0)
	document.Lighting.Lights[0].Color.X = 0
	document.Lighting.Lights[0].Color.Y = 0
	document.Lighting.Lights[0].Color.Z = 0
	tile := [4]float64{0, 0, 2, 4}
	layout, err := worldlightmap.CompileSelectedReceivers(
		document,
		worldlightmap.Options{PageSize: 32, TexelsPerUnit: 2, AllStatic: true},
		[]int{0},
		nil,
		worldlightmap.SurfaceSelection{
			Bindings: []worldlightmap.Binding{{Kind: "sector-floor", Index: 0, Edge: -1, Chart: -1}},
			Clip:     &tile,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := BakeFixed(context.Background(), document, layout, materials, Options{Samples: 1, Workers: 1})
	if err != nil {
		t.Fatal(err)
	}
	return layout, result
}

func TestCompactFixedLightingPreservesBlackAndAllInputs(t *testing.T) {
	layout, result := compactFixture(t)
	compact, ok, err := CompactFixedLighting(layout, result.Image, result.Lighting)
	if err != nil || !ok || len(compact.ConstantRNM) != 1 || compact.ConstantRNM[0].Coefficients != [9]float32{} {
		t.Fatal("covered black did not compact", compact.ConstantRNM, ok, err)
	}
	compact.ConstantRNM = nil
	if !reflect.DeepEqual(compact, result.Lighting) {
		t.Fatal("compaction changed bake identities or non-image lighting")
	}
}

func TestCompactFixedLightingChecksInteriorAndEveryLobe(t *testing.T) {
	for basis := range 3 {
		layout, result := compactFixture(t)
		rect := layout.Charts[0].ReceiverRect
		x, y := (rect[0]+rect[2])/2, (rect[1]+rect[3])/2
		result.Image.SetNRGBA(x+basis*32, y, color.NRGBA{R: 1, A: 255})
		_, ok, err := CompactFixedLighting(layout, result.Image, result.Lighting)
		if err != nil || ok {
			t.Fatal("interior directional shadow/color difference was discarded", basis, ok, err)
		}
	}
}

func TestCompactFixedLightingRejectsAbsentOrPartialCoverage(t *testing.T) {
	layout, result := compactFixture(t)
	result.Image = image.NewNRGBA(result.Image.Bounds())
	if _, ok, err := CompactFixedLighting(layout, result.Image, result.Lighting); err != nil || ok {
		t.Fatal("uncovered chart became covered black", ok, err)
	}
	rect := layout.Charts[0].ReceiverRect
	result.Image.SetNRGBA(rect[0], rect[1], color.NRGBA{A: 1})
	if _, ok, err := CompactFixedLighting(layout, result.Image, result.Lighting); err != nil || ok {
		t.Fatal("partially covered RNM lobes compacted", ok, err)
	}
}

func TestCompactFixedLightingKeepsIndependentChartColors(t *testing.T) {
	layout, result := compactFixture(t)
	chart := layout.Charts[0]
	chart.ID, chart.Rect, chart.ReceiverRect = 1, [4]int{20, 20, 22, 22}, [4]int{20, 20, 22, 22}
	layout.Charts = append(layout.Charts, chart)
	for basis := range 3 {
		for y := 20; y < 22; y++ {
			for x := 20; x < 22; x++ {
				result.Image.SetNRGBA(x+basis*32, y, color.NRGBA{R: 255, A: 255})
			}
		}
	}
	compact, ok, err := CompactFixedLighting(layout, result.Image, result.Lighting)
	if err != nil || !ok || len(compact.ConstantRNM) != 2 {
		t.Fatal("independent constant charts rejected", compact.ConstantRNM, ok, err)
	}
	if compact.ConstantRNM[0].Coefficients != [9]float32{} || compact.ConstantRNM[1].Coefficients[0] != float32(result.RGBMRange) {
		t.Fatal("different chart colors merged", compact.ConstantRNM)
	}
}

func TestCompactFixedLightingRejectsMalformedImageStorage(t *testing.T) {
	layout, result := compactFixture(t)
	for _, pixels := range []*image.NRGBA{
		{Rect: result.Image.Rect},
		{Rect: result.Image.Rect, Pix: make([]byte, len(result.Image.Pix)), Stride: 1},
		{Rect: result.Image.Rect, Pix: make([]byte, len(result.Image.Pix)-1), Stride: result.Image.Stride},
		{Rect: result.Image.Rect, Pix: make([]byte, len(result.Image.Pix)), Stride: int(^uint(0) >> 1)},
	} {
		if _, _, err := CompactFixedLighting(layout, pixels, result.Lighting); err == nil {
			t.Fatal("malformed image storage accepted", len(pixels.Pix), pixels.Stride)
		}
	}
}
