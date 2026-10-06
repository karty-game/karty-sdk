package worldsource

import (
	"errors"
	"fmt"

	sdkworld "github.com/karty-game/karty-sdk/format/world"
)

// Lighting authors the world/lighting@1 payload. Version is the nested
// payload version (sdkworld.LightingVersion, 1), not the source document version.
// Ambient and Color use linear RGB intensities in [0,1]. Lights retain authored
// order and global positions; prefab-local lights are not supported.
type Lighting struct {
	Version     uint32       `json:"version"                yaml:"version"`
	Ambient     Vec3         `json:"ambient"                yaml:"ambient"`
	Lights      []PointLight `json:"lights"                 yaml:"lights"`
	AmbientCube *AmbientCube `json:"ambient_cube,omitempty" yaml:"ambient_cube,omitempty"`
	Actors      bool         `json:"actors,omitempty"       yaml:"actors,omitempty"`
}

// AmbientCube supplies six world-space directional ambient colors. It replaces
// Ambient when present; squared unit-normal components weight each selected face.
// Colors are finite linear RGB intensities in [0,1].
type AmbientCube struct {
	PositiveX Vec3 `json:"positive_x" yaml:"positive_x"`
	NegativeX Vec3 `json:"negative_x" yaml:"negative_x"`
	PositiveY Vec3 `json:"positive_y" yaml:"positive_y"`
	NegativeY Vec3 `json:"negative_y" yaml:"negative_y"`
	PositiveZ Vec3 `json:"positive_z" yaml:"positive_z"`
	NegativeZ Vec3 `json:"negative_z" yaml:"negative_z"`
}

// PointLight has the same bounds and semantics as sdkworld.PointLight.
type PointLight struct {
	ID       string       `json:"id"               yaml:"id"`
	Position Vec3         `json:"position"         yaml:"position"`
	Color    Vec3         `json:"color"            yaml:"color"`
	Radius   float64      `json:"radius"           yaml:"radius"`
	Motion   *LightMotion `json:"motion,omitempty" yaml:"motion,omitempty"`
}

// LightMotion authors the bounded version-1 runtime light path.
type LightMotion struct {
	Version       uint32  `json:"version"        yaml:"version"`
	Offset        Vec3    `json:"offset"         yaml:"offset"`
	PeriodSeconds float64 `json:"period_seconds" yaml:"period_seconds"`
}

func validateLighting(lighting *Lighting) error {
	if lighting.Version != sdkworld.LightingVersion {
		return fmt.Errorf("lighting: %w", ErrVersion)
	}
	if len(lighting.Lights) > MaxLights {
		return fmt.Errorf("lighting: %w", ErrBounds)
	}
	// Reuse the compiled contract's validation without a dependency on any
	// authoring parser or compiler, and bound conversion storage before copying.
	var lights [MaxLights]sdkworld.PointLight
	var motions [MaxLights]sdkworld.LightMotion
	for index, light := range lighting.Lights {
		lights[index] = sdkworld.PointLight{
			ID: light.ID, Position: sdkworld.Vec3(light.Position),
			Color: sdkworld.Vec3(light.Color), Radius: light.Radius,
		}
		if light.Motion != nil {
			motions[index] = sdkworld.LightMotion{
				Version:       light.Motion.Version,
				Offset:        sdkworld.Vec3(light.Motion.Offset),
				PeriodSeconds: light.Motion.PeriodSeconds,
			}
			lights[index].Motion = &motions[index]
		}
	}
	compiled := sdkworld.Lighting{
		Version: lighting.Version, Ambient: sdkworld.Vec3(lighting.Ambient),
		Lights: lights[:len(lighting.Lights)], Actors: lighting.Actors,
	}
	if cube := lighting.AmbientCube; cube != nil {
		compiled.AmbientCube = &sdkworld.AmbientCube{
			PositiveX: sdkworld.Vec3(cube.PositiveX),
			NegativeX: sdkworld.Vec3(cube.NegativeX),
			PositiveY: sdkworld.Vec3(cube.PositiveY),
			NegativeY: sdkworld.Vec3(cube.NegativeY),
			PositiveZ: sdkworld.Vec3(cube.PositiveZ),
			NegativeZ: sdkworld.Vec3(cube.NegativeZ),
		}
	}
	if err := sdkworld.ValidateLighting(&compiled); err != nil {
		if errors.Is(err, sdkworld.ErrIdentity) {
			//nolint:errorlint // Wrap the source sentinel, retaining the compiled error as text.
			return fmt.Errorf("lighting: %v: %w", err, ErrIdentity)
		}
		//nolint:errorlint // Wrap the source sentinel, retaining the compiled error as text.
		return fmt.Errorf("lighting: %v: %w", err, ErrLighting)
	}

	return nil
}
