package worldlightmap

import (
	"fmt"
	"math"
	"slices"

	"github.com/karty-game/karty-sdk/codec/qoi"
	"github.com/karty-game/karty-sdk/format/cartridge"
	"github.com/karty-game/karty-sdk/format/level"
	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/internal/binwire"
)

const (
	EnvironmentVariantsSchema      = "karty.world-lightmap-environment-variants@1"
	EnvironmentVariantsEntryName   = "@world/lightmaps/environment-variants"
	EnvironmentImageEntryName      = "@world/lightmaps/environment-image"
	EnvironmentVariantsMetadataKey = "kartyWorldLightmapEnvironmentVariants"
	EnvironmentVariantMetadataKey  = "kartyWorldLightmapEnvironmentVariant"
	EnvironmentVariantsFeature     = cartridge.FeatureWorldLightmapsEnvironmentVariantsV1
	EnvironmentPrebakeProducer     = "cpu-environment-rnm3@1"
	MaxEnvironmentVariants         = 32
	MaxEnvironmentCatalogSize      = 32 * 1024
)

// EnvironmentVariants binds one immutable receiver layout and a bounded set of
// content-addressed passive condition modules. The default image is embedded in
// the owning render chunk; other images need not be fetched until selected.
type EnvironmentVariants struct {
	Schema, GeometrySHA256, LayoutSHA256, SurfaceSHA256, ReflectanceSHA256, Producer string
	Samples, Bounces                                                                 int
	Seed                                                                             uint64
	Denoise                                                                          string
	Default                                                                          int
	Variants                                                                         []EnvironmentVariant
}

type EnvironmentVariant struct {
	TimeOfDay, CloudAmount, RGBMRange float64
	ContentSHA256, ImageSHA256        string
	Size, ImageSize, DecodedBytes     uint64
}

func EnvironmentSurfaceDigest(document *world.Document) (string, error) {
	if document == nil {
		return "", ErrLayout
	}
	if err := world.Validate(document); err != nil {
		return "", err
	}
	base, err := OfflineSurfaceDigest(document)
	if err != nil {
		return "", err
	}
	geometry := *document
	geometry.Animations, geometry.Contents, geometry.SolidRenderers = nil, nil, nil
	materials := world.MaterialIDs(&geometry)
	type opacityBinding struct {
		material uint32
		opacity  float64
	}
	var bindings []opacityBinding
	if document.Animations != nil {
		for _, binding := range document.Animations.Materials {
			if !slices.Contains(materials, binding.Material) {
				continue
			}
			for _, preset := range document.Animations.Presets {
				if preset.Name == binding.Preset && preset.Kind == "liquid" && preset.Opacity != nil && *preset.Opacity < 1 {
					bindings = append(bindings, opacityBinding{binding.Material, *preset.Opacity})
				}
			}
		}
	}
	slices.SortFunc(bindings, func(a, b opacityBinding) int {
		if a.material < b.material {
			return -1
		}
		if a.material > b.material {
			return 1
		}
		return 0
	})
	w := binwire.NewWriter("KES1", 4096)
	w.String(base, 64)
	w.Count(len(bindings), false, world.MaxMaterialAnimations)
	for _, binding := range bindings {
		w.U32(binding.material)
		w.F64(binding.opacity)
	}
	encoded, err := w.Bytes()
	if err != nil {
		return "", err
	}
	return digest(encoded), nil
}

func EnvironmentRGBMRange(environment *world.Environment, settings world.EnvironmentSettings, bounces int) (float64, error) {
	if bounces < 0 || bounces > MaxOfflineBounces {
		return 0, ErrLayout
	}
	resolved, err := world.ResolveEnvironmentSettings(environment, settings)
	if err != nil {
		return 0, err
	}
	cube, sky := resolved.AmbientCube, 0.0
	for _, face := range [...]world.Vec3{cube.PositiveX, cube.NegativeX, cube.PositiveY, cube.NegativeY, cube.PositiveZ, cube.NegativeZ} {
		sky = max(sky, face.X, face.Y, face.Z)
	}
	energy := max(resolved.SunColor.X, resolved.SunColor.Y, resolved.SunColor.Z)*resolved.SunIntensity*(1-settings.CloudAmount) + sky
	factor, reflectance := 1.0, 1.0
	for range bounces {
		reflectance *= MaxDiffuseReflectance
		factor += reflectance
	}
	return max(1, 3*energy*factor), nil
}

func EncodeEnvironmentVariants(catalog EnvironmentVariants, layout Layout, document *world.Document) ([]byte, error) {
	if err := ValidateEnvironmentVariants(&catalog, layout, document); err != nil {
		return nil, err
	}
	w := binwire.NewWriter("KLV1", MaxEnvironmentCatalogSize)
	writeWorldlightmapEnvironmentVariants(w, &catalog)
	return w.Bytes()
}

func DecodeEnvironmentVariants(encoded []byte, layout Layout, document *world.Document) (EnvironmentVariants, error) {
	scan := binwire.NewReader(encoded, "KLV1", MaxEnvironmentCatalogSize)
	scanWorldlightmapEnvironmentVariants(scan)
	if scan.Done() != nil {
		return EnvironmentVariants{}, ErrLayout
	}
	r := binwire.NewReader(encoded, "KLV1", MaxEnvironmentCatalogSize)
	var catalog EnvironmentVariants
	readWorldlightmapEnvironmentVariants(r, &catalog)
	if r.Done() != nil {
		return EnvironmentVariants{}, ErrLayout
	}
	if err := ValidateEnvironmentVariants(&catalog, layout, document); err != nil {
		return EnvironmentVariants{}, err
	}
	return catalog, nil
}

func ValidateEnvironmentVariants(catalog *EnvironmentVariants, layout Layout, document *world.Document) error {
	if catalog == nil || document == nil || document.Environment == nil || layout.RuntimeBake != nil || layout.Receivers == nil ||
		catalog.Schema != EnvironmentVariantsSchema || catalog.Producer != EnvironmentPrebakeProducer ||
		catalog.Samples < 1 || catalog.Samples > MaxOfflineSamples || catalog.Bounces < 0 || catalog.Bounces > MaxOfflineBounces ||
		len(catalog.Variants) < 2 || len(catalog.Variants) > MaxEnvironmentVariants || len(catalog.Variants)%2 != 0 ||
		catalog.Default < 0 || catalog.Default >= len(catalog.Variants) || !validEnvironmentHash(catalog.ReflectanceSHA256) {
		return ErrLayout
	}
	if err := Validate(&layout, document); err != nil {
		return err
	}
	if _, err := OfflineDenoiseProducer(catalog.Denoise); err != nil {
		return err
	}
	layoutHash, err := LayoutDigest(&layout)
	if err != nil {
		return err
	}
	surfaceHash, err := EnvironmentSurfaceDigest(document)
	if err != nil {
		return err
	}
	if catalog.GeometrySHA256 != layout.GeometrySHA256 || catalog.LayoutSHA256 != layoutHash || catalog.SurfaceSHA256 != surfaceHash {
		return fmt.Errorf("environment variant geometry, surface or chart identity: %w", ErrLayout)
	}
	decoded := uint64(layout.Pages[0].Width * layout.Pages[0].Height * 12)
	for i, variant := range catalog.Variants {
		if world.ValidateEnvironmentSettings(
			world.EnvironmentSettings{TimeOfDay: variant.TimeOfDay, CloudAmount: variant.CloudAmount},
		) != nil ||
			variant.CloudAmount != float64(i%2) ||
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
		if i%2 == 1 && variant.TimeOfDay != catalog.Variants[i-1].TimeOfDay ||
			i%2 == 0 && i > 0 && variant.TimeOfDay <= catalog.Variants[i-2].TimeOfDay {
			return ErrLayout
		}
		expectedRange, err := EnvironmentRGBMRange(
			document.Environment,
			world.EnvironmentSettings{TimeOfDay: variant.TimeOfDay, CloudAmount: variant.CloudAmount},
			catalog.Bounces,
		)
		if err != nil || variant.RGBMRange != expectedRange {
			return ErrLayout
		}
	}
	return nil
}

// ValidateEnvironmentImage checks exact content, dimensions and complete QOI
// stream before callers allocate decoded pixels or replace a resident map.
func ValidateEnvironmentImage(variant EnvironmentVariant, encoded []byte, layout Layout) error {
	if len(layout.Pages) != 1 || layout.Pages[0].Width != layout.Pages[0].Height ||
		(layout.Pages[0].Width != 512 && layout.Pages[0].Width != 1024) || len(encoded) > MaxPrebakeImageSize ||
		uint64(len(encoded)) != variant.ImageSize || digest(encoded) != variant.ImageSHA256 {
		return ErrLayout
	}
	metadata, err := qoi.Validate(encoded)
	if err != nil || metadata.Channels != qoi.ChannelsRGBA || metadata.Colorspace != qoi.ColorspaceLinear ||
		metadata.Width != uint32(layout.Pages[0].Width*3) || metadata.Height != uint32(layout.Pages[0].Height) ||
		metadata.DecodedBytes != variant.DecodedBytes {
		return ErrLayout
	}
	return nil
}

func validEnvironmentHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

// SelectEnvironmentVariants chooses one authored clock and its clear/overcast
// endpoints. Clock distance wraps midnight. Ties retain the first authored time.
// Continuous weather blends decoded linear RNM coefficients; time is discrete.
func SelectEnvironmentVariants(catalog *EnvironmentVariants, settings world.EnvironmentSettings) (int, int, float64, error) {
	if catalog == nil || len(catalog.Variants) < 2 || len(catalog.Variants) > MaxEnvironmentVariants || len(catalog.Variants)%2 != 0 ||
		world.ValidateEnvironmentSettings(settings) != nil {
		return 0, 0, 0, ErrLayout
	}
	selected, distance := 0, math.Inf(1)
	for i := 0; i < len(catalog.Variants); i += 2 {
		clearSky, overcast := catalog.Variants[i], catalog.Variants[i+1]
		if world.ValidateEnvironmentSettings(world.EnvironmentSettings{TimeOfDay: clearSky.TimeOfDay}) != nil ||
			clearSky.CloudAmount != 0 || overcast.CloudAmount != 1 || overcast.TimeOfDay != clearSky.TimeOfDay ||
			i > 0 && clearSky.TimeOfDay <= catalog.Variants[i-2].TimeOfDay {
			return 0, 0, 0, ErrLayout
		}
		delta := math.Abs(settings.TimeOfDay - catalog.Variants[i].TimeOfDay)
		delta = min(delta, 24-delta)
		if delta < distance {
			selected, distance = i, delta
		}
	}
	return selected, selected + 1, settings.CloudAmount, nil
}
