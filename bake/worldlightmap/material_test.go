package worldlightmapbake

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"testing"
)

func TestDefaultMaterialMatchesExplicitAtlasAlbedo(t *testing.T) {
	d, l, _ := room(t, 0)
	for i := range d.Sectors {
		s := &d.Sectors[i]
		s.FloorMaterial, s.CeilingMaterial = 0, 0
		for j := range s.Walls {
			s.Walls[j].Material = 0
		}
	}
	options := Options{Workers: 1, Samples: 16, Bounces: 2, Seed: 22}
	implicit, err := Bake(context.Background(), d, l, nil, options)
	if err != nil {
		t.Fatal(err)
	}
	materials := []Material{{ID: 0, Albedo: solid(color.NRGBA{192, 192, 192, 255})}}
	explicit, err := Bake(context.Background(), d, l, materials, options)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(implicit.Image.Pix, explicit.Image.Pix) || implicit.ReflectanceSHA256 != explicit.ReflectanceSHA256 {
		t.Fatal("omitted default differs from CLI default albedo")
	}
	identity, err := ReflectanceDigest(d, nil)
	if err != nil || identity != implicit.ReflectanceSHA256 {
		t.Fatalf("default identity: %s %v", identity, err)
	}
	materials[0].Albedo = solid(color.NRGBA{255, 0, 0, 255})
	changed, err := Bake(context.Background(), d, l, materials, options)
	if err != nil {
		t.Fatal(err)
	}
	if changed.ReflectanceSHA256 == implicit.ReflectanceSHA256 || bytes.Equal(changed.Image.Pix, implicit.Image.Pix) {
		t.Fatal("explicit default albedo did not affect transport and identity")
	}
	if _, err := ReflectanceDigest(d, append(materials, materials[0])); err == nil {
		t.Fatal("duplicate default accepted")
	}
	d.Sectors[0].FloorMaterial = 99
	if _, err := Bake(context.Background(), d, l, nil, options); err == nil {
		t.Fatal("missing nondefault albedo accepted")
	}
}

type cancellingImage struct {
	cancel context.CancelFunc
	calls  int
}

func (*cancellingImage) ColorModel() color.Model { return color.NRGBAModel }
func (*cancellingImage) Bounds() image.Rectangle { return image.Rect(0, 0, 1024, 1024) }
func (im *cancellingImage) At(int, int) color.Color {
	im.calls++
	if im.calls == 1 {
		im.cancel()
	}
	return color.NRGBA{A: 255}
}

func TestBakeCancelsDuringReflectancePreparation(t *testing.T) {
	d, l, materials := room(t, 0)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	im := &cancellingImage{cancel: cancel}
	materials[0].Albedo = im
	result, err := Bake(ctx, d, l, materials, Options{Workers: 1})
	if !errors.Is(err, context.Canceled) || result.Image != nil {
		t.Fatalf("cancelled preparation: %+v %v", result, err)
	}
	if im.calls == 0 || im.calls > 1024 {
		t.Fatalf("cancellation took %d pixel reads", im.calls)
	}
}
