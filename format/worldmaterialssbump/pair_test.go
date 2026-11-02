package worldmaterialssbump

import (
	"bytes"
	"image"
	"image/color"
	"testing"

	"github.com/karty-game/karty-sdk/codec/qoi"
	"github.com/karty-game/karty-sdk/format/worldmaterial"
)

func fixture(t *testing.T, mips bool) (worldmaterial.Layout, Pair) {
	t.Helper()
	l, err := worldmaterial.NewLayout([]uint32{7, 3})
	if mips {
		l, err = worldmaterial.NewMipLayout(l)
	}
	if err != nil {
		t.Fatal(err)
	}
	d, err := NewDescriptor(l, []uint32{7})
	if err != nil {
		t.Fatal(err)
	}
	p := Pair{Descriptor: d, Atlas: encodeFlat(t, l.Width, l.Height)}
	if mips {
		w, h := l.MipDimensions()
		p.MipTail = encodeFlat(t, w, h)
	}
	return l, p
}

func encodeFlat(t *testing.T, w, h int) []byte {
	t.Helper()
	pixels := image.NewNRGBA(image.Rect(0, 0, w, h))
	for o := 0; o < len(pixels.Pix); o += 4 {
		copy(pixels.Pix[o:o+4], []byte{85, 85, 85, 255})
	}
	return encodePixels(t, pixels)
}

func encodePixels(t *testing.T, p *image.NRGBA) []byte {
	t.Helper()
	b, _, err := qoi.Encode(p, qoi.Options{Channels: qoi.ChannelsRGBA, Colorspace: qoi.ColorspaceLinear})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestDescriptorBindingAndCompletePayload(t *testing.T) {
	l, p := fixture(t, false)
	b, err := EncodeDescriptor(p.Descriptor)
	if err != nil {
		t.Fatal(err)
	}
	d, err := DecodeDescriptor(b)
	if err != nil || d.Validate(l) != nil {
		t.Fatal("round trip", err)
	}
	d.MaterialIDs[0] = 3
	if p.Descriptor.MaterialIDs[0] != 7 {
		t.Fatal("decoded IDs alias input")
	}
	for _, bad := range [][]byte{nil, b[:len(b)-1], append(bytes.Clone(b), 0), append([]byte("KSB2"), b[4:]...)} {
		if _, err := DecodeDescriptor(bad); err == nil {
			t.Fatal("malformed descriptor accepted")
		}
	}
	for _, ids := range [][]uint32{nil, {7, 7}, {3, 7}, {99}} {
		if _, err := NewDescriptor(l, ids); err == nil {
			t.Fatal("invalid IDs accepted", ids)
		}
	}
	l.Materials[0].Strengths = &worldmaterial.Strengths{Normal: 1}
	if p.Descriptor.Validate(l) == nil {
		t.Fatal("different strengths accepted under old digest")
	}
}

func TestPairCompleteValidationAndReservedPixels(t *testing.T) {
	l, p := fixture(t, true)
	decoded, err := p.Decode(l)
	if err != nil {
		t.Fatal(err)
	}
	// Coefficient energy is authored, not constrained by byte-sum heuristics.
	r := l.Materials[0]
	decoded.Atlas.SetNRGBA(r.X, r.Y, color.NRGBA{255, 255, 255, 255})
	p.Atlas = encodePixels(t, decoded.Atlas)
	if p.Validate(l) != nil {
		t.Fatal("authored energy rejected")
	}
	for _, kind := range []string{"alpha", "padding", "unselected", "colorspace", "missing-tail", "bad-stream", "wrong-digest", "tail-albedo-cell"} {
		t.Run(kind, func(t *testing.T) {
			l, p := fixture(t, true)
			decoded, err := p.Decode(l)
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "alpha":
				decoded.Atlas.SetNRGBA(l.Materials[0].X, l.Materials[0].Y, color.NRGBA{85, 85, 85, 0})
				p.Atlas = encodePixels(t, decoded.Atlas)
			case "padding":
				decoded.Atlas.SetNRGBA(0, 0, color.NRGBA{0, 0, 0, 255})
				p.Atlas = encodePixels(t, decoded.Atlas)
			case "unselected":
				r := l.Materials[1]
				decoded.Atlas.SetNRGBA(r.X, r.Y, color.NRGBA{0, 0, 0, 255})
				p.Atlas = encodePixels(t, decoded.Atlas)
			case "colorspace":
				p.Atlas[13] = qoi.ColorspaceSRGB
			case "missing-tail":
				p.MipTail = nil
			case "bad-stream":
				p.MipTail = append(bytes.Clone(p.MipTail[:14]), p.MipTail[len(p.MipTail)-8:]...)
			case "wrong-digest":
				p.Descriptor.LayoutSHA256[0] ^= 1
			case "tail-albedo-cell":
				r, _ := l.MipRect(1, 0, false)
				decoded.MipTail.SetNRGBA(r.X, r.Y, color.NRGBA{0, 0, 0, 255})
				p.MipTail = encodePixels(t, decoded.MipTail)
			}
			got, err := p.Decode(l)
			if err == nil || got.Atlas != nil || got.MipTail != nil {
				t.Fatal("partial or malformed pair accepted")
			}
		})
	}
}

func TestTogetherRejectsIncompleteBase(t *testing.T) {
	l, p := fixture(t, false)
	base := worldmaterial.Pair{Layout: l, Data: encodeFlat(t, l.Width, l.Height)}
	if p.ValidateTogether(base) == nil {
		t.Fatal("missing base albedo accepted")
	}
	base.Albedo = bytes.Clone(base.Data)
	base.Albedo[13] = qoi.ColorspaceSRGB
	if err := p.ValidateTogether(base); err != nil {
		t.Fatal(err)
	}
	base.Data = append(bytes.Clone(base.Data[:14]), base.Data[len(base.Data)-8:]...)
	if p.ValidateTogether(base) == nil {
		t.Fatal("incomplete base stream accepted")
	}
}

func FuzzDescriptor(f *testing.F) {
	f.Add([]byte("KSB1"))
	f.Fuzz(func(t *testing.T, b []byte) {
		d, err := DecodeDescriptor(b)
		if err != nil {
			return
		}
		encoded, err := EncodeDescriptor(d)
		if err != nil || !bytes.Equal(b, encoded) {
			t.Fatal("noncanonical round trip")
		}
	})
}
