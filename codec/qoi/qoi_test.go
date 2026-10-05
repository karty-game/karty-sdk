package qoi_test

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	"testing"

	"github.com/karty-game/karty-sdk/codec/qoi"
	"github.com/karty-game/karty-sdk/format/asset"
)

// A 1x1 opaque red image encoded according to the QOI reference specification:
// header, QOI_OP_RGB, then the required eight-byte end marker.
const referenceGolden = "716f696600000001000000010400feff00000000000000000001"

// The pinned encoder selects the shorter QOI_OP_DIFF representation for the
// same red pixel. Both streams are valid according to the reference format.
const encoderGolden = "716f6966000000010000000104005a0000000000000001"

func TestReferenceGoldenInteroperability(t *testing.T) {
	t.Parallel()

	golden, err := hex.DecodeString(referenceGolden)
	if err != nil {
		t.Fatal(err)
	}
	metadata, decoded, err := qoi.Decode(golden)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Width != 1 || metadata.Height != 1 || metadata.Channels != qoi.ChannelsRGBA ||
		metadata.Colorspace != qoi.ColorspaceSRGB || metadata.DecodedBytes != 4 {
		t.Fatalf("metadata = %+v", metadata)
	}
	if got := decoded.NRGBAAt(0, 0); got != (color.NRGBA{R: 255, A: 255}) {
		t.Fatalf("decoded pixel = %#v", got)
	}

	source := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	source.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	encoded, encodedMetadata, err := qoi.Encode(source, qoi.Options{
		Channels: qoi.ChannelsRGBA, Colorspace: qoi.ColorspaceSRGB,
	})
	if err != nil {
		t.Fatal(err)
	}
	wantEncoded, err := hex.DecodeString(encoderGolden)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, wantEncoded) || encodedMetadata != metadata {
		t.Fatalf("Encode() = %x, %+v", encoded, encodedMetadata)
	}
}

func TestDecodeRejectsMalformedAndUnsafeData(t *testing.T) {
	t.Parallel()

	golden, err := hex.DecodeString(referenceGolden)
	if err != nil {
		t.Fatal(err)
	}

	for name, mutate := range map[string]func([]byte) []byte{
		"zero width": func(data []byte) []byte {
			binary.BigEndian.PutUint32(data[4:8], 0)

			return data
		},
		"dimension limit": func(data []byte) []byte {
			binary.BigEndian.PutUint32(data[4:8], asset.MaxTextureDimension+1)

			return data
		},
		"pixel limit": func(data []byte) []byte {
			binary.BigEndian.PutUint32(data[4:8], asset.MaxTextureDimension)
			binary.BigEndian.PutUint32(data[8:12], asset.MaxTextureDimension)

			return data
		},
		"channels":   func(data []byte) []byte { data[12] = 2; return data },
		"colorspace": func(data []byte) []byte { data[13] = 2; return data },
		"bad end marker": func(data []byte) []byte {
			data[len(data)-1] = 0

			return data
		},
		"truncated operation": func(data []byte) []byte {
			return append(data[:14], data[len(data)-8:]...)
		},
		"trailing before end": func(data []byte) []byte {
			return append(append(data, 0), data[len(data)-8:]...)
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			data := mutate(bytes.Clone(golden))
			_, _, err := qoi.Decode(data)
			if err == nil {
				t.Fatal("Decode() accepted malformed QOI")
			}
		})
	}
}

func TestMalformedStreamsRejectBeforePixelAllocation(t *testing.T) {
	golden, err := hex.DecodeString(referenceGolden)
	if err != nil {
		t.Fatal(err)
	}
	for name, operation := range map[string][]byte{
		"missing pixel":  {},
		"run overflow":   {0xfd},
		"extra pixel":    {0xc0, 0xc0},
		"truncated RGB":  {0xfe, 255, 0},
		"truncated RGBA": {0xff, 128, 128, 73},
		"truncated luma": {0x80},
	} {
		t.Run(name, func(t *testing.T) {
			encoded := append(bytes.Clone(golden[:14]), operation...)
			encoded = append(encoded, golden[len(golden)-8:]...)
			if _, err := qoi.Inspect(encoded); err != nil {
				t.Fatalf("fixture should pass header-only inspection: %v", err)
			}
			_, decoded, err := qoi.Decode(encoded)
			if !errors.Is(err, qoi.ErrInvalid) || decoded != nil {
				t.Fatalf("Decode() = %v, %v", decoded, err)
			}
			// In particular, an oversized run must not grow a dependency buffer
			// beyond the allocation budget declared by the QOI header.
			if allocations := testing.AllocsPerRun(1, func() { _, _, _ = qoi.Decode(encoded) }); allocations != 0 {
				t.Fatalf("malformed stream allocated %g times before rejection", allocations)
			}
		})
	}
}

func TestReferenceStreamOperationsPreserveStraightData(t *testing.T) {
	t.Parallel()

	// A reference stream, not produced by the pinned encoder: maximum RUN,
	// INDEX (including the initial RUN's hash), RGBA, DIFF, LUMA and RGB.
	// Alpha zero must not discard or premultiply the material data channels.
	encoded := make([]byte, 14)
	copy(encoded, qoi.Magic)
	binary.BigEndian.PutUint32(encoded[4:8], 69)
	binary.BigEndian.PutUint32(encoded[8:12], 1)
	encoded[12], encoded[13] = qoi.ChannelsRGBA, qoi.ColorspaceLinear
	encoded = append(encoded,
		0xfd, 53,
		0xff, 128, 255, 73, 0, 58,
		0x79, 0xa1, 0x97,
		0xfe, 128, 128, 73,
		0xff, 192, 192, 192, 255,
		0, 0, 0, 0, 0, 0, 0, 1,
	)
	metadata, decoded, err := qoi.Decode(encoded)
	if err != nil || metadata.DecodedBytes != 69*4 {
		t.Fatalf("reference stream decode: %+v, %v", metadata, err)
	}
	for x := range 63 {
		if got := decoded.NRGBAAt(x, 0); got != (color.NRGBA{A: 255}) {
			t.Fatalf("initial RUN/INDEX pixel %d = %+v", x, got)
		}
	}
	for i, want := range []color.NRGBA{
		{R: 128, G: 255, B: 73},
		{R: 128, G: 255, B: 73},
		{R: 129, G: 255, B: 72},
		{R: 131, G: 0, B: 72},
		{R: 128, G: 128, B: 73},
		{R: 192, G: 192, B: 192, A: 255},
	} {
		if got := decoded.NRGBAAt(63+i, 0); got != want {
			t.Fatalf("reference data pixel %d = %+v, want %+v", i, got, want)
		}
	}
}

func TestEncodeRejectsInvalidInputsAndContainsPanics(t *testing.T) {
	t.Parallel()

	valid := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	for name, test := range map[string]struct {
		image.Image
		qoi.Options
	}{
		"nil image":   {},
		"empty image": {Image: image.NewNRGBA(image.Rectangle{}), Options: validOptions()},
		"large dimensions": {
			Image:   imageWithBounds{rectangle: image.Rect(0, 0, asset.MaxTextureDimension+1, 1)},
			Options: validOptions(),
		},
		"invalid channels":   {Image: valid, Options: qoi.Options{Channels: 2, Colorspace: qoi.ColorspaceSRGB}},
		"invalid colorspace": {Image: valid, Options: qoi.Options{Channels: qoi.ChannelsRGBA, Colorspace: 2}},
		"panicking image":    {Image: imageWithBounds{rectangle: image.Rect(0, 0, 1, 1), panicAt: true}, Options: validOptions()},
	} {
		if _, _, err := qoi.Encode(test.Image, test.Options); err == nil {
			t.Errorf("Encode() accepted %s", name)
		}
	}
}

func TestEncodeSupportsNonZeroBoundsAndHeaderOptions(t *testing.T) {
	t.Parallel()

	source := image.NewNRGBA(image.Rect(-2, -3, 0, -2))
	source.SetNRGBA(-2, -3, color.NRGBA{G: 255, A: 255})
	source.SetNRGBA(-1, -3, color.NRGBA{B: 255, A: 255})
	encoded, metadata, err := qoi.Encode(source, qoi.Options{
		Channels: qoi.ChannelsRGB, Colorspace: qoi.ColorspaceLinear,
	})
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Width != 2 || metadata.Height != 1 || metadata.Channels != qoi.ChannelsRGB ||
		metadata.Colorspace != qoi.ColorspaceLinear {
		t.Fatalf("metadata = %+v", metadata)
	}
	if inspected, err := qoi.Inspect(encoded); err != nil || inspected != metadata {
		t.Fatalf("Inspect() = %+v, %v", inspected, err)
	}
}

func FuzzInspectAndDecode(f *testing.F) {
	golden, err := hex.DecodeString(referenceGolden)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(golden)
	f.Add([]byte(qoi.Magic))
	f.Add(append([]byte(nil), golden[:len(golden)-1]...))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = qoi.Inspect(data)
		_, _, _ = qoi.Decode(data)
	})
}

func validOptions() qoi.Options {
	return qoi.Options{Channels: qoi.ChannelsRGBA, Colorspace: qoi.ColorspaceSRGB}
}

type imageWithBounds struct {
	rectangle image.Rectangle
	panicAt   bool
}

func (source imageWithBounds) ColorModel() color.Model {
	return color.NRGBAModel
}

func (source imageWithBounds) Bounds() image.Rectangle {
	return source.rectangle
}

func (source imageWithBounds) At(_, _ int) color.Color {
	if source.panicAt {
		panic("malicious image")
	}

	return color.NRGBA{A: 255}
}

func TestValidateCompleteStreamWithoutAllocation(t *testing.T) {
	encoded, err := hex.DecodeString(referenceGolden)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := qoi.Validate(encoded)
	if err != nil || metadata.DecodedBytes != 4 {
		t.Fatalf("complete preflight: %v", err)
	}
	if allocations := testing.AllocsPerRun(100, func() { _, _ = qoi.Validate(encoded) }); allocations != 0 {
		t.Fatalf("valid stream preflight allocated %g times", allocations)
	}
	malformed := append(bytes.Clone(encoded[:14]), 0xfd)
	malformed = append(malformed, encoded[len(encoded)-8:]...)
	if _, err := qoi.Inspect(malformed); err != nil {
		t.Fatal(err)
	}
	if _, err := qoi.Validate(malformed); err == nil {
		t.Fatal("header-valid overlong run accepted")
	}
	if allocations := testing.AllocsPerRun(100, func() { _, _ = qoi.Validate(malformed) }); allocations != 0 {
		t.Fatalf("malformed stream preflight allocated %g times", allocations)
	}
}
