package worldlightmap

import (
	"github.com/karty-game/karty-sdk/format/level"
	"github.com/karty-game/karty-sdk/format/world"
)

// DecodeEnvironmentVariantsArtifact decodes a builder-approved catalog after
// whole-file integrity admission. It checks binary/resource bounds and identities
// without reconstructing source geometry, surface digests or lighting proofs.
func DecodeEnvironmentVariantsArtifact(encoded []byte, layout Layout) (EnvironmentVariants, error) {
	catalog, err := decodeEnvironmentVariantsBinary(encoded)
	if err != nil {
		return EnvironmentVariants{}, err
	}
	if err := ValidateEnvironmentVariantsArtifact(&catalog, layout); err != nil {
		return EnvironmentVariants{}, err
	}
	return catalog, nil
}

// ValidateEnvironmentVariantsArtifact checks runtime references and allocation
// bounds. Builder callers retain ValidateEnvironmentVariants for source proof.
func ValidateEnvironmentVariantsArtifact(catalog *EnvironmentVariants, layout Layout) error {
	if catalog == nil || layout.RuntimeBake != nil || layout.Receivers == nil || len(layout.Pages) != 1 ||
		layout.Pages[0].Width != layout.Pages[0].Height || (layout.Pages[0].Width != 512 && layout.Pages[0].Width != 1024) ||
		catalog.Schema != EnvironmentVariantsSchema || !environmentProducer(catalog.Producer) ||
		catalog.Samples < 1 || catalog.Samples > MaxOfflineSamples || catalog.Bounces < 0 || catalog.Bounces > MaxOfflineBounces ||
		len(catalog.Variants) == 0 || len(catalog.Variants) > MaxEnvironmentVariants || catalog.Default < 0 ||
		catalog.Default >= len(catalog.Variants) || !validEnvironmentHash(catalog.ReflectanceSHA256) ||
		!validEnvironmentHash(catalog.SurfaceSHA256) || !validEnvironmentHash(catalog.GeometrySHA256) {
		return ErrLayout
	}
	if _, err := environmentVariantStride(catalog); err != nil || catalog.Variants[catalog.Default].CloudAmount != 0 {
		return ErrLayout
	}
	if _, err := OfflineDenoiseProducer(catalog.Denoise); err != nil {
		return err
	}
	layoutHash, err := LayoutDigest(&layout)
	if err != nil || catalog.GeometrySHA256 != layout.GeometrySHA256 || catalog.LayoutSHA256 != layoutHash {
		return ErrLayout
	}
	decoded := uint64(layout.Pages[0].Width * layout.Pages[0].Height * 12)
	for _, variant := range catalog.Variants {
		if world.ValidateEnvironmentSettings(
			world.EnvironmentSettings{TimeOfDay: variant.TimeOfDay, CloudAmount: variant.CloudAmount},
		) != nil ||
			!finite(variant.RGBMRange) ||
			variant.RGBMRange < 1 ||
			variant.RGBMRange > 256 ||
			!validEnvironmentHash(variant.ContentSHA256) ||
			!validEnvironmentHash(variant.ImageSHA256) ||
			variant.Size == 0 ||
			variant.Size > level.MaxEnvelopeSize ||
			variant.ImageSize == 0 ||
			variant.ImageSize > MaxPrebakeImageSize ||
			variant.ImageSize >= variant.Size ||
			variant.DecodedBytes != decoded {
			return ErrLayout
		}
	}
	return nil
}
