package worldmaterial_test

import (
	"image"
	"testing"

	"github.com/karty-game/karty-sdk/codec/qoi"
	"github.com/karty-game/karty-sdk/format/worldmaterial"
)

func BenchmarkPairValidate(b *testing.B) {
	l, err := worldmaterial.NewLayout([]uint32{1, 2, 3, 4})
	if err != nil {
		b.Fatal(err)
	}
	im := image.NewNRGBA(image.Rect(0, 0, l.Width, l.Height))
	for i := 3; i < len(im.Pix); i += 4 {
		im.Pix[i] = 255
	}
	albedo, _, err := qoi.Encode(im, qoi.Options{Channels: 4, Colorspace: 0})
	if err != nil {
		b.Fatal(err)
	}
	data, _, err := qoi.Encode(im, qoi.Options{Channels: 4, Colorspace: 1})
	if err != nil {
		b.Fatal(err)
	}
	pair := worldmaterial.Pair{Layout: l, Albedo: albedo, Data: data}
	b.ReportAllocs()
	for b.Loop() {
		if err := pair.Validate(); err != nil {
			b.Fatal(err)
		}
	}
}
