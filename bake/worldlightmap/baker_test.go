package worldlightmapbake

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

func TestSharedBakerScopesAndValidation(t *testing.T) {
	t.Parallel()
	document, first, materials := receiverRooms(t, true)
	second, err := worldlightmap.CompileReceivers(document,
		worldlightmap.Options{TexelsPerUnit: 1, PageSize: 512, Padding: 2, Lights: []string{"white"}}, []int{1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	baker := NewBaker(document, materials)
	options := Options{Samples: 4, Bounces: 1, Workers: 2, Seed: 3, Denoise: "medium"}
	var previous *preparedTransport
	for _, layout := range []worldlightmap.Layout{first, second, first} {
		fresh, err := Bake(t.Context(), document, layout, materials, options)
		if err != nil {
			t.Fatal(err)
		}
		shared, err := baker.Bake(t.Context(), layout, options)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(fresh.Image.Pix, shared.Image.Pix) || fresh.Stats.Rays != shared.Stats.Rays ||
			fresh.ReflectanceSHA256 != shared.ReflectanceSHA256 {
			t.Fatal("shared preparation changed pixels, counts or provenance")
		}
		if previous != nil && previous != baker.prepared {
			t.Fatal("ordinary neighbours failed to share transport")
		}
		previous = baker.prepared
	}
	options.MaxRayDistance = .5
	for _, layout := range []worldlightmap.Layout{first, second} {
		fresh, err := Bake(t.Context(), document, layout, materials, options)
		if err != nil {
			t.Fatal(err)
		}
		shared, err := baker.Bake(t.Context(), layout, options)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(fresh.Image.Pix, shared.Image.Pix) || fresh.ReflectanceSHA256 != shared.ReflectanceSHA256 {
			t.Fatal("bounded scope reused incompatible transport")
		}
		if previous == baker.prepared {
			t.Fatal("changed bounded scope retained previous preparation")
		}
		previous = baker.prepared
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if result, err := baker.Bake(ctx, first, options); !errors.Is(err, context.Canceled) || result.Image != nil {
		t.Fatalf("cancelled shared bake returned image: %v", err)
	}
	invalid := first
	invalid.Pages = []worldlightmap.Page{{Width: 1, Height: 1}}
	if _, err := baker.Bake(t.Context(), invalid, options); err == nil {
		t.Fatal("shared transport bypassed layout validation")
	}
	var absent *Baker
	if _, err := absent.Bake(t.Context(), first, options); err == nil {
		t.Fatal("nil baker accepted")
	}
}

func TestSharedBakerEnvironmentConcurrentVariants(t *testing.T) {
	t.Parallel()
	document, layout, materials := environmentRoom(t, true, 1.5)
	baker := NewBaker(document, materials)
	options := Options{Samples: 4, Bounces: 1, Workers: 2, Seed: 7, Denoise: "medium"}
	settings := []world.EnvironmentSettings{{TimeOfDay: 12}, {TimeOfDay: 15, CloudAmount: 1}, {TimeOfDay: 0}}
	want := make([]Result, len(settings))
	for i, condition := range settings {
		var err error
		want[i], err = BakeEnvironment(t.Context(), document, layout, materials, condition, options)
		if err != nil {
			t.Fatal(err)
		}
	}
	var group sync.WaitGroup
	for i, condition := range settings {
		group.Go(func() {
			actual, err := baker.BakeEnvironment(t.Context(), layout, condition, options)
			if err != nil {
				t.Error(err)
				return
			}
			if !bytes.Equal(actual.Image.Pix, want[i].Image.Pix) || actual.Stats.Rays != want[i].Stats.Rays {
				t.Error("concurrent environment variant changed pixels or ray count")
			}
		})
	}
	group.Wait()
	prepared := baker.prepared
	if _, err := baker.BakeEnvironment(t.Context(), layout, settings[0], options); err != nil || prepared != baker.prepared {
		t.Fatalf("environment preparation not reused: %v", err)
	}
}
