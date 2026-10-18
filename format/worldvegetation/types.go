// Package worldvegetation defines bounded, persistent sector surface decoration.
package worldvegetation

import (
	"errors"

	"github.com/karty-game/karty-sdk/format/cartridge"
)

const (
	Version              = uint16(1)
	GeometryVersion      = uint16(2)
	NormalVersion        = uint16(3)
	TreeVersion          = uint16(4)
	PalmVersion          = uint16(5)
	EntryName            = "@world/vegetation"
	Feature              = cartridge.FeatureWorldVegetationV1
	FeatureV2            = cartridge.FeatureWorldVegetationV2
	FeatureV3            = cartridge.FeatureWorldVegetationV3
	FeatureV4            = cartridge.FeatureWorldVegetationV4
	FeatureV5            = cartridge.FeatureWorldVegetationV5
	BinaryMagic          = "KWV1"
	BinaryMagicV2        = "KWV2"
	BinaryMagicV3        = "KWV3"
	BinaryMagicV4        = "KWV4"
	BinaryMagicV5        = "KWV5"
	GeometryCrossedCards = uint8(0)
	GeometryShrub        = uint8(1)
	GeometryTree         = uint8(2)
	GeometryPalm         = uint8(3)
	MaxEncodedSize       = 1024 * 1024
	MaxPresets           = 64
	MaxLayers            = 1024
	MaxDensity           = 16.0
	// MaxInstances bounds submitted cards; crossed-card plants consume two.
	MaxInstances     = 16384
	MaxSize          = 64.0
	MaxWindAmplitude = 8.0
	MaxWindFrequency = 16.0
	MaxDrawDistance  = 128.0
	SurfaceFloor     = "floor"
	SurfaceCeiling   = "ceiling"
)

var (
	ErrVersion     = errors.New("vegetation version is unsupported")
	ErrBounds      = errors.New("vegetation resource exceeds a bound")
	ErrIdentity    = errors.New("vegetation identity is invalid")
	ErrVegetation  = errors.New("vegetation configuration is invalid")
	ErrConfinement = errors.New("vegetation leaves its sector")
	ErrSyntax      = errors.New("vegetation binary syntax is invalid")
)

type Document struct {
	Version uint16
	Presets []Preset
	Layers  []Layer
}
type Range struct {
	Min float64 `yaml:"min"`
	Max float64 `yaml:"max"`
}
type Preset struct {
	Name          string
	Texture       uint32
	NormalTexture uint32
	Geometry      uint8
	Width         Range
	Height        Range
	WindAmplitude float64
	WindFrequency float64
	DrawDistance  float64
}
type Layer struct {
	Sector  uint32
	Preset  string
	Density float64
	Seed    uint32
	Surface string
}

// FeatureForVersion returns the exact runtime capability for an encoded sidecar.
func FeatureForVersion(version uint16) (string, error) {
	switch version {
	case Version:
		return Feature, nil
	case GeometryVersion:
		return FeatureV2, nil
	case NormalVersion:
		return FeatureV3, nil
	case TreeVersion:
		return FeatureV4, nil
	case PalmVersion:
		return FeatureV5, nil
	default:
		return "", ErrVersion
	}
}
