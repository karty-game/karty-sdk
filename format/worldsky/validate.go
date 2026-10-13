package worldsky

import (
	"fmt"
	"math"

	"github.com/karty-game/karty-sdk/internal/binwire"
)

const (
	// Header, document version, thirteen float64 settings, four image presences.
	documentEncodedOverhead = binwire.HeaderSize + 2 + 13*8 + 4
	// Each present image carries int32 width/height and uint32 packed length.
	imageEncodedOverhead = 3 * 4
)

// Validate checks the complete resource, including every gzip checksum, using
// bounded decompression scratch rather than allocating expanded panoramas.
func Validate(document *Document) error {
	if document == nil || document.Version != Version {
		return ErrVersion
	}
	if err := ValidateSettings(document.Settings); err != nil {
		return err
	}
	encoded, decoded := documentEncodedOverhead, 0
	images := [4]*Image{document.CloudSparse, document.CloudDense, document.Background, document.Horizon}
	for index, image := range images {
		if image == nil {
			continue
		}
		if err := image.validateDimensions(); err != nil {
			return fmt.Errorf("sky image %d: %w", index, err)
		}
		encoded += imageEncodedOverhead + len(image.Packed)
		decoded += image.Width * image.Height * 4
		if encoded > MaxEncodedSize || decoded > MaxDecodedBytes {
			return ErrBounds
		}
	}
	// Establish all dimensions and aggregate bounds before opening any stream.
	for index, image := range images {
		if image != nil {
			if err := image.inspect(); err != nil {
				return fmt.Errorf("sky image %d: %w", index, err)
			}
		}
	}
	return nil
}

// ValidateSettings checks finite, bounded authored presentation values.
func ValidateSettings(settings Settings) error {
	for _, value := range settings.CloudUVScale {
		if !within(value, 0, 4) || value == 0 {
			return ErrSettings
		}
	}
	if !within(settings.HorizonUVScale, 0, 4) || settings.HorizonUVScale == 0 ||
		!within(settings.CloudDenseScrollScale, 0, 4) || !within(settings.HorizonParallax, 0, .01) ||
		!within(settings.HorizonHaze, 0, 1) {
		return ErrSettings
	}
	for _, value := range settings.CloudDenseOffset {
		if !within(value, -1, 1) {
			return ErrSettings
		}
	}
	for _, value := range settings.CloudParallax {
		if !within(value, -.01, .01) {
			return ErrSettings
		}
	}
	for _, value := range settings.HorizonEmission {
		if !within(value, 0, 1) {
			return ErrSettings
		}
	}
	return nil
}

func within(value, minimum, maximum float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= minimum && value <= maximum
}

func (image Image) validateDimensions() error {
	if image.Width < 1 || image.Height < 1 || image.Width > MaxDimension || image.Height > MaxDimension ||
		image.Width > MaxPixels/image.Height || len(image.Packed) == 0 || len(image.Packed) > MaxPackedImageBytes {
		return ErrBounds
	}
	return nil
}
