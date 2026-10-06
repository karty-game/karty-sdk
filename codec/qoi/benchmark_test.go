package qoi_test

import (
	"github.com/karty-game/karty-sdk/codec/qoi"
	"image"
	"testing"
)

func BenchmarkQOI(b *testing.B) {
	im := image.NewNRGBA(image.Rect(0, 0, 1024, 1024))
	for i := 3; i < len(im.Pix); i += 4 {
		im.Pix[i] = 255
	}
	options := qoi.Options{Channels: qoi.ChannelsRGBA, Colorspace: qoi.ColorspaceSRGB}
	encoded, _, err := qoi.Encode(im, options)
	if err != nil {
		b.Fatal(err)
	}
	b.Run("Encode1024", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, _, err := qoi.Encode(im, options); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("Validate1024", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := qoi.Validate(encoded); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("Decode1024", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, _, err := qoi.Decode(encoded); err != nil {
				b.Fatal(err)
			}
		}
	})
}
