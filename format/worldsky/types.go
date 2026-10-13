// Package worldsky defines project-owned, bounded skydome textures and presentation.
package worldsky

import (
	"errors"

	"github.com/karty-game/karty-sdk/format/cartridge"
)

const (
	Version             = uint16(1)
	EntryName           = "@world/sky.bin"
	Feature             = cartridge.FeatureWorldSkyV1
	BinaryMagic         = "KSK1"
	MaxDimension        = 4096
	MaxPixels           = 4 * 1024 * 1024
	MaxPackedImageBytes = MaxPixels*4 + 64*1024
	MaxEncodedSize      = 24 * 1024 * 1024
	MaxDecodedBytes     = 64 * 1024 * 1024
)

var (
	ErrVersion  = errors.New("sky version is unsupported")
	ErrBounds   = errors.New("sky resource exceeds a bound")
	ErrSettings = errors.New("sky presentation is invalid")
	ErrImage    = errors.New("sky packed image is invalid")
	ErrSyntax   = errors.New("sky binary syntax is invalid")
)

// Document is independent of the frozen compiled world/environment@1 payload.
// Any image may be omitted, including all images for an asset-free sky.
type Document struct {
	Version     uint16
	Settings    Settings
	CloudSparse *Image
	CloudDense  *Image
	Background  *Image
	Horizon     *Image
}

// Image contains exactly one gzip member of linear, premultiplied RGBA8 pixels.
// Producers own PNG decoding and the sRGB-to-linear conversion.
type Image struct {
	Width, Height int
	Packed        []byte
}

// Settings describes artwork mapping and decoration, without changing lighting.
type Settings struct {
	CloudUVScale          [2]float64 `yaml:"cloud_uv_scale"`
	CloudDenseScrollScale float64    `yaml:"cloud_dense_scroll_scale"`
	CloudDenseOffset      [2]float64 `yaml:"cloud_dense_offset"`
	CloudParallax         [2]float64 `yaml:"cloud_parallax"`
	HorizonUVScale        float64    `yaml:"horizon_uv_scale"`
	HorizonParallax       float64    `yaml:"horizon_parallax"`
	HorizonHaze           float64    `yaml:"horizon_haze"`
	HorizonEmission       [3]float64 `yaml:"horizon_emission"`
}

// DefaultSettings uses unmodified panorama mapping and no decorative effects.
func DefaultSettings() Settings {
	return Settings{CloudUVScale: [2]float64{1, 1}, CloudDenseScrollScale: 1, HorizonUVScale: 1}
}
