// Package ktx2 inspects Karty's bounded UASTC LDR 4x4 + Zstandard profile.
// It does not decode pixels or transcode GPU blocks.
package ktx2

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math/bits"
	"slices"

	"github.com/karty-game/karty-sdk/format/asset"
)

const (
	HeaderSize      = 80
	MaxLevels       = 14
	MaxLayers       = 256
	MaxEncodedBytes = asset.MaxSourceAssetBytes
	headerLevelSize = 24
)

var (
	ErrInvalid = errors.New("KTX2 UASTC texture is invalid")
	identifier = []byte{0xab, 'K', 'T', 'X', ' ', '2', '0', 0xbb, 13, 10, 26, 10}
)

// Level identifies one complete mip, with all array layers in order.
type Level struct{ Offset, Length, UncompressedLength uint64 }

// Info describes a validated container and preflight allocation costs. RGBABytes
// is the full fallback texture size; InflatedBytes is the UASTC block size or
// the exact RGBA byte size for the separate lossless profile.
// Alpha is straight and linear even when RGB uses the sRGB transfer function.
type Info struct {
	Width, Height, Layers    uint32
	SRGB                     bool
	HasAlpha                 bool // False means the fourth component is implicitly 1.
	Levels                   []Level
	InflatedBytes, RGBABytes uint64
}

type section struct{ offset, length uint64 }

// Inspect validates all ranges, the complete mip chain, DFD, supported metadata
// and Zstandard frame boundaries before any inflated or GPU allocation. It
// cannot validate compressed block contents: consumers must fully decompress
// and transcode before publishing a texture. Returned slices are newly owned.
func Inspect(encoded []byte) (Info, error) {
	info, count, err := inspectHeader(encoded)
	if err != nil {
		return Info{}, err
	}
	indexEnd := uint64(HeaderSize) + uint64(count)*headerLevelSize
	sections, err := inspectMetadataSections(encoded, indexEnd, &info)
	if err != nil {
		return Info{}, err
	}
	sections, err = inspectLevels(encoded, count, indexEnd, &info, sections)
	if err != nil || inspectSectionCoverage(encoded, sections) != nil {
		return Info{}, ErrInvalid
	}
	return info, nil
}

func inspectHeader(encoded []byte) (Info, uint32, error) {
	if len(encoded) < HeaderSize || len(encoded) > MaxEncodedBytes || !bytes.Equal(encoded[:12], identifier) {
		return Info{}, 0, ErrInvalid
	}
	u32 := func(offset int) uint32 { return binary.LittleEndian.Uint32(encoded[offset:]) }
	info := Info{Width: u32(20), Height: u32(24), Layers: u32(32)}
	count := u32(40)
	if u32(12) != 0 || u32(16) != 1 || u32(28) != 0 || u32(36) != 1 || u32(44) != 2 ||
		binary.LittleEndian.Uint64(encoded[64:]) != 0 || binary.LittleEndian.Uint64(encoded[72:]) != 0 {
		return Info{}, 0, ErrInvalid
	}
	if info.Width == 0 || info.Height == 0 || info.Layers == 0 || info.Layers > MaxLayers ||
		info.Width > asset.MaxTextureDimension || info.Height > asset.MaxTextureDimension ||
		count != uint32(bits.Len32(max(info.Width, info.Height))) || count > MaxLevels ||
		uint64(HeaderSize)+uint64(count)*headerLevelSize > uint64(len(encoded)) {
		return Info{}, 0, ErrInvalid
	}
	return info, count, nil
}

func inspectMetadataSections(encoded []byte, indexEnd uint64, info *Info) ([]section, error) {
	u32 := func(offset int) uint32 { return binary.LittleEndian.Uint32(encoded[offset:]) }
	dfd := section{uint64(u32(48)), uint64(u32(52))}
	kvd := section{uint64(u32(56)), uint64(u32(60))}
	if !validSection(dfd, indexEnd, uint64(len(encoded))) || dfd.length != 44 || dfd.offset%4 != 0 ||
		(kvd.length == 0 && kvd.offset != 0) || (kvd.length != 0 && (!validSection(kvd, indexEnd, uint64(len(encoded))) || kvd.offset%4 != 0 || kvd.length > 64<<10)) {
		return nil, ErrInvalid
	}
	var err error
	info.SRGB, info.HasAlpha, err = inspectDFD(encoded[dfd.offset : dfd.offset+dfd.length])
	if err != nil {
		return nil, err
	}
	sections := []section{{0, indexEnd}, dfd}
	if kvd.length != 0 {
		if inspectMetadata(encoded[kvd.offset:kvd.offset+kvd.length]) != nil {
			return nil, ErrInvalid
		}
		sections = append(sections, kvd)
	}
	return sections, nil
}

func inspectLevels(encoded []byte, count uint32, indexEnd uint64, info *Info, sections []section) ([]section, error) {
	info.Levels = make([]Level, count)
	for i := range count {
		start := HeaderSize + int(i)*headerLevelSize
		level := Level{
			binary.LittleEndian.Uint64(encoded[start:]),
			binary.LittleEndian.Uint64(encoded[start+8:]),
			binary.LittleEndian.Uint64(encoded[start+16:]),
		}
		w, h := uint64(max(1, info.Width>>i)), uint64(max(1, info.Height>>i))
		inflated := ((w + 3) / 4) * ((h + 3) / 4) * 16 * uint64(info.Layers)
		rgba := w * h * 4 * uint64(info.Layers)
		if !validSection(section{level.Offset, level.Length}, indexEnd, uint64(len(encoded))) ||
			level.UncompressedLength != inflated || inflated > asset.MaxDecodedTextures-info.InflatedBytes ||
			rgba > asset.MaxDecodedTextures-info.RGBABytes || inspectZstd(encoded[level.Offset:level.Offset+level.Length], inflated) != nil {
			return nil, ErrInvalid
		}
		info.InflatedBytes += inflated
		info.RGBABytes += rgba
		info.Levels[i] = level
		sections = append(sections, section{level.Offset, level.Length})
	}
	return sections, nil
}

func inspectSectionCoverage(encoded []byte, sections []section) error {
	slices.SortFunc(sections, func(a, b section) int {
		if a.offset < b.offset {
			return -1
		}
		if a.offset > b.offset {
			return 1
		}
		return 0
	})
	end := uint64(0)
	for _, s := range sections {
		if s.offset < end || s.offset-end > 7 || !allZero(encoded[end:s.offset]) {
			return ErrInvalid
		}
		end = s.offset + s.length
	}
	if end != uint64(len(encoded)) {
		return ErrInvalid
	}
	return nil
}

func validSection(s section, lower, upper uint64) bool {
	return s.length != 0 && s.offset >= lower && s.offset <= upper && s.length <= upper-s.offset
}

func allZero(data []byte) bool {
	for _, v := range data {
		if v != 0 {
			return false
		}
	}
	return true
}

func inspectDFD(d []byte) (bool, bool, error) {
	if binary.LittleEndian.Uint32(d) != 44 || binary.LittleEndian.Uint32(d[4:]) != 0 ||
		binary.LittleEndian.Uint16(d[8:]) != 2 || binary.LittleEndian.Uint16(d[10:]) != 40 ||
		d[12] != 166 || d[13] > 1 || (d[14] != 1 && d[14] != 2) || d[15] != 0 ||
		!bytes.Equal(d[16:20], []byte{3, 3, 0, 0}) || (d[20] != 16 && d[20] != 0) || !allZero(d[21:28]) ||
		binary.LittleEndian.Uint16(d[28:]) != 0 || d[30] != 127 || (d[31] != 0 && d[31] != 3) ||
		!allZero(d[32:40]) || binary.LittleEndian.Uint32(d[40:]) != 0xffffffff {
		return false, false, ErrInvalid
	}
	return d[14] == 2, d[31] == 3, nil
}
