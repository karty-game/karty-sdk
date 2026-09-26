package cartridge

import (
	"encoding/binary"
	"errors"
	"fmt"
	"unicode/utf8"
)

const (
	ManifestSectionName = "karty.manifest.v1"
	manifestMagic       = "KTYM"
	manifestVersion     = uint16(1)
	manifestHeaderSize  = 20
	manifestLevelHeader = 80
	manifestHashLength  = 64
	MaxManifestSize     = 1024 * 1024
	MaxManifestLevels   = 4096
	MaxManifestString   = 1024
)

var ErrManifest = errors.New("game cartridge manifest is invalid")

type Manifest struct {
	ProjectName string
	Compiler    string
	Width       uint32
	Height      uint32
	Levels      []LevelDependency
}

type LevelDependency struct {
	Name            string
	Kind            string
	ContentSHA256   string
	Size            uint64
	EnvelopeVersion uint16
}

func EncodeManifest(manifest Manifest) ([]byte, error) {
	if !validManifestString(manifest.ProjectName) || !validManifestString(manifest.Compiler) ||
		manifest.Width == 0 || manifest.Height == 0 || len(manifest.Levels) > MaxManifestLevels {
		return nil, ErrManifest
	}

	total := manifestHeaderSize + len(manifest.ProjectName) + len(manifest.Compiler)
	previous := ""

	for _, level := range manifest.Levels {
		if !validManifestString(level.Name) || !validManifestString(level.Kind) || level.Name <= previous ||
			!validManifestHash(level.ContentSHA256) || level.Size == 0 || level.EnvelopeVersion == 0 {
			return nil, fmt.Errorf("level %q: %w", level.Name, ErrManifest)
		}

		previous = level.Name
		total += manifestLevelHeader + len(level.Name) + len(level.Kind)

		if total > MaxManifestSize {
			return nil, ErrManifest
		}
	}

	result := make([]byte, total)
	copy(result, manifestMagic)
	binary.LittleEndian.PutUint16(result[4:6], manifestVersion)
	binary.LittleEndian.PutUint16(result[6:8], uint16(len(manifest.ProjectName)))
	binary.LittleEndian.PutUint16(result[8:10], uint16(len(manifest.Compiler)))
	binary.LittleEndian.PutUint32(result[12:16], manifest.Width)
	binary.LittleEndian.PutUint32(result[16:20], manifest.Height)

	offset := manifestHeaderSize
	copy(result[offset:], manifest.ProjectName)
	offset += len(manifest.ProjectName)
	copy(result[offset:], manifest.Compiler)
	offset += len(manifest.Compiler)
	result = appendManifestLevels(result, offset, manifest.Levels)

	return result, nil
}

func appendManifestLevels(result []byte, offset int, levels []LevelDependency) []byte {
	countOffset := 10
	binary.LittleEndian.PutUint16(result[countOffset:countOffset+2], uint16(len(levels)))

	for _, level := range levels {
		binary.LittleEndian.PutUint16(result[offset:offset+2], uint16(len(level.Name)))
		binary.LittleEndian.PutUint16(result[offset+2:offset+4], uint16(len(level.Kind)))
		binary.LittleEndian.PutUint16(result[offset+4:offset+6], level.EnvelopeVersion)
		binary.LittleEndian.PutUint64(result[offset+8:offset+16], level.Size)
		copy(result[offset+16:offset+80], level.ContentSHA256)
		offset += manifestLevelHeader
		copy(result[offset:], level.Name)
		offset += len(level.Name)
		copy(result[offset:], level.Kind)
		offset += len(level.Kind)
	}

	return result
}

func DecodeManifest(encoded []byte) (Manifest, error) {
	if len(encoded) < manifestHeaderSize || len(encoded) > MaxManifestSize || string(encoded[:4]) != manifestMagic ||
		binary.LittleEndian.Uint16(encoded[4:6]) != manifestVersion {
		return Manifest{}, ErrManifest
	}

	nameLength := int(binary.LittleEndian.Uint16(encoded[6:8]))
	compilerLength := int(binary.LittleEndian.Uint16(encoded[8:10]))
	count := int(binary.LittleEndian.Uint16(encoded[10:12]))
	manifest := Manifest{Width: binary.LittleEndian.Uint32(encoded[12:16]), Height: binary.LittleEndian.Uint32(encoded[16:20])}
	offset := manifestHeaderSize

	if count > MaxManifestLevels || nameLength < 1 || compilerLength < 1 || nameLength > len(encoded)-offset ||
		compilerLength > len(encoded)-offset-nameLength || manifest.Width == 0 || manifest.Height == 0 {
		return Manifest{}, ErrManifest
	}

	manifest.ProjectName = string(encoded[offset : offset+nameLength])
	offset += nameLength
	manifest.Compiler = string(encoded[offset : offset+compilerLength])
	offset += compilerLength

	if !validManifestString(manifest.ProjectName) || !validManifestString(manifest.Compiler) {
		return Manifest{}, ErrManifest
	}

	levels, err := decodeManifestLevels(encoded, offset, count)
	if err != nil {
		return Manifest{}, err
	}

	manifest.Levels = levels

	return manifest, nil
}

func decodeManifestLevels(encoded []byte, offset, count int) ([]LevelDependency, error) {
	levels := make([]LevelDependency, 0, count)
	previous := ""

	for range count {
		level, next, err := decodeManifestLevel(encoded, offset)
		if err != nil || level.Name <= previous {
			return nil, ErrManifest
		}

		levels = append(levels, level)
		previous = level.Name
		offset = next
	}

	if offset != len(encoded) {
		return nil, ErrManifest
	}

	return levels, nil
}

func decodeManifestLevel(encoded []byte, offset int) (LevelDependency, int, error) {
	if len(encoded)-offset < manifestLevelHeader || binary.LittleEndian.Uint16(encoded[offset+6:offset+8]) != 0 {
		return LevelDependency{}, 0, ErrManifest
	}

	nameLength := int(binary.LittleEndian.Uint16(encoded[offset : offset+2]))
	kindLength := int(binary.LittleEndian.Uint16(encoded[offset+2 : offset+4]))
	level := LevelDependency{
		EnvelopeVersion: binary.LittleEndian.Uint16(encoded[offset+4 : offset+6]),
		Size:            binary.LittleEndian.Uint64(encoded[offset+8 : offset+16]),
		ContentSHA256:   string(encoded[offset+16 : offset+80]),
	}
	offset += manifestLevelHeader

	if nameLength < 1 || kindLength < 1 || nameLength > len(encoded)-offset || kindLength > len(encoded)-offset-nameLength {
		return LevelDependency{}, 0, ErrManifest
	}

	level.Name = string(encoded[offset : offset+nameLength])
	offset += nameLength
	level.Kind = string(encoded[offset : offset+kindLength])
	offset += kindLength

	if !validManifestString(level.Name) || !validManifestString(level.Kind) || level.Size == 0 ||
		level.EnvelopeVersion == 0 || !validManifestHash(level.ContentSHA256) {
		return LevelDependency{}, 0, ErrManifest
	}

	return level, offset, nil
}

func validManifestHash(value string) bool {
	if len(value) != manifestHashLength {
		return false
	}

	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}

	return true
}

func validManifestString(value string) bool {
	return len(value) > 0 && len(value) <= MaxManifestString && utf8.ValidString(value)
}
