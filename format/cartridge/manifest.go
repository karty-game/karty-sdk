package cartridge

import (
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"unicode/utf8"
)

const (
	ManifestSectionName  = "karty.manifest.v1"
	manifestMagic        = "KTYM"
	manifestVersionV1    = uint16(1)
	manifestVersionV2    = uint16(2)
	manifestHeaderSizeV1 = 20
	manifestHeaderSizeV2 = 24
	manifestLevelHeader  = 80
	manifestHashLength   = 64
	MaxManifestSize      = 1024 * 1024
	MaxManifestLevels    = 4096
	MaxManifestString    = 1024
)

const (
	FeatureTextureQOIv1             = "texture/qoi@1"
	FeatureSoundQOAv1               = "sound/qoa@1"
	FeatureAudioStreamQOAv1         = "audio-stream/qoa@1"
	FeatureWorldSectorsV1           = "world/sectors@1"
	FeatureWorldLightingV1          = "world/lighting@1"
	FeatureWorldLightmapsV1         = "world/lightmaps@1"
	FeatureWorldLightmapsPrebakedV1 = "world/lightmaps-prebaked@1"
	FeatureWorldMaterialMappingV1   = "world/material-mapping@1"
	FeatureWorldStaticSolidsV1      = "world/static-solids@1"
	FeatureWorldMaterialAtlasV1     = "world/material-atlas@1"
)

var ErrManifest = errors.New("game cartridge manifest is invalid")

type Manifest struct {
	ProjectName string
	Compiler    string
	Width       uint32
	Height      uint32
	// Features lists runtime capabilities required to load this cartridge.
	// A non-empty canonical list selects manifest wire version 2 so hosts that
	// only understand version 1 reject the cartridge instead of ignoring it.
	Features []string
	Levels   []LevelDependency
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
		manifest.Width == 0 || manifest.Height == 0 || len(manifest.Levels) > MaxManifestLevels ||
		!validManifestFeatures(manifest.Features) {
		return nil, ErrManifest
	}

	headerSize := manifestHeaderSizeV1
	version := manifestVersionV1
	if len(manifest.Features) > 0 {
		headerSize = manifestHeaderSizeV2
		version = manifestVersionV2
	}

	total := headerSize + len(manifest.ProjectName) + len(manifest.Compiler)
	for _, feature := range manifest.Features {
		total += 2 + len(feature)
	}
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
	binary.LittleEndian.PutUint16(result[4:6], version)
	binary.LittleEndian.PutUint16(result[6:8], uint16(len(manifest.ProjectName)))
	binary.LittleEndian.PutUint16(result[8:10], uint16(len(manifest.Compiler)))
	binary.LittleEndian.PutUint16(result[10:12], uint16(len(manifest.Levels)))
	if version == manifestVersionV1 {
		binary.LittleEndian.PutUint32(result[12:16], manifest.Width)
		binary.LittleEndian.PutUint32(result[16:20], manifest.Height)
	} else {
		binary.LittleEndian.PutUint16(result[12:14], uint16(len(manifest.Features)))
		binary.LittleEndian.PutUint32(result[16:20], manifest.Width)
		binary.LittleEndian.PutUint32(result[20:24], manifest.Height)
	}

	offset := headerSize
	copy(result[offset:], manifest.ProjectName)
	offset += len(manifest.ProjectName)
	copy(result[offset:], manifest.Compiler)
	offset += len(manifest.Compiler)
	for _, feature := range manifest.Features {
		binary.LittleEndian.PutUint16(result[offset:offset+2], uint16(len(feature)))
		offset += 2
		copy(result[offset:], feature)
		offset += len(feature)
	}
	result = appendManifestLevels(result, offset, manifest.Levels)

	return result, nil
}

func appendManifestLevels(result []byte, offset int, levels []LevelDependency) []byte {
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
	if len(encoded) < manifestHeaderSizeV1 || len(encoded) > MaxManifestSize || string(encoded[:4]) != manifestMagic {
		return Manifest{}, ErrManifest
	}
	version := binary.LittleEndian.Uint16(encoded[4:6])
	if version != manifestVersionV1 && version != manifestVersionV2 {
		return Manifest{}, ErrManifest
	}

	nameLength := int(binary.LittleEndian.Uint16(encoded[6:8]))
	compilerLength := int(binary.LittleEndian.Uint16(encoded[8:10]))
	count := int(binary.LittleEndian.Uint16(encoded[10:12]))
	featureCount := 0
	headerSize := manifestHeaderSizeV1
	manifest := Manifest{Width: binary.LittleEndian.Uint32(encoded[12:16]), Height: binary.LittleEndian.Uint32(encoded[16:20])}
	if version == manifestVersionV2 {
		if len(encoded) < manifestHeaderSizeV2 || binary.LittleEndian.Uint16(encoded[14:16]) != 0 {
			return Manifest{}, ErrManifest
		}
		featureCount = int(binary.LittleEndian.Uint16(encoded[12:14]))
		headerSize = manifestHeaderSizeV2
		manifest.Width = binary.LittleEndian.Uint32(encoded[16:20])
		manifest.Height = binary.LittleEndian.Uint32(encoded[20:24])
	}
	offset := headerSize

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

	features, next, err := decodeManifestFeatures(encoded, offset, featureCount)
	if err != nil {
		return Manifest{}, err
	}
	manifest.Features = features

	levels, err := decodeManifestLevels(encoded, next, count)
	if err != nil {
		return Manifest{}, err
	}

	manifest.Levels = levels

	return manifest, nil
}

func decodeManifestFeatures(encoded []byte, offset, count int) ([]string, int, error) {
	if count == 0 {
		return nil, offset, nil
	}

	features := make([]string, 0, count)
	previous := ""
	for range count {
		if len(encoded)-offset < 2 {
			return nil, 0, ErrManifest
		}
		length := int(binary.LittleEndian.Uint16(encoded[offset : offset+2]))
		offset += 2
		if length < 1 || length > MaxManifestString || length > len(encoded)-offset {
			return nil, 0, ErrManifest
		}
		feature := string(encoded[offset : offset+length])
		offset += length
		if !validManifestFeature(feature) || feature <= previous {
			return nil, 0, ErrManifest
		}
		features = append(features, feature)
		previous = feature
	}

	return features, offset, nil
}

func decodeManifestLevels(encoded []byte, offset, count int) ([]LevelDependency, error) {
	if count == 0 {
		if offset != len(encoded) {
			return nil, ErrManifest
		}

		return nil, nil
	}

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

func validManifestFeatures(features []string) bool {
	if slices.Contains(features, FeatureWorldLightmapsPrebakedV1) &&
		(!slices.Contains(features, FeatureWorldLightmapsV1) || !slices.Contains(features, FeatureWorldLightingV1) ||
			!slices.Contains(features, FeatureWorldSectorsV1)) {
		return false
	}
	previous := ""
	for _, feature := range features {
		if !validManifestFeature(feature) || feature <= previous {
			return false
		}
		previous = feature
	}

	return true
}

func validManifestFeature(feature string) bool {
	return feature == FeatureAudioStreamQOAv1 || feature == FeatureSoundQOAv1 || feature == FeatureTextureQOIv1 ||
		feature == FeatureVideoMPEG1v1 || feature == FeatureWorldSectorsV1 || feature == FeatureWorldLightingV1 ||
		feature == FeatureWorldLightmapsV1 || feature == FeatureWorldLightmapsPrebakedV1 || feature == FeatureWorldMaterialAtlasV1 || feature == FeatureWorldMaterialMappingV1 || feature == FeatureWorldStaticSolidsV1
}
