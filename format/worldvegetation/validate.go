package worldvegetation

import (
	"fmt"
	"math"
	"unicode/utf8"

	"github.com/karty-game/karty-sdk/format/world"
)

func number(v, lo, hi float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= lo && v <= hi }
func identifier(s string) bool {
	return len(s) > 0 && len(s) <= world.MaxIdentifierBytes && utf8.ValidString(s)
}
func validRange(r Range) bool { return number(r.Min, 0.001, MaxSize) && number(r.Max, r.Min, MaxSize) }

// ValidatePreset checks even unused presets before any host resources are allocated.
func ValidatePreset(p *Preset) error {
	if p == nil || !identifier(p.Name) || p.Texture == 0 {
		return ErrIdentity
	}
	if p.Geometry > GeometryPalm {
		return ErrVegetation
	}
	if !validRange(p.Width) || !validRange(p.Height) || !number(p.WindAmplitude, 0, MaxWindAmplitude) ||
		!number(p.WindFrequency, 0, MaxWindFrequency) ||
		!number(p.DrawDistance, 1, MaxDrawDistance) {
		return ErrVegetation
	}
	return nil
}

// Validate checks a complete bounded document. Density zero disables a layer;
// duplicate sector/preset/surface triples are rejected, including disabled ones.
func Validate(d *Document) error {
	if d == nil {
		return ErrVersion
	}
	if _, err := FeatureForVersion(d.Version); err != nil {
		return ErrVersion
	}
	if len(d.Presets) == 0 || len(d.Presets) > MaxPresets || len(d.Layers) == 0 || len(d.Layers) > MaxLayers {
		return ErrBounds
	}
	presets := make(map[string]bool, len(d.Presets))
	for i := range d.Presets {
		p := &d.Presets[i]
		if p.Geometry == GeometryPalm && d.Version < PalmVersion {
			return ErrVersion
		}
		if p.Geometry == GeometryTree && d.Version < TreeVersion {
			return ErrVersion
		}
		if d.Version < NormalVersion && p.NormalTexture != 0 {
			return ErrVersion
		}
		if d.Version == Version && p.Geometry != GeometryCrossedCards {
			return ErrVersion
		}
		if err := ValidatePreset(p); err != nil {
			return fmt.Errorf("preset %q: %w", p.Name, err)
		}
		if presets[p.Name] {
			return ErrIdentity
		}
		presets[p.Name] = true
	}
	type identity struct {
		sector          uint32
		preset, surface string
	}
	layers := make(map[identity]bool, len(d.Layers))
	for _, l := range d.Layers {
		if !presets[l.Preset] {
			return ErrIdentity
		}
		if !number(l.Density, 0, MaxDensity) || l.Surface != SurfaceFloor && l.Surface != SurfaceCeiling {
			return ErrVegetation
		}
		id := identity{l.Sector, l.Preset, l.Surface}
		if layers[id] {
			return ErrIdentity
		}
		layers[id] = true
	}
	return nil
}

// ValidateTextures proves every declared texture identity, including unused
// presets. The caller supplies only successfully decoded texture IDs.
func ValidateTextures(d *Document, textures map[uint32]bool) error {
	if err := Validate(d); err != nil {
		return err
	}
	for _, p := range d.Presets {
		if !textures[p.Texture] || p.NormalTexture != 0 && !textures[p.NormalTexture] {
			return ErrIdentity
		}
	}
	return nil
}

// ValidateForWorld checks sector references and conservative vertical clearance
// against an already validated compiled world. Individual roots and lateral
// support must additionally be clipped by the renderer at opaque/transformed
// boundaries; reciprocal coincident portals with matching planes and sky status
// permit uninterrupted decoration.
func ValidateForWorld(d *Document, w *world.Document) error {
	if err := Validate(d); err != nil {
		return err
	}
	if w == nil {
		return ErrConfinement
	}
	presets := make(map[string]Preset, len(d.Presets))
	for _, p := range d.Presets {
		presets[p.Name] = p
	}
	for _, l := range d.Layers {
		if uint64(l.Sector) >= uint64(len(w.Sectors)) {
			return ErrConfinement
		}
		s := &w.Sectors[int(l.Sector)]
		if len(s.Walls) < 3 || l.Surface == SurfaceCeiling && s.Sky {
			return ErrConfinement
		}
		p := presets[l.Preset]
		radius := p.Width.Max/2 + p.WindAmplitude
		// Cards follow the attachment plane at their base; reserve the opposite
		// plane's worst-case relative slope across all possible lateral support.
		margin := radius * math.Hypot(s.Ceiling.A-s.Floor.A, s.Ceiling.B-s.Floor.B)
		for _, wall := range s.Walls {
			v := wall.Start
			clearance := (s.Ceiling.A-s.Floor.A)*v.X + (s.Ceiling.B-s.Floor.B)*v.Y + s.Ceiling.C - s.Floor.C
			if !number(clearance, p.Height.Max+margin, 2*world.MaxCoordinate) {
				return ErrConfinement
			}
		}
	}
	return nil
}
