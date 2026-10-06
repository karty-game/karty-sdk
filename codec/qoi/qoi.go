// Package qoi provides a bounded adapter around Karty's pinned QOI codec.
package qoi

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"

	hchargois "github.com/hchargois/qoi"
	"github.com/karty-game/karty-sdk/format/asset"
)

const (
	Magic            = "qoif"
	ChannelsRGB      = uint8(3)
	ChannelsRGBA     = uint8(4)
	ColorspaceSRGB   = uint8(0)
	ColorspaceLinear = uint8(1)

	headerSize           = 14
	endMarkerSize        = 8
	decodedBytesPerPixel = 4
	maximumBytesPerPixel = 5
	MaxEncodedBytes      = asset.MaxTexturePixels*maximumBytesPerPixel + headerSize + endMarkerSize
)

var (
	ErrInvalid = errors.New("QOI data is invalid")
	endMarker  = [endMarkerSize]byte{0, 0, 0, 0, 0, 0, 0, 1}
)

// Metadata describes validated QOI content and its bounded decoded cost.
type Metadata struct {
	Width        uint32
	Height       uint32
	Channels     uint8
	Colorspace   uint8
	DecodedBytes uint64
}

// Options selects QOI header metadata. It does not change pixel encoding.
type Options struct {
	Channels   uint8
	Colorspace uint8
}

// Inspect validates a QOI header, resource bounds, and the exact end marker
// without allocating decoded pixels. Decode performs complete stream validation.
func Inspect(encoded []byte) (Metadata, error) {
	if len(encoded) < headerSize+endMarkerSize || uint64(len(encoded)) > uint64(MaxEncodedBytes) ||
		string(encoded[:4]) != Magic || !bytes.Equal(encoded[len(encoded)-endMarkerSize:], endMarker[:]) {
		return Metadata{}, ErrInvalid
	}

	metadata := Metadata{
		Width:      binary.BigEndian.Uint32(encoded[4:8]),
		Height:     binary.BigEndian.Uint32(encoded[8:12]),
		Channels:   encoded[12],
		Colorspace: encoded[13],
	}
	if metadata.Width == 0 || metadata.Height == 0 ||
		metadata.Width > asset.MaxTextureDimension || metadata.Height > asset.MaxTextureDimension ||
		(metadata.Channels != ChannelsRGB && metadata.Channels != ChannelsRGBA) ||
		(metadata.Colorspace != ColorspaceSRGB && metadata.Colorspace != ColorspaceLinear) {
		return Metadata{}, ErrInvalid
	}

	pixels := uint64(metadata.Width) * uint64(metadata.Height)
	metadata.DecodedBytes = pixels * decodedBytesPerPixel
	maximumEncoded := pixels*maximumBytesPerPixel + headerSize + endMarkerSize
	if pixels > asset.MaxTexturePixels || metadata.DecodedBytes > asset.MaxDecodedTextureBytes ||
		uint64(len(encoded)) > maximumEncoded {
		return Metadata{}, ErrInvalid
	}

	return metadata, nil
}

// Validate checks header bounds and the complete stream without allocating
// decoded pixels. Its metadata can be used to preflight a shared image budget.
func Validate(encoded []byte) (Metadata, error) {
	metadata, err := Inspect(encoded)
	if err != nil || !validStream(encoded, metadata.DecodedBytes/decodedBytesPerPixel) {
		return Metadata{}, ErrInvalid
	}
	return metadata, nil
}

// Decode returns straight-alpha NRGBA pixels only after bounded preflight,
// allocation-free stream validation and complete dependency decoding. Panics
// from malformed dependency input are contained and reported as ErrInvalid.
func Decode(encoded []byte) (metadata Metadata, decoded *image.NRGBA, err error) {
	defer func() {
		if recover() != nil {
			metadata, decoded, err = Metadata{}, nil, ErrInvalid
		}
	}()

	metadata, err = Validate(encoded)
	if err != nil {
		return Metadata{}, nil, err
	}
	imageValue, decodeErr := hchargois.DecodeBytes(encoded)
	if decodeErr != nil {
		return Metadata{}, nil, ErrInvalid
	}
	decoded, ok := imageValue.(*image.NRGBA)
	if !ok || decoded.Rect != image.Rect(0, 0, int(metadata.Width), int(metadata.Height)) ||
		decoded.Stride != int(metadata.Width)*decodedBytesPerPixel ||
		uint64(len(decoded.Pix)) != metadata.DecodedBytes {
		return Metadata{}, nil, ErrInvalid
	}

	return metadata, decoded, nil
}

// validStream checks operation boundaries and the exact output pixel count
// before the dependency can allocate. In particular, an overlong final run
// would otherwise grow its buffer beyond the inspected decoded-byte budget.
// Inspect has already checked the header and end marker; marker bytes must
// never satisfy a truncated operation's payload.
func validStream(encoded []byte, pixels uint64) bool {
	stream := encoded[headerSize : len(encoded)-endMarkerSize]
	var decoded uint64
	for len(stream) != 0 {
		op := stream[0]
		length, count := 1, uint64(1)
		switch {
		case op == 0xfe: // QOI_OP_RGB
			length = 4
		case op == 0xff: // QOI_OP_RGBA
			length = 5
		case op&0xc0 == 0x80: // QOI_OP_LUMA
			length = 2
		case op&0xc0 == 0xc0: // QOI_OP_RUN (RGB/RGBA handled above)
			count = uint64(op&0x3f) + 1
		}
		if len(stream) < length || count > pixels-decoded {
			return false
		}
		decoded += count
		stream = stream[length:]
	}

	return decoded == pixels
}

// Encode converts a bounded image into deterministic QOI and validates the
// complete output before returning it. Panics from custom image implementations
// or the dependency are contained and reported as ErrInvalid.
func Encode(source image.Image, options Options) (encoded []byte, metadata Metadata, err error) {
	defer func() {
		if recover() != nil {
			encoded, metadata, err = nil, Metadata{}, ErrInvalid
		}
	}()

	if source == nil || (options.Channels != ChannelsRGB && options.Channels != ChannelsRGBA) ||
		(options.Colorspace != ColorspaceSRGB && options.Colorspace != ColorspaceLinear) {
		return nil, Metadata{}, ErrInvalid
	}

	width, height, valid := boundedDimensions(source.Bounds())
	if !valid {
		return nil, Metadata{}, ErrInvalid
	}

	writer := boundedWriter{maximum: uint64(width)*uint64(height)*maximumBytesPerPixel + headerSize + endMarkerSize}
	encodeErr := hchargois.Encode(&writer, source, &hchargois.Options{
		Channels:   int(options.Channels),
		Colorspace: int(options.Colorspace),
	})
	if encodeErr != nil || writer.failed {
		return nil, Metadata{}, ErrInvalid
	}

	encoded = writer.buffer.Bytes()
	metadata, validateErr := Validate(encoded)
	if validateErr != nil || metadata.Width != width || metadata.Height != height ||
		metadata.Channels != options.Channels || metadata.Colorspace != options.Colorspace {
		return nil, Metadata{}, ErrInvalid
	}

	// This local writer owns its buffer and is never reused after return.
	return encoded, metadata, nil
}

func boundedDimensions(bounds image.Rectangle) (uint32, uint32, bool) {
	if bounds.Max.X <= bounds.Min.X || bounds.Max.Y <= bounds.Min.Y {
		return 0, 0, false
	}

	width := uint64(bounds.Max.X) - uint64(bounds.Min.X)
	height := uint64(bounds.Max.Y) - uint64(bounds.Min.Y)
	if width > asset.MaxTextureDimension || height > asset.MaxTextureDimension ||
		width*height > asset.MaxTexturePixels || width*height*decodedBytesPerPixel > asset.MaxDecodedTextureBytes {
		return 0, 0, false
	}

	return uint32(width), uint32(height), true
}

type boundedWriter struct {
	buffer  bytes.Buffer
	maximum uint64
	failed  bool
}

func (writer *boundedWriter) Write(data []byte) (int, error) {
	if writer.failed || uint64(writer.buffer.Len())+uint64(len(data)) > writer.maximum {
		writer.failed = true

		return 0, ErrInvalid
	}

	return writer.buffer.Write(data)
}
