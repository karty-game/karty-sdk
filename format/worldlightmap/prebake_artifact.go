package worldlightmap

import (
	"math"

	"github.com/karty-game/karty-sdk/codec/qoi"
)

// DecodePrebakeArtifact admits a compiled RNM manifest and image after exact
// whole-artifact catalog integrity checks. Layout must already be admitted.
// It validates manifest shape, layout/image identity, complete bounded QOI and
// dimensions/range, without reconstructing geometry or authored bake inputs.
// Image remains borrowed and must stay immutable throughout pair publication.
func DecodePrebakeArtifact(encoded, image []byte, layout Layout) (PrebakePair, error) {
	manifest, err := DecodePrebakeManifest(encoded)
	if err != nil {
		return PrebakePair{}, err
	}
	pair := PrebakePair{Manifest: manifest, Image: image}
	if err := validatePrebakeArtifactPair(pair, layout); err != nil {
		return PrebakePair{}, err
	}
	return pair, nil
}

// ValidatePrebakeArtifact validates structural/resource identities against an
// already admitted immutable layout. BakeSHA256, SurfaceSHA256 and reflectance
// record compiler provenance; full source proof remains PrebakePair.Validate.
func ValidatePrebakeArtifact(pair PrebakePair, layout Layout) error {
	encoded, err := encodePrebakeManifest(pair.Manifest)
	if err != nil {
		return err
	}
	if _, err := DecodePrebakeManifest(encoded); err != nil {
		return err
	}
	return validatePrebakeArtifactPair(pair, layout)
}

func validatePrebakeArtifactPair(pair PrebakePair, layout Layout) error {
	if len(layout.Pages) != 1 || layout.Pages[0].Width != layout.Pages[0].Height ||
		(layout.Pages[0].Width != 512 && layout.Pages[0].Width != 1024) ||
		layout.RuntimeBake == nil || layout.RuntimeBake.Encoding != DirectRNMEncoding {
		return ErrLayout
	}
	if !finite(pair.Manifest.RGBMRange) || pair.Manifest.RGBMRange < 1 || pair.Manifest.RGBMRange > math.MaxFloat32 {
		return ErrLayout
	}
	if len(pair.Image) == 0 || len(pair.Image) > MaxPrebakeImageSize || len(pair.Image) != pair.Manifest.ImageBytes ||
		digest(pair.Image) != pair.Manifest.ImageSHA256 {
		return ErrLayout
	}
	layoutHash, err := LayoutDigest(&layout)
	if err != nil || layoutHash != pair.Manifest.LayoutSHA256 {
		return ErrLayout
	}
	metadata, err := qoi.Validate(pair.Image)
	page := layout.Pages[0]
	if err != nil || int(metadata.Width) != 3*page.Width || int(metadata.Height) != page.Height ||
		int(metadata.Width) != pair.Manifest.Width || int(metadata.Height) != pair.Manifest.Height ||
		metadata.Channels != qoi.ChannelsRGBA || metadata.Colorspace != qoi.ColorspaceLinear {
		return ErrLayout
	}
	return nil
}
