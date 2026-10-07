package worldsource

import "fmt"

// SecondarySettings describes one colour-only detail image in source v7.
// Nil fields inherit room settings; Enabled=false disables the resolved layer.
type SecondarySettings struct {
	Enabled  *bool       `json:"enabled,omitempty"  yaml:"enabled,omitempty"`
	Texture  string      `json:"texture,omitempty"  yaml:"texture,omitempty"`
	Strength *float64    `json:"strength,omitempty" yaml:"strength,omitempty"`
	UV       *UVSettings `json:"uv,omitempty"       yaml:"uv,omitempty"`
}

// BandSettings adds optional horizontal trim textures to visible wall spans.
// Builders resolve texture references, coverage and UVs before packaging.
type BandSettings struct {
	Enabled *bool                   `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	Top     *HorizontalBandSettings `json:"top,omitempty"     yaml:"top,omitempty"`
	Bottom  *HorizontalBandSettings `json:"bottom,omitempty"  yaml:"bottom,omitempty"`
}

type HorizontalBandSettings struct {
	Enabled     *bool    `json:"enabled,omitempty"      yaml:"enabled,omitempty"`
	Texture     string   `json:"texture,omitempty"      yaml:"texture,omitempty"`
	Height      *float64 `json:"height,omitempty"       yaml:"height,omitempty"`
	RepeatWidth *float64 `json:"repeat_width,omitempty" yaml:"repeat_width,omitempty"`
	Offset      *Vec2    `json:"offset,omitempty"       yaml:"offset,omitempty"`
}

func ValidateBandSettings(b *BandSettings) error {
	if b == nil {
		return ErrReference
	}
	for _, h := range []*HorizontalBandSettings{b.Top, b.Bottom} {
		if h == nil {
			continue
		}
		if h.Texture != "" && !validIdentifier(h.Texture) {
			return ErrIdentity
		}
		if !validBandSize(h.Height) || !validBandSize(h.RepeatWidth) || h.Offset != nil && !validVec2(*h.Offset) {
			return ErrBounds
		}
	}
	return nil
}
func validBandSize(p *float64) bool { return p == nil || finite(*p) && *p >= MinUVScale }
func ValidateSecondarySettings(s *SecondarySettings) error {
	if s == nil {
		return ErrReference
	}
	if s.Texture != "" && !validIdentifier(s.Texture) {
		return ErrIdentity
	}
	if s.Strength != nil && (!finite(*s.Strength) || *s.Strength < 0 || *s.Strength > 1) {
		return ErrBounds
	}
	if s.UV != nil {
		return ValidateUVSettings(s.UV)
	}
	return nil
}
func validateDocumentMaterials(d *Document) error {
	rooms := func(rooms []Room) error {
		for _, r := range rooms {
			for index, s := range []*SecondarySettings{r.FloorSecondary, r.CeilingSecondary, r.WallSecondary} {
				if s != nil {
					if d.Version < MaterialsVersion {
						return ErrVersion
					}
					if err := ValidateSecondarySettings(s); err != nil {
						return err
					}
					if s.UV != nil && index < 2 && (s.UV.Mode == UVWrap || s.UV.Anchor == UVTop || s.UV.Anchor == UVBottom) {
						return ErrUVMapping
					}
				}
			}
			if r.WallBands != nil {
				if d.Version < MaterialsVersion {
					return ErrVersion
				}
				if err := ValidateBandSettings(r.WallBands); err != nil {
					return err
				}
			}
			for _, e := range r.Boundary {
				if e.Bands != nil {
					if d.Version < MaterialsVersion {
						return ErrVersion
					}
					if err := ValidateBandSettings(e.Bands); err != nil {
						return err
					}
				}
				if e.Secondary != nil {
					if d.Version < MaterialsVersion {
						return ErrVersion
					}
					if err := ValidateSecondarySettings(e.Secondary); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	if err := rooms(d.Rooms); err != nil {
		return fmt.Errorf("advanced materials: %w", err)
	}
	for _, p := range d.Prefabs {
		if err := rooms(p.Rooms); err != nil {
			return fmt.Errorf("prefab %q advanced materials: %w", p.ID, err)
		}
	}
	return nil
}

// MergeBandSettings applies non-omitted edge fields over a room default. The
// returned records are independent; nested pointer values remain immutable inputs.
func MergeBandSettings(base, override *BandSettings) *BandSettings {
	if base == nil && override == nil {
		return nil
	}
	out := BandSettings{}
	if base != nil {
		out = *base
	}
	if override != nil {
		if override.Enabled != nil {
			out.Enabled = override.Enabled
		}
	}
	var top, bottom *HorizontalBandSettings
	if override != nil {
		top, bottom = override.Top, override.Bottom
	}
	out.Top = MergeHorizontalBandSettings(out.Top, top)
	out.Bottom = MergeHorizontalBandSettings(out.Bottom, bottom)
	return &out
}
func MergeHorizontalBandSettings(base, override *HorizontalBandSettings) *HorizontalBandSettings {
	if base == nil && override == nil {
		return nil
	}
	out := HorizontalBandSettings{}
	if base != nil {
		out = *base
	}
	if override != nil {
		if override.Enabled != nil {
			out.Enabled = override.Enabled
		}
		if override.Texture != "" {
			out.Texture = override.Texture
		}
		if override.Height != nil {
			out.Height = override.Height
		}
		if override.RepeatWidth != nil {
			out.RepeatWidth = override.RepeatWidth
		}
		if override.Offset != nil {
			out.Offset = override.Offset
		}
	}
	return &out
}
