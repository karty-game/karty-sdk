package worldlightmap

import (
	"math"

	"github.com/karty-game/karty-sdk/codec/qoi"
	"github.com/karty-game/karty-sdk/format/cartridge"
	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldterrain"
)

const (
	FixedSchema          = "karty.world-fixed-lighting@1"
	FixedCompactSchema   = "karty.world-fixed-lighting@2"
	FixedEntryName       = "@world/lightmaps/fixed"
	FixedImageEntryName  = "@world/lightmaps/fixed-rnm3"
	FixedMetadataKey     = "kartyWorldFixedLighting"
	FixedFeature         = cartridge.FeatureWorldFixedLightingV1
	FixedCompactFeature  = cartridge.FeatureWorldFixedCompactLightingV1
	FixedProducer        = "cpu-fixed-rnm3@3"
	FixedSpatialProducer = "cpu-fixed-spatial-rnm3@1"
	MaxFixedLightingSize = 2 << 20
	MaxLightingProbes    = 4096
	MaxCompiledLights    = world.MaxLights + 1
)

// FixedLighting is an atomic, chunk-owned fixed illumination result. Ambient
// cubes contain sky, bounced light and non-nominated direct sources. The two
// nominated sources are evaluated only through Direct, never again in Ambient.
// Cubes use +X,-X,+Y,-Y,+Z,-Z order. No runtime condition variants exist.
type FixedLighting struct {
	TerrainSHA256, TerrainReceiverSHA256 string
	Terrain                              []TerrainPatchLighting
	// ConstantRNM replaces the entire atlas only when every chart is uniform.
	// Every layout chart is present in canonical index order.
	ConstantRNM                                           []ChartRNM
	Schema, GeometrySHA256, LayoutSHA256, SurfaceSHA256   string
	ReflectanceSHA256, InputSHA256, ImageSHA256, Producer string
	ImageBytes, Width, Height, Samples, Bounces           int
	Seed                                                  uint64
	Denoise                                               string
	RGBMRange                                             float64
	Lights                                                []CompiledLight
	Probes                                                []LightingProbe
}

// ChartRNM stores linear RGB irradiance for the three existing RNM lobes.
// All zero coefficients are valid covered black, not missing lighting.
type ChartRNM struct {
	Chart        int
	Coefficients [9]float32
}

type CompiledLight struct {
	ID, Kind                   string
	Position, Direction, Color world.Vec3
	Radius                     float64
}

// TerrainPatchLighting stores three directional RGB coefficients per exact
// CompilePatch vertex. Vertices use TerrainLightingBasis for their RNM frame.
type TerrainPatchLighting struct {
	Patch        int
	Coefficients []float32
}

type LightingProbe struct {
	Sector   int
	Position world.Vec3
	Ambient  [6]world.Vec3
	Direct   [2]ProbeDirect
}

type ProbeDirect struct {
	Light      int // -1 means no source; Visibility must then be zero.
	Visibility float64
}

// FixedLightingBytes bounds retained scalar data, including Go slice/string
// headers and alignment. Image storage is charged separately by the owner.
func FixedLightingBytes(lighting *FixedLighting) uint64 {
	if lighting == nil {
		return 0
	}
	bytes := uint64(512 + len(lighting.Probes)*216 + len(lighting.Lights)*112 + len(lighting.ConstantRNM)*48)
	for _, patch := range lighting.Terrain {
		bytes += uint64(32 + len(patch.Coefficients)*4)
	}
	for _, light := range lighting.Lights {
		bytes += uint64(len(light.ID) + len(light.Kind))
	}
	return bytes
}

// NewFixedLighting binds a producer result to its immutable layout and QOI image.
func NewFixedLighting(layout Layout, document *world.Document, image []byte, lighting FixedLighting) (FixedLighting, error) {
	var err error
	lighting.Schema = FixedSchema
	if len(lighting.ConstantRNM) > 0 {
		lighting.Schema = FixedCompactSchema
	}
	if lighting.Producer == "" {
		lighting.Producer = FixedProducer
	}
	lighting.GeometrySHA256 = layout.GeometrySHA256
	lighting.LayoutSHA256, err = LayoutDigest(&layout)
	if err != nil {
		return FixedLighting{}, err
	}
	lighting.ImageBytes, lighting.ImageSHA256 = len(image), digest(image)
	if len(layout.Pages) != 1 {
		return FixedLighting{}, ErrLayout
	}
	lighting.Width, lighting.Height = 3*layout.Pages[0].Width, layout.Pages[0].Height
	if lighting.Schema == FixedCompactSchema {
		if len(image) != 0 {
			return FixedLighting{}, ErrLayout
		}
		lighting.ImageBytes, lighting.Width, lighting.Height = 0, 0, 0
		lighting.ImageSHA256 = ""
	}
	if err := ValidateFixedLighting(lighting, layout, document); err != nil {
		return FixedLighting{}, err
	}
	if err := ValidateFixedLightingArtifact(lighting, image, layout); err != nil {
		return FixedLighting{}, err
	}
	return lighting, nil
}

func ValidateFixedLighting(lighting FixedLighting, layout Layout, document *world.Document) error {
	if err := Validate(&layout, document); err != nil {
		return err
	}
	if err := validateFixedStructure(lighting); err != nil {
		return err
	}
	if document == nil {
		return ErrLayout
	}
	if err := validateFixedSources(lighting, layout, document); err != nil {
		return err
	}
	surface, err := EnvironmentSurfaceDigest(document)
	if err != nil || surface != lighting.SurfaceSHA256 {
		return ErrLayout
	}
	for _, probe := range lighting.Probes {
		if probe.Sector >= len(document.Sectors) ||
			layout.Receivers != nil && !containsFixedSector(layout.Receivers.Sectors, probe.Sector) {
			return ErrLayout
		}
	}
	return validateFixedLayout(lighting, layout)
}

func containsFixedSector(sectors []int, sector int) bool {
	for _, candidate := range sectors {
		if candidate == sector {
			return true
		}
	}
	return false
}

func validateFixedStructure(lighting FixedLighting) error {
	if (len(lighting.Terrain) > 0) != (lighting.TerrainSHA256 != "") ||
		(len(lighting.Terrain) > 0) != (lighting.TerrainReceiverSHA256 != "") ||
		len(lighting.Terrain) > worldterrain.MaxPatches {
		return ErrLayout
	}
	if len(lighting.Terrain) > 0 && (!lowerSHA256(lighting.TerrainSHA256) || !lowerSHA256(lighting.TerrainReceiverSHA256)) {
		return ErrLayout
	}
	for index, patch := range lighting.Terrain {
		if patch.Patch != index || len(patch.Coefficients) < 36 || len(patch.Coefficients)%9 != 0 ||
			len(patch.Coefficients) > worldterrain.MaxVertices*9 {
			return ErrLayout
		}
		for _, value := range patch.Coefficients {
			if !finite(float64(value)) || value < 0 {
				return ErrLayout
			}
		}
	}
	if !validFixedImageStructure(lighting) ||
		lighting.Producer != FixedSpatialProducer && lighting.Producer != FixedProducer && lighting.Producer != "cpu-fixed-rnm3@1" &&
			lighting.Producer != "cpu-fixed-rnm3@2" ||
		lighting.Samples < 1 || lighting.Samples > MaxOfflineSamples || lighting.Bounces < 0 || lighting.Bounces > MaxOfflineBounces ||
		!finite(lighting.RGBMRange) || lighting.RGBMRange < 1 || lighting.RGBMRange > math.MaxFloat32 ||
		len(
			lighting.Lights,
		) > MaxCompiledLights || len(lighting.Probes) < 1 && lighting.Producer != FixedSpatialProducer || len(lighting.Probes) > MaxLightingProbes {
		return ErrLayout
	}
	for _, hash := range []string{lighting.GeometrySHA256, lighting.LayoutSHA256, lighting.SurfaceSHA256, lighting.ReflectanceSHA256, lighting.InputSHA256} {
		if !lowerSHA256(hash) {
			return ErrLayout
		}
	}
	if _, err := OfflineDenoiseProducer(lighting.Denoise); err != nil {
		return err
	}
	identities := make(map[string]bool, len(lighting.Lights))
	for _, light := range lighting.Lights {
		if light.ID == "" || len(light.ID) > world.MaxIdentifierBytes || identities[light.ID] ||
			!fixedVector(light.Position, world.MaxCoordinate, false) ||
			!fixedVector(light.Direction, 1, false) ||
			!fixedVector(light.Color, math.MaxFloat32, true) ||
			!finite(light.Radius) {
			return ErrLayout
		}
		identities[light.ID] = true
		switch light.Kind {
		case "point":
			if light.Radius < world.MinLightRadius || light.Radius > world.MaxCoordinate || dot(light.Direction, light.Direction) != 0 {
				return ErrLayout
			}
		case "sun":
			if light.Radius != 0 || math.Abs(dot(light.Direction, light.Direction)-1) > 1e-6 {
				return ErrLayout
			}
		default:
			return ErrLayout
		}
	}
	for _, probe := range lighting.Probes {
		if probe.Sector < 0 || probe.Sector >= world.MaxSectors || !fixedVector(probe.Position, world.MaxCoordinate, false) {
			return ErrLayout
		}
		for _, color := range probe.Ambient {
			if !fixedVector(color, math.MaxFloat32, true) {
				return ErrLayout
			}
		}
		for index, direct := range probe.Direct {
			if direct.Light < -1 || direct.Light >= len(lighting.Lights) || !finite(direct.Visibility) || direct.Visibility < 0 ||
				direct.Visibility > 1 ||
				direct.Light == -1 && direct.Visibility != 0 ||
				index > 0 && direct.Light >= 0 && probe.Direct[0].Light == direct.Light {
				return ErrLayout
			}
		}
	}
	return nil
}

func fixedVector(v world.Vec3, limit float64, positive bool) bool {
	for _, value := range [3]float64{v.X, v.Y, v.Z} {
		if !finite(value) || math.Abs(value) > limit || positive && value < 0 {
			return false
		}
	}
	return true
}

func validateFixedLayout(lighting FixedLighting, layout Layout) error {
	spatial := layout.Selection != nil && layout.Selection.Clip != nil
	if spatial != (lighting.Producer == FixedSpatialProducer) || spatial && (len(lighting.Probes) != 0 || len(lighting.Terrain) != 0) {
		return ErrLayout
	}
	if len(layout.Pages) != 1 || lighting.GeometrySHA256 != layout.GeometrySHA256 {
		return ErrLayout
	}
	hash, err := LayoutDigest(&layout)
	if err != nil || hash != lighting.LayoutSHA256 {
		return ErrLayout
	}
	if lighting.Schema == FixedCompactSchema {
		if !spatial || len(lighting.ConstantRNM) != len(layout.Charts) {
			return ErrLayout
		}
	} else if lighting.Width != 3*layout.Pages[0].Width || lighting.Height != layout.Pages[0].Height {
		return ErrLayout
	}
	return nil
}

func ValidateFixedLightingArtifact(lighting FixedLighting, image []byte, layout Layout) error {
	if err := validateFixedStructure(lighting); err != nil {
		return err
	}
	if err := validateFixedLayout(lighting, layout); err != nil {
		return err
	}
	if lighting.Schema == FixedCompactSchema {
		if len(image) != 0 {
			return ErrLayout
		}
		return nil
	}
	if len(image) != lighting.ImageBytes || digest(image) != lighting.ImageSHA256 {
		return ErrLayout
	}
	metadata, err := qoi.Validate(image)
	if err != nil || int(metadata.Width) != lighting.Width || int(metadata.Height) != lighting.Height ||
		metadata.Channels != qoi.ChannelsRGBA ||
		metadata.Colorspace != qoi.ColorspaceLinear {
		return ErrLayout
	}
	return nil
}

func DecodeFixedLightingArtifact(encoded, image []byte, layout Layout) (FixedLighting, error) {
	lighting, err := DecodeFixedLighting(encoded)
	if err != nil {
		return FixedLighting{}, err
	}
	if err := ValidateFixedLightingArtifact(lighting, image, layout); err != nil {
		return FixedLighting{}, err
	}
	return lighting, nil
}
