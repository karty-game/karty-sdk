package worldlightmapbake

import (
	"context"
	"errors"
	"image"
	"image/color"
	"math"
	"testing"
)

type wrappedAlbedo struct{ image.Image }

// Sampling may cache image representations, but decoded pixel semantics,
// including premultiplied alpha, subimages and shifted bounds, must not change.
func TestPreparedAlbedoPreservesPixels(t *testing.T) {
	bounds := image.Rect(-3, 2, 5, 7)
	rgba := image.NewRGBA(bounds)
	rgba64 := image.NewRGBA64(bounds)
	gray := image.NewGray(bounds)
	paletted := image.NewPaletted(bounds, color.Palette{color.NRGBA{}, color.NRGBA{R: 71, G: 192, B: 43, A: 127}, color.White})
	ycbcr := image.NewYCbCr(bounds, image.YCbCrSubsampleRatio420)
	nrgba := image.NewNRGBA(image.Rect(-4, 1, 6, 8))
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := color.NRGBA{R: uint8((x + 3) * 29), G: uint8((y - 2) * 47), B: 93, A: uint8((x + 3) * 31)}
			rgba.Set(x, y, c)
			rgba64.Set(x, y, c)
			gray.Set(x, y, c)
			paletted.Set(x, y, c)
			nrgba.SetNRGBA(x, y, c)
			ycbcr.Y[ycbcr.YOffset(x, y)] = c.R
			j := ycbcr.COffset(x, y)
			ycbcr.Cb[j], ycbcr.Cr[j] = c.G, c.B
		}
	}
	subimage := nrgba.SubImage(bounds).(*image.NRGBA)
	for _, source := range []image.Image{rgba, rgba64, gray, paletted, ycbcr, subimage, wrappedAlbedo{rgba}} {
		prepared, err := prepareAlbedo(context.Background(), source)
		if err != nil {
			t.Fatal(err)
		}
		if prepared.Rect != source.Bounds() {
			t.Fatalf("%T: bounds changed", source)
		}
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				if got, want := prepared.NRGBAAt(x, y), albedoPixel(source, x, y); got != want {
					t.Fatalf("%T: at %d,%d: %v != %v", source, x, y, got, want)
				}
			}
		}
	}
	borrowed, err := prepareAlbedo(context.Background(), subimage)
	if err != nil || borrowed != subimage {
		t.Fatal("NRGBA subimage was copied unnecessarily")
	}
	ctx, cancel := context.WithCancel(context.Background())
	source := &cancellingImage{cancel: cancel}
	if prepared, err := prepareAlbedo(ctx, source); !errors.Is(err, context.Canceled) || prepared != nil {
		t.Fatalf("cancelled conversion: %v %v", prepared, err)
	}
}

func TestPreparedRayBoxIntervals(t *testing.T) {
	boxes := []box{{vec{X: -1, Y: -1, Z: -1}, vec{X: 1, Y: 1, Z: 1}}, {vec{}, vec{X: 1, Y: 1}}, {vec{X: 1e100, Y: -1, Z: -1}, vec{X: 1e100, Y: 1, Z: 1}}}
	origins := []vec{{}, {X: 1}, {X: -1}, {X: 1, Y: 1, Z: 1}, {X: 2}, {X: 1e100}, {X: -1e100}}
	directions := []vec{{X: 1}, {X: -1}, {Y: 1}, {Z: 1}, {X: 1, Y: 1, Z: 1}, {X: 1e-16, Y: 1}, {X: 1e-15, Y: 1}, {X: math.Copysign(0, -1), Y: 1}}
	for _, bounds := range boxes {
		for _, o := range origins {
			for _, d := range directions {
				for _, limit := range []float64{0, math.Copysign(0, -1), 1, 2, math.Inf(1)} {
					if got, want := bounds.ray(o, d, limit), scalarBoxRay(bounds, o, d, limit); got != want {
						t.Fatalf("box %v ray %v/%v limit %g: %v != %v", bounds, o, d, limit, got, want)
					}
				}
			}
		}
	}
}
