package world

import "github.com/karty-game/karty-sdk/format/cartridge"

const ExplicitEnvironmentVersion = uint32(2)
const FeatureEnvironmentAppearance = cartridge.FeatureWorldEnvironmentV2

type EnvironmentLighting struct {
	SunColor     Vec3         `yaml:"sun_color"`
	SunIntensity float64      `yaml:"sun_intensity"`
	Ambient      Vec3         `yaml:"ambient"`
	AmbientCube  *AmbientCube `yaml:"ambient_cube,omitempty"`
}

// EnvironmentColors configures the asset-free fallback and sky texture tints.
type EnvironmentColors struct {
	Zenith  Vec3 `yaml:"zenith"`
	Horizon Vec3 `yaml:"horizon"`
	Nadir   Vec3 `yaml:"nadir"`
	Cloud   Vec3 `yaml:"cloud"`
	Sun     Vec3 `yaml:"sun"`
}
type EnvironmentAppearance struct {
	Lighting EnvironmentLighting
	Colors   EnvironmentColors
}

// DefaultEnvironmentColors is a neutral fallback; project sky artwork remains independent.
func DefaultEnvironmentColors() EnvironmentColors {
	return EnvironmentColors{
		Zenith:  Vec3{X: .055, Y: .22, Z: .56},
		Horizon: Vec3{X: .57, Y: .72, Z: .83},
		Nadir:   Vec3{X: .12, Y: .16, Z: .2},
		Cloud:   Vec3{X: .94, Y: .96, Z: 1},
		Sun:     Vec3{X: 1, Y: .96, Z: .78},
	}
}
func validAppearanceColor(c Vec3, upperBound float64) bool {
	for _, v := range [...]float64{c.X, c.Y, c.Z} {
		if !finiteBounded(v) || v < 0 || v > upperBound {
			return false
		}
	}
	return true
}
func validateEnvironmentAppearance(a *EnvironmentAppearance) error {
	if a == nil || !validAppearanceColor(a.Lighting.SunColor, 1) || !validAppearanceColor(a.Lighting.Ambient, 4) ||
		!finiteBounded(a.Lighting.SunIntensity) ||
		a.Lighting.SunIntensity < 0 ||
		a.Lighting.SunIntensity > 16 {
		return ErrEnvironment
	}
	if c := a.Lighting.AmbientCube; c != nil {
		for _, v := range [...]Vec3{c.PositiveX, c.NegativeX, c.PositiveY, c.NegativeY, c.PositiveZ, c.NegativeZ} {
			if !validAppearanceColor(v, 8) {
				return ErrEnvironment
			}
		}
	}
	for _, v := range [...]Vec3{a.Colors.Zenith, a.Colors.Horizon, a.Colors.Nadir, a.Colors.Cloud, a.Colors.Sun} {
		if !validAppearanceColor(v, 1) {
			return ErrEnvironment
		}
	}
	return nil
}

// CloneEnvironmentAppearance preserves ownership of the optional directional cube.
func CloneEnvironmentAppearance(a *EnvironmentAppearance) *EnvironmentAppearance {
	if a == nil {
		return nil
	}
	value := *a
	if a.Lighting.AmbientCube != nil {
		cube := *a.Lighting.AmbientCube
		value.Lighting.AmbientCube = &cube
	}
	return &value
}
