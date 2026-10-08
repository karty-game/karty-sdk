// Package asset defines the public, versioned processing contract shared by
// Karty SDK manifests, the CLI asset processors, and runtime hosts.
package asset

import (
	"errors"
	"fmt"
	"slices"
)

// Processor identifies a byte-producing processor whose output and defaults
// are stable for the lifetime of the identifier.
type Processor string

const (
	ProcessorCopyPNGv1 Processor = "copy-png@1"
	ProcessorQOIv1     Processor = "qoi@1"
	ProcessorQOAv1     Processor = "qoa@1"
)

// Capability identifies a runtime asset encoding accepted by a host.
type Capability string

const (
	CapabilityTextureQOIv1             Capability = "texture/qoi@1"
	CapabilitySoundQOAv1               Capability = "sound/qoa@1"
	CapabilityAudioStreamQOAv1         Capability = "audio-stream/qoa@1"
	CapabilityVideoMPEG1v1             Capability = "video/mpeg1@1"
	CapabilityWorldLightingV1          Capability = "world/lighting@1"
	CapabilityWorldLightmapsV1         Capability = "world/lightmaps@1"
	CapabilityWorldLightmapsPrebakedV1 Capability = "world/lightmaps-prebaked@1"
	CapabilityWorldMaterialMappingV1   Capability = "world/material-mapping@1"
	CapabilityWorldAnimationsV1        Capability = "world/animations@1"
	CapabilityWorldEmissionV1          Capability = "world/emission@1"
	CapabilityWorldStaticSolidsV1      Capability = "world/static-solids@1"
	CapabilityWorldMaterialAtlasV1     Capability = "world/material-atlas@1"
	CapabilityWorldMaterialAtlasV2     Capability = "world/material-atlas@2"
	CapabilityWorldMaterialLayersV1    Capability = "world/material-layers@1"
)

const (
	MaxSourceAssetBytes           = 64 * 1024 * 1024
	MaxSourceImageDimension       = 16_384
	MaxSourceImagePixels          = 64 * 1024 * 1024
	MaxTextureDimension           = 8_192
	MaxTexturePixels              = 16 * 1024 * 1024
	MaxDecodedTextureBytes        = 64 * 1024 * 1024
	MaxDecodedTextures            = 256 * 1024 * 1024
	MaxSoundDurationSeconds       = 30
	MaxDecodedSoundBytes          = 6 * 1024 * 1024
	MaxDecodedSounds              = 64 * 1024 * 1024
	MaxSimultaneousSoundVoices    = 32
	MaxAudioStreamDurationSeconds = 4 * 60 * 60
	MaxEncodedAudioStreamBytes    = 512 * 1024 * 1024
)

var ErrContract = errors.New("asset processing contract is invalid")

// Capabilities is the canonical SDK-manifest declaration of processors and
// runtime encodings. Lists must be sorted, unique, and contain known values.
type Capabilities struct {
	Processors []Processor  `toml:"processors"`
	Runtime    []Capability `toml:"runtime"`
}

func (capabilities Capabilities) Validate() error {
	if err := validateCanonical(capabilities.Processors, []Processor{ProcessorCopyPNGv1, ProcessorQOAv1, ProcessorQOIv1}); err != nil {
		return fmt.Errorf("processors: %w", err)
	}
	if err := validateCanonical(
		capabilities.Runtime,
		[]Capability{
			CapabilityAudioStreamQOAv1,
			CapabilitySoundQOAv1,
			CapabilityTextureQOIv1,
			CapabilityVideoMPEG1v1,
			CapabilityWorldLightingV1,
			CapabilityWorldLightmapsV1,
			CapabilityWorldLightmapsPrebakedV1,
			CapabilityWorldMaterialAtlasV1,
			CapabilityWorldMaterialAtlasV2,
			CapabilityWorldMaterialLayersV1,
			CapabilityWorldMaterialMappingV1,
			CapabilityWorldStaticSolidsV1,
			CapabilityWorldAnimationsV1,
			CapabilityWorldEmissionV1,
		},
	); err != nil {
		return fmt.Errorf("runtime capabilities: %w", err)
	}

	return nil
}

type ImageFilter string

const (
	ImageFilterNearest ImageFilter = "nearest"
	ImageFilterSmooth  ImageFilter = "smooth-lanczos3"
)

// ImageRecipe is the resolved, canonical input to qoi@1. Zero maximums mean
// unlimited within the resource bounds. QOI output always has 8-bit channels.
// Resizing never upscales; dimensions use integer floor rounding and remain at
// least one pixel. Smooth filtering means Lanczos3 with straight-alpha pixels.
type ImageRecipe struct {
	MaxWidth  uint32      `toml:"max_width"  json:"max_width"`
	MaxHeight uint32      `toml:"max_height" json:"max_height"`
	Filter    ImageFilter `toml:"filter"     json:"filter"`
	BitDepth  uint8       `toml:"bit_depth"  json:"bit_depth"`
}

func (recipe ImageRecipe) Validate() error {
	if recipe.MaxWidth > MaxTextureDimension || recipe.MaxHeight > MaxTextureDimension ||
		(recipe.Filter != ImageFilterNearest && recipe.Filter != ImageFilterSmooth) || recipe.BitDepth != 8 {
		return ErrContract
	}

	return nil
}

type ChannelMode string

const (
	ChannelPreserve ChannelMode = "preserve"
	ChannelMono     ChannelMode = "mono"
)

// AudioRecipe is the resolved, canonical input to qoa@1. Samples are converted
// to signed PCM16 before QOA encoding. Mono conversion is explicit; preserve
// keeps mono or stereo source channels unchanged.
type AudioRecipe struct {
	SampleRate  uint32      `toml:"sample_rate" json:"sample_rate"`
	ChannelMode ChannelMode `toml:"channels"    json:"channels"`
}

func (recipe AudioRecipe) Validate() error {
	if !slices.Contains([]uint32{22_050, 24_000, 44_100, 48_000}, recipe.SampleRate) ||
		(recipe.ChannelMode != ChannelPreserve && recipe.ChannelMode != ChannelMono) {
		return ErrContract
	}

	return nil
}

func validateCanonical[T ~string](values, supported []T) error {
	for index, value := range values {
		if !slices.Contains(supported, value) || (index > 0 && values[index-1] >= value) {
			return ErrContract
		}
	}

	return nil
}
