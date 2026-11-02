package ktx2

import (
	"bytes"
	"encoding/binary"
	"math/bits"

	"github.com/karty-game/karty-sdk/format/asset"
)

// InspectRGBA8 validates the separate lossless, linear RGBA8_UNORM + Zstandard
// array profile. It has exactly one level, straight alpha and no pixel transforms.
// Consumers must completely decompress and verify the frame before publication.
func InspectRGBA8(encoded []byte) (Info, error) {
	info, err := InspectRGBA8Array(encoded)
	if err != nil || info.SRGB || len(info.Levels) != 1 {
		return Info{}, ErrInvalid
	}
	return info, nil
}

// InspectRGBA8Array validates prepared lossless RGBA8 arrays with linear or
// sRGB RGB transfer and explicit mip levels. Every level already contains all
// layers in GPU order; consumers only decompress complete validated frames.
func InspectRGBA8Array(encoded []byte) (Info, error) {
	if len(encoded) < HeaderSize || len(encoded) > MaxEncodedBytes || !bytes.Equal(encoded[:12], identifier) {
		return Info{}, ErrInvalid
	}
	u32 := func(offset int) uint32 { return binary.LittleEndian.Uint32(encoded[offset:]) }
	info := Info{Width: u32(20), Height: u32(24), Layers: u32(32), HasAlpha: true, SRGB: u32(12) == 43}
	count := u32(40)
	if (u32(12) != 37 && u32(12) != 43) || u32(16) != 1 || u32(28) != 0 || u32(36) != 1 || count < 1 || count > MaxLevels || u32(44) != 2 ||
		binary.LittleEndian.Uint64(encoded[64:]) != 0 ||
		binary.LittleEndian.Uint64(encoded[72:]) != 0 ||
		info.Width == 0 ||
		info.Height == 0 ||
		info.Width > asset.MaxTextureDimension ||
		info.Height > asset.MaxTextureDimension ||
		info.Layers == 0 ||
		info.Layers > MaxLayers ||
		count > uint32(bits.Len32(max(info.Width, info.Height))) ||
		len(encoded) < HeaderSize+headerLevelSize*int(count) {
		return Info{}, ErrInvalid
	}
	if uint64(info.Width)*uint64(info.Height)*uint64(info.Layers)*4 > asset.MaxDecodedTextures {
		return Info{}, ErrInvalid
	}
	dfd := section{uint64(u32(48)), uint64(u32(52))}
	kvd := section{uint64(u32(56)), uint64(u32(60))}
	indexEnd := uint64(HeaderSize + headerLevelSize*int(count))
	if !validSection(dfd, indexEnd, uint64(len(encoded))) || dfd.length != 92 || dfd.offset%4 != 0 ||
		(kvd.length == 0 && kvd.offset != 0) || (kvd.length != 0 && (!validSection(kvd, indexEnd, uint64(len(encoded))) || kvd.offset%4 != 0 || kvd.length > 64<<10)) {
		return Info{}, ErrInvalid
	}
	if !validRGBA8DFD(encoded[dfd.offset:dfd.offset+dfd.length], info.SRGB) {
		return Info{}, ErrInvalid
	}
	sections := []section{{0, indexEnd}, dfd}
	if kvd.length > 0 {
		if inspectMetadata(encoded[kvd.offset:kvd.offset+kvd.length]) != nil {
			return Info{}, ErrInvalid
		}
		sections = append(sections, kvd)
	}
	for index := range count {
		offset := HeaderSize + int(index)*headerLevelSize
		level := Level{binary.LittleEndian.Uint64(encoded[offset:]), binary.LittleEndian.Uint64(encoded[offset+8:]),
			binary.LittleEndian.Uint64(encoded[offset+16:])}
		expected := uint64(max(info.Width>>index, 1)) * uint64(max(info.Height>>index, 1)) * uint64(info.Layers) * 4
		if !validSection(section{level.Offset, level.Length}, indexEnd, uint64(len(encoded))) || level.UncompressedLength != expected ||
			inspectZstd(encoded[level.Offset:level.Offset+level.Length], expected) != nil {
			return Info{}, ErrInvalid
		}
		info.RGBABytes += expected
		if info.RGBABytes > asset.MaxDecodedTextures {
			return Info{}, ErrInvalid
		}
		sections = append(sections, section{level.Offset, level.Length})
		info.Levels = append(info.Levels, level)
	}
	if inspectSectionCoverage(encoded, sections) != nil {
		return Info{}, ErrInvalid
	}
	info.InflatedBytes = info.RGBABytes
	return info, nil
}
func validRGBA8DFD(d []byte, srgb bool) bool {
	transfer := byte(1)
	if srgb {
		transfer = 2
	}
	if binary.LittleEndian.Uint32(d) != 92 || binary.LittleEndian.Uint32(d[4:]) != 0 || binary.LittleEndian.Uint16(d[8:]) != 2 ||
		binary.LittleEndian.Uint16(d[10:]) != 88 ||
		d[12] != 1 ||
		d[13] > 1 ||
		d[14] != transfer ||
		d[15] != 0 ||
		!allZero(d[16:20]) ||
		d[20] != 4 ||
		!allZero(d[21:28]) {
		return false
	}
	for i, channel := range []byte{0, 1, 2, 15} {
		sample := d[28+i*16 : 28+(i+1)*16]
		if srgb && i == 3 && sample[3] == channel|16 {
			channel |= 16
		}
		if binary.LittleEndian.Uint16(sample) != uint16(i*8) || sample[2] != 7 || sample[3] != channel || !allZero(sample[4:12]) ||
			binary.LittleEndian.Uint32(sample[12:]) != 255 {
			return false
		}
	}
	return true
}
