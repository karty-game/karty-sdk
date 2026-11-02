package worldreflection

import (
	"github.com/karty-game/karty-sdk/codec/ktx2"
	"github.com/karty-game/karty-sdk/format/level"
	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldgpuchunk"
	"github.com/karty-game/karty-sdk/internal/binwire"
)

const (
	GPUFeature    = "world/reflection-gpu@1"
	GPUProducer   = "karty-static-cubemap-gpu@1"
	GPUMipLevels  = 4
	GPUImageBytes = ImageBytes + ImageBytes/4 + ImageBytes/16 + ImageBytes/64
)

// Prepared contains metadata and one final sRGB RGBA8+Zstd array. Every probe
// occupies one layer; its four explicit mip levels preserve all six faces.
// Metadata.Probes contain no QOI images, so shipped levels have one image copy.
type Prepared struct {
	Metadata Bundle
	Texture  []byte

	// DrawProbes contains final LE32 slots in dense CellID/draw order.
	// Each slice borrows immutable wire bytes after decoding.
	DrawProbes [][]byte
}

func (prepared *Prepared) Validate() error {
	_, err := prepared.inspect()

	return err
}

func (prepared *Prepared) inspect() (ktx2.Info, error) {
	if err := prepared.Metadata.validate(true); err != nil || len(prepared.Metadata.Probes) == 0 {
		return ktx2.Info{}, ErrCubemaps
	}

	if !validDrawProbes(prepared.DrawProbes, prepared.Metadata.Probes) {
		return ktx2.Info{}, ErrCubemaps
	}

	info, err := ktx2.InspectRGBA8Array(prepared.Texture)
	if err != nil || !info.SRGB || info.Width != FaceSize*Faces || info.Height != FaceSize ||
		uint64(info.Layers) != uint64(len(prepared.Metadata.Probes)) || len(info.Levels) != GPUMipLevels ||
		info.RGBABytes != uint64(len(prepared.Metadata.Probes))*GPUImageBytes || info.RGBABytes > MaxDecodedBytes {
		return ktx2.Info{}, ErrCubemaps
	}

	return info, nil
}

func (prepared *Prepared) ValidateSource(envelope level.Envelope, document *world.Document) error {
	if !prepared.sourceMatches(envelope, document) {
		return ErrCubemaps
	}

	return prepared.Validate()
}

func (prepared *Prepared) sourceMatches(envelope level.Envelope, document *world.Document) bool {
	if document == nil {
		return false
	}

	solids := 0
	if document.StaticSolids != nil {
		solids = len(document.StaticSolids.Items)
	}

	if len(prepared.Metadata.Sectors) != len(document.Sectors) || len(prepared.Metadata.Solids) != solids ||
		prepared.Metadata.InputSHA256 != InputDigest(envelope) {
		return false
	}

	return true
}

func EncodePrepared(prepared Prepared) ([]byte, error) {
	if err := prepared.Validate(); err != nil {
		return nil, err
	}

	writer := binwire.NewWriter("KRG1", MaxEncodedSize)
	writeBundle(writer, prepared.Metadata)
	writer.Count(len(prepared.DrawProbes), false, worldgpuchunk.MaxCells)

	for _, slots := range prepared.DrawProbes {
		writer.Count(len(slots)/4, false, worldgpuchunk.MaxDraws)
		writer.Raw(slots)
	}

	writer.Count(len(prepared.Texture), false, MaxEncodedSize)
	writer.Raw(prepared.Texture)

	data, err := writer.Bytes()
	if err != nil {
		return nil, ErrCubemaps
	}

	return data, nil
}

// DecodePrepared scans complete lengths before allocating metadata tables.
// Texture bytes remain borrowed from the admitted entry and are immutable.
func DecodePrepared(data []byte) (Prepared, error) {
	prepared, err := decodePrepared(data)
	if err != nil {
		return Prepared{}, err
	}

	if err := prepared.Validate(); err != nil {
		return Prepared{}, err
	}

	return prepared, nil
}

// DecodePreparedSource admits a newly loaded asset and its exact source binding
// once, returning the already checked texture layout for allocation preflight.
func DecodePreparedSource(data []byte, envelope level.Envelope, document *world.Document) (Prepared, ktx2.Info, error) {
	prepared, err := decodePrepared(data)
	if err != nil || !prepared.sourceMatches(envelope, document) {
		return Prepared{}, ktx2.Info{}, ErrCubemaps
	}

	info, err := prepared.inspect()
	if err != nil {
		return Prepared{}, ktx2.Info{}, err
	}

	return prepared, info, nil
}

func decodePrepared(data []byte) (Prepared, error) {
	scan := binwire.NewReader(data, "KRG1", MaxEncodedSize)
	readBundle(scan, false)
	readDrawProbes(scan, false)
	size, _ := scan.Count(MaxEncodedSize, 1, 0)
	scan.Raw(size)

	if scan.Done() != nil {
		return Prepared{}, ErrCubemaps
	}

	reader := binwire.NewReader(data, "KRG1", MaxEncodedSize)
	prepared := Prepared{Metadata: readBundle(reader, true)}
	prepared.DrawProbes = readDrawProbes(reader, true)
	size, _ = reader.Count(MaxEncodedSize, 1, 0)

	prepared.Texture = reader.Raw(size)
	if reader.Done() != nil {
		return Prepared{}, ErrCubemaps
	}

	return prepared, nil
}
