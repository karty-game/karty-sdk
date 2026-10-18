package worldsource

import (
	"fmt"

	"github.com/karty-game/karty-sdk/format/worldvegetation"
)

// Vegetation is a root preset library, introduced in source version 13.
type Vegetation struct {
	Version uint16             `yaml:"version"`
	Presets []VegetationPreset `yaml:"presets"`
}
type VegetationPreset struct {
	Name          string                `yaml:"name"`
	Geometry      string                `yaml:"geometry,omitempty"`
	NormalTexture string                `yaml:"normal_texture,omitempty"`
	Texture       string                `yaml:"texture"`
	Width         worldvegetation.Range `yaml:"width"`
	Height        worldvegetation.Range `yaml:"height"`
	WindAmplitude float64               `yaml:"wind_amplitude"`
	WindFrequency float64               `yaml:"wind_frequency"`
	DrawDistance  float64               `yaml:"draw_distance"`
}

// VegetationLayer assigns independent density and seed to one room surface.
// Surface must be explicit; omission does not silently pick a surface.
type VegetationLayer struct {
	Preset  string  `yaml:"preset"`
	Density float64 `yaml:"density"`
	Seed    uint32  `yaml:"seed"`
	Surface string  `yaml:"surface"`
}

func CompileVegetationPreset(p VegetationPreset, textures map[string]uint32) (worldvegetation.Preset, error) {
	return CompileVegetationPresetForVersion(p, textures, worldvegetation.Version)
}

// CompileVegetationPresetForVersion resolves a built-in shape, albedo and optional tangent-space normal map.
func CompileVegetationPresetForVersion(p VegetationPreset, textures map[string]uint32, version uint16) (worldvegetation.Preset, error) {
	result := worldvegetation.Preset{
		Name:          p.Name,
		Texture:       textures[p.Texture],
		Width:         p.Width,
		Height:        p.Height,
		WindAmplitude: p.WindAmplitude,
		WindFrequency: p.WindFrequency,
		DrawDistance:  p.DrawDistance,
	}
	if _, err := worldvegetation.FeatureForVersion(version); err != nil {
		return result, ErrVersion
	}
	switch p.Geometry {
	case "", "crossed_cards":
	case "shrub":
		result.Geometry = worldvegetation.GeometryShrub
	case "tree":
		if version < worldvegetation.TreeVersion {
			return result, ErrVersion
		}
		result.Geometry = worldvegetation.GeometryTree
	case "palm":
		if version < worldvegetation.PalmVersion {
			return result, ErrVersion
		}
		result.Geometry = worldvegetation.GeometryPalm
	default:
		return result, ErrContent
	}
	if version == worldvegetation.Version && p.Geometry != "" {
		return result, ErrVersion
	}
	if !validIdentifier(p.Texture) || result.Texture == 0 {
		return result, ErrReference
	}
	if p.NormalTexture != "" {
		if version < worldvegetation.NormalVersion {
			return result, ErrVersion
		}
		result.NormalTexture = textures[p.NormalTexture]
		if !validIdentifier(p.NormalTexture) || result.NormalTexture == 0 {
			return result, ErrReference
		}
	}
	if err := worldvegetation.ValidatePreset(&result); err != nil {
		return result, fmt.Errorf("vegetation preset: %w: %w", err, ErrContent)
	}
	return result, nil
}
func validateVegetation(d *Document) error {
	count := 0
	for _, r := range d.Rooms {
		count += len(r.VegetationLayers)
	}
	for _, p := range d.Prefabs {
		for _, r := range p.Rooms {
			count += len(r.VegetationLayers)
		}
	}
	if d.Vegetation == nil && count == 0 {
		return nil
	}
	if d.Version < VegetationVersion {
		return ErrVersion
	}
	if d.Vegetation == nil {
		return ErrReference
	}
	v := d.Vegetation
	if _, err := worldvegetation.FeatureForVersion(v.Version); err != nil {
		return ErrVersion
	}
	if v.Version >= worldvegetation.GeometryVersion && d.Version < VegetationGeometryVersion {
		return ErrVersion
	}
	if v.Version >= worldvegetation.NormalVersion && d.Version < VegetationNormalVersion {
		return ErrVersion
	}
	if v.Version >= worldvegetation.PalmVersion && d.Version < VegetationPalmVersion {
		return ErrVersion
	}
	if v.Version >= worldvegetation.TreeVersion && d.Version < VegetationTreeVersion {
		return ErrVersion
	}
	if len(v.Presets) == 0 || len(v.Presets) > worldvegetation.MaxPresets || count > worldvegetation.MaxLayers {
		return ErrBounds
	}
	document := worldvegetation.Document{Version: v.Version}
	textures := make(map[string]uint32, len(v.Presets))
	for _, p := range v.Presets {
		if textures[p.Texture] == 0 {
			textures[p.Texture] = uint32(len(textures) + 1)
		}
		if p.NormalTexture != "" && textures[p.NormalTexture] == 0 {
			textures[p.NormalTexture] = uint32(len(textures) + 1)
		}
		compiled, err := CompileVegetationPresetForVersion(p, textures, v.Version)
		if err != nil {
			return err
		}
		document.Presets = append(document.Presets, compiled)
	}
	sector := uint32(0)
	room := func(r Room) error {
		for _, l := range r.VegetationLayers {
			if l.Surface == worldvegetation.SurfaceCeiling && r.Sky {
				return ErrContent
			}
			document.Layers = append(
				document.Layers,
				worldvegetation.Layer{Sector: sector, Preset: l.Preset, Density: l.Density, Seed: l.Seed, Surface: l.Surface},
			)
		}
		sector++
		return nil
	}
	for _, r := range d.Rooms {
		if err := room(r); err != nil {
			return err
		}
	}
	for _, p := range d.Prefabs {
		for _, r := range p.Rooms {
			if err := room(r); err != nil {
				return err
			}
		}
	}
	// A reusable library with no authored placements is valid, but all presets
	// must still pass the same complete uniqueness/configuration checks.
	if len(document.Layers) == 0 {
		document.Layers = []worldvegetation.Layer{{Preset: v.Presets[0].Name, Surface: worldvegetation.SurfaceFloor}}
	}
	if err := worldvegetation.Validate(&document); err != nil {
		return fmt.Errorf("vegetation: %w: %w", err, ErrContent)
	}
	return nil
}
