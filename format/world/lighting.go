package world

import "fmt"

// Lighting is the independently versioned world/lighting@1 payload.
// Ambient and point colors are linear RGB intensities in [0,1]. Lights retain
// authored order and global world coordinates, without sector assignment,
// prefab transforms, shadows, portal transport or PBR parameters.
type Lighting struct {
	Version     uint32       `json:"version"`
	Ambient     Vec3         `json:"ambient"`
	Lights      []PointLight `json:"lights"`
	AmbientCube *AmbientCube `json:"ambient_cube,omitempty"`
	Actors      bool         `json:"actors,omitempty"`
}

// AmbientCube supplies six world-space directional ambient colors. It replaces
// Ambient when present; squared unit-normal components weight each selected face.
// Colors are finite linear RGB intensities in [0,1].
type AmbientCube struct {
	PositiveX Vec3 `json:"positive_x"`
	NegativeX Vec3 `json:"negative_x"`
	PositiveY Vec3 `json:"positive_y"`
	NegativeY Vec3 `json:"negative_y"`
	PositiveZ Vec3 `json:"positive_z"`
	NegativeZ Vec3 `json:"negative_z"`
}

// PointLight is one bounded global light. ID is unique within Lighting;
// Radius is an influence radius in world units in [MinLightRadius, MaxCoordinate].
type PointLight struct {
	ID       string       `json:"id"`
	Position Vec3         `json:"position"`
	Color    Vec3         `json:"color"`
	Radius   float64      `json:"radius"`
	Motion   *LightMotion `json:"motion,omitempty"`
}

// LightMotion version 1 moves a light smoothly from Position to Position+Offset
// and back once per PeriodSeconds. It is runtime direct lighting, never baked.
// The complete segment must stay within the ordinary light coordinate bounds.
type LightMotion struct {
	Version       uint32  `json:"version"`
	Offset        Vec3    `json:"offset"`
	PeriodSeconds float64 `json:"period_seconds"`
}

// ValidateLighting checks the complete lighting payload without requiring
// sector geometry. Optional absence is represented by Document.Lighting == nil;
// passing nil here is invalid. Capability/world consistency is the consumer's
// responsibility because the payload does not contain a cartridge manifest.
func ValidateLighting(lighting *Lighting) error {
	if lighting == nil {
		return ErrLighting
	}
	if lighting.Version != LightingVersion {
		return fmt.Errorf("lighting: %w", ErrVersion)
	}
	if len(lighting.Lights) > MaxLights {
		return fmt.Errorf("lighting: %w", ErrBounds)
	}
	if !validLightColor(lighting.Ambient) {
		return fmt.Errorf("lighting ambient: %w", ErrLighting)
	}
	if cube := lighting.AmbientCube; cube != nil {
		for index, color := range [...]Vec3{cube.PositiveX, cube.NegativeX, cube.PositiveY, cube.NegativeY, cube.PositiveZ, cube.NegativeZ} {
			if !validLightColor(color) {
				return fmt.Errorf("lighting ambient cube face %d: %w", index, ErrLighting)
			}
		}
	}
	for index, light := range lighting.Lights {
		if !validIdentifier(light.ID) {
			return fmt.Errorf("light %d: %w", index, ErrIdentity)
		}
		// The fixed 50-light bound permits checking identities without an index
		// allocation and preserves the authored list order.
		for previous := range index {
			if lighting.Lights[previous].ID == light.ID {
				return fmt.Errorf("light %q: %w", light.ID, ErrIdentity)
			}
		}
		if !validVec3(light.Position) || !validLightColor(light.Color) ||
			!finiteBounded(light.Radius) || light.Radius < MinLightRadius {
			return fmt.Errorf("light %q: %w", light.ID, ErrLighting)
		}
		if motion := light.Motion; motion != nil {
			end := Vec3{X: light.Position.X + motion.Offset.X, Y: light.Position.Y + motion.Offset.Y, Z: light.Position.Z + motion.Offset.Z}
			if motion.Version != 1 || !validVec3(motion.Offset) || !validVec3(end) ||
				!finiteBounded(motion.PeriodSeconds) || motion.PeriodSeconds < 0.1 || motion.PeriodSeconds > 3600 {
				return fmt.Errorf("light %q motion: %w", light.ID, ErrLighting)
			}
		}
	}

	return nil
}

func validLightColor(color Vec3) bool {
	return validVec3(color) && color.X >= 0 && color.X <= 1 &&
		color.Y >= 0 && color.Y <= 1 && color.Z >= 0 && color.Z <= 1
}
