package worldlightmapbake

import (
	"bytes"
	"context"
	"image/color"
	"math"
	"testing"
)

func TestDenoiseReducesBounceErrorAndKeepsClosedShadow(t *testing.T) {
	d, l, materials := room(t, 1.5)
	bake := func(samples int, mode string, workers int) Result {
		t.Helper()
		r, err := Bake(t.Context(), d, l, materials, Options{Samples: samples, Bounces: 1, Workers: workers, Seed: 17, Denoise: mode})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	reference := bake(256, "off", 2)
	noisy := bake(16, "off", 2)
	clean := bake(16, "medium", 2)
	floor := l.Charts[l.Bindings[0].Chart].ReceiverRect
	errorFor := func(r Result) float64 {
		sum := 0.0
		count := 0
		// Evaluate inside the directly shadowed half, away from wall contacts.
		for y := floor[1] + 3; y < floor[3]-3; y++ {
			for x := floor[0] + 3; x < floor[2]-3; x++ {
				if maximum(decoded(noisy, x, y)) > .5 {
					continue
				}
				delta := sub(decoded(r, x, y), decoded(reference, x, y))
				sum += dot(delta, delta)
				count++
			}
		}
		if count == 0 {
			t.Fatal("empty reference comparison")
		}
		return math.Sqrt(sum / float64(count))
	}
	before, after := errorFor(noisy), errorFor(clean)
	t.Logf("indirect reference RMSE: noisy %.6f, denoised %.6f; filter %s", before, after, clean.Stats.DenoiseDuration)
	if after >= before*.8 {
		t.Fatalf("denoiser did not substantially reduce reference error: %g -> %g", before, after)
	}
	if !bytes.Equal(clean.Image.Pix, bake(16, "medium", 1).Image.Pix) {
		t.Fatal("worker count changed filtered pixels")
	}
	if clean.Stats.DenoiseDuration <= 0 || clean.Stats.DenoiseRays == 0 {
		t.Fatal("missing filter diagnostics")
	}
	// A fully closed divider is in the same floor chart: chart isolation alone
	// would blur its lit side into its black side.
	d, l, materials = room(t, 3)
	for i := range materials {
		materials[i].Albedo = solid(color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	}
	closed := bake(16, "medium", 2)
	for _, p := range []vec{{X: 2.2, Y: 1}, {X: 2.2, Y: 2}, {X: 3, Y: 2}} {
		x, y := floorAt(l, p)
		if maximum(decoded(closed, x, y)) > 1e-8 {
			t.Fatalf("denoiser leaked through closed divider at %+v", p)
		}
	}
}

func TestDenoiseDirectOnlyAndInvalidPreset(t *testing.T) {
	d, l, materials := room(t, 1.5)
	off, err := Bake(t.Context(), d, l, materials, Options{Workers: 1})
	if err != nil {
		t.Fatal(err)
	}
	on, err := Bake(t.Context(), d, l, materials, Options{Workers: 2, Denoise: "medium"})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(off.Image.Pix, on.Image.Pix) || on.Stats.DenoiseDuration != 0 {
		t.Fatal("direct-only shadows were filtered")
	}
	if _, err := Bake(context.Background(), d, l, materials, Options{Denoise: "unknown"}); err == nil {
		t.Fatal("unknown preset accepted")
	}
}
