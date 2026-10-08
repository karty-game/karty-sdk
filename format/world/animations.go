package world

import (
	"fmt"
	"math"

	"github.com/karty-game/karty-sdk/format/cartridge"
)

const (
	AnimationVersion      = uint32(1)
	FeatureAnimations     = cartridge.FeatureWorldAnimationsV1
	MaxAnimationPresets   = 64
	MaxMaterialAnimations = 196
	MaxAnimationFrames    = 64
	MaxAnimationSeconds   = 86400.0
)

// Animations is the optional world/animations@1 payload in compiled world v3.
type Animations struct {
	Version   uint32              `json:"version"`
	Presets   []AnimationPreset   `json:"presets"`
	Materials []MaterialAnimation `json:"materials,omitempty"`
}

// AnimationPreset contains exactly the fields relevant to Kind. Speeds are
// radians/second for spin and liquid; liquid flow and amplitude use UV units.
// SurfaceAmplitude is visual displacement in world metres on horizontal up-facing
// surfaces. Opacity defaults to 1; PixelSize defaults to 0 (continuous sampling).
// Axis, Pivot and Offset are actor-local, before actor scale and orientation.
type AnimationPreset struct {
	Name             string   `json:"name"`
	Kind             string   `json:"kind"`
	Frames           []uint32 `json:"frames,omitempty"`
	FPS              float64  `json:"fps,omitempty"`
	IntervalSeconds  float64  `json:"interval_seconds,omitempty"`
	Flow             Vec2     `json:"flow,omitempty"`
	Amplitude        float64  `json:"amplitude,omitempty"`
	SurfaceAmplitude float64  `json:"surface_amplitude,omitempty"`
	Opacity          *float64 `json:"opacity,omitempty"`
	PixelSize        uint32   `json:"pixel_size,omitempty"`
	Frequency        float64  `json:"frequency,omitempty"`
	Speed            float64  `json:"speed,omitempty"`
	Axis             Vec3     `json:"axis,omitempty"`
	Pivot            Vec3     `json:"pivot,omitempty"`
	Offset           Vec3     `json:"offset,omitempty"`
	PeriodSeconds    float64  `json:"period_seconds,omitempty"`
}

// AnimationBinding selects a named preset and a nonnegative clock offset.
type AnimationBinding struct {
	Preset       string  `json:"preset"`
	PhaseSeconds float64 `json:"phase_seconds,omitempty"`
}

type MaterialAnimation struct {
	Material     uint32  `json:"material"`
	Preset       string  `json:"preset"`
	PhaseSeconds float64 `json:"phase_seconds,omitempty"`
}

// ValidateAnimationPreset checks kind-specific bounds and rejects fields from
// other kinds. Flipbook holds frame zero outside its optional repeat burst.
func ValidateAnimationPreset(p *AnimationPreset) error {
	if p == nil || !validIdentifier(p.Name) {
		return ErrIdentity
	}
	zero2, zero3 := Vec2{}, Vec3{}
	if !finiteBounded(p.SurfaceAmplitude) || p.SurfaceAmplitude < 0 || p.SurfaceAmplitude > .25 || p.PixelSize > 32 ||
		p.Opacity != nil && (!finiteBounded(*p.Opacity) || *p.Opacity < 0 || *p.Opacity > 1) {
		return ErrBounds
	}
	if !finiteBounded(p.FPS) || !animationSeconds(p.IntervalSeconds, true) || !validVec2(p.Flow) || !finiteBounded(p.Amplitude) ||
		!finiteBounded(p.Frequency) ||
		!finiteBounded(p.Speed) ||
		!validVec3(p.Axis) ||
		!validVec3(p.Pivot) ||
		!validVec3(p.Offset) ||
		!animationSeconds(p.PeriodSeconds, true) {
		return ErrBounds
	}
	surfaceZero := p.Flow == zero2 && p.Amplitude == 0 && p.Frequency == 0 && p.SurfaceAmplitude == 0 && p.Opacity == nil &&
		p.PixelSize == 0
	motionZero := p.Axis == zero3 && p.Pivot == zero3 && p.Offset == zero3 && p.PeriodSeconds == 0
	framesZero := len(p.Frames) == 0 && p.FPS == 0 && p.IntervalSeconds == 0
	switch p.Kind {
	case "flipbook":
		if len(p.Frames) < 2 || len(p.Frames) > MaxAnimationFrames || p.FPS < 0.001 || p.FPS > 120 || !surfaceZero || !motionZero ||
			p.Speed != 0 ||
			p.IntervalSeconds != 0 && p.IntervalSeconds < float64(len(p.Frames))/p.FPS {
			return ErrBounds
		}
		for _, id := range p.Frames {
			if id == 0 {
				return ErrContent
			}
		}
	case "liquid":
		if !framesZero || !motionZero || math.Abs(p.Flow.X) > 10 || math.Abs(p.Flow.Y) > 10 || p.Amplitude < 0 || p.Amplitude > 1 ||
			p.Frequency <= 0 ||
			p.Frequency > 100 ||
			math.Abs(p.Speed) > 100 {
			return ErrBounds
		}
	case "spin":
		length := p.Axis.X*p.Axis.X + p.Axis.Y*p.Axis.Y + p.Axis.Z*p.Axis.Z
		if !framesZero || !surfaceZero || p.Offset != zero3 || p.PeriodSeconds != 0 || math.Abs(length-1) > 1e-6 || p.Speed == 0 ||
			math.Abs(p.Speed) > 100 {
			return ErrBounds
		}
	case "oscillate":
		if !framesZero || !surfaceZero || p.Axis != zero3 || p.Pivot != zero3 || p.Speed != 0 || p.Offset == zero3 ||
			!animationSeconds(p.PeriodSeconds, false) {
			return ErrBounds
		}
	default:
		return ErrContent
	}
	return nil
}

func animationSeconds(v float64, zero bool) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v <= MaxAnimationSeconds && (v >= 0 && zero || v >= 0.001 && !zero)
}

// ValidateAnimations validates presets and bindings together, including complete
// actor references and nonrecursive frame targets. Texture existence is checked
// by the level compiler and host against packaged assets.
func ValidateAnimations(document *Document) error {
	if document == nil {
		return ErrSyntax
	}
	a := document.Animations
	presets := map[string]*AnimationPreset{}
	if a != nil {
		if document.Version != Version || a.Version != AnimationVersion {
			return ErrVersion
		}
		if len(a.Presets) == 0 || len(a.Presets) > MaxAnimationPresets || len(a.Materials) > MaxMaterialAnimations {
			return ErrBounds
		}
		for i := range a.Presets {
			p := &a.Presets[i]
			if err := ValidateAnimationPreset(p); err != nil {
				return fmt.Errorf("animation %q: %w", p.Name, err)
			}
			if presets[p.Name] != nil {
				return ErrIdentity
			}
			presets[p.Name] = p
		}

		geometry := *document
		geometry.Animations = nil
		geometryIDs := map[uint32]bool{}
		for _, id := range MaterialIDs(&geometry) {
			geometryIDs[id] = true
		}
		spriteIDs := map[uint32]bool{}
		for _, c := range document.Contents {
			if c.Actor != nil && c.Actor.Sprite != nil {
				spriteIDs[c.Actor.Sprite.AssetID] = true
			}
		}
		bound := map[uint32]bool{}
		for _, b := range a.Materials {
			p := presets[b.Preset]
			if b.Material == 0 || (!geometryIDs[b.Material] && !spriteIDs[b.Material]) ||
				p != nil && p.Kind == "liquid" && !geometryIDs[b.Material] ||
				bound[b.Material] ||
				p == nil ||
				p.Kind != "flipbook" && p.Kind != "liquid" ||
				!animationSeconds(b.PhaseSeconds, true) {
				return ErrContent
			}
			if p.Kind == "liquid" && p.SurfaceAmplitude > 0 && !horizontalLiquidSurface(document, b.Material) {
				return fmt.Errorf("liquid displacement needs a horizontal up-facing surface: %w", ErrContent)
			}
			bound[b.Material] = true
		}
		for _, b := range a.Materials {
			for _, id := range presets[b.Preset].Frames {
				if id != b.Material && bound[id] {
					return fmt.Errorf("recursive material animation: %w", ErrContent)
				}
			}
		}
	}
	for _, c := range document.Contents {
		if c.Actor == nil || c.Actor.Animation == nil {
			continue
		}
		b := c.Actor.Animation
		p := presets[b.Preset]
		if p == nil || !animationSeconds(b.PhaseSeconds, true) || c.Actor.Sprite == nil || p.Kind == "liquid" ||
			(p.Kind == "spin" || p.Kind == "oscillate") && c.Actor.Sprite.Facing != SpriteFixed {
			return fmt.Errorf("content %q animation: %w", c.ID, ErrContent)
		}
	}
	return nil
}

// Other faces sharing the material keep their authored position. Lighting and
// collision use the rest geometry; displaced floors/tops are a rendering effect.
func horizontalLiquidSurface(document *Document, material uint32) bool {
	for _, sector := range document.Sectors {
		if sector.FloorMaterial == material && sector.Floor.A == 0 && sector.Floor.B == 0 {
			return true
		}
	}
	if document.StaticSolids != nil {
		for _, solid := range document.StaticSolids.Items {
			if solid.TopMaterial == material && solid.Top.A == 0 && solid.Top.B == 0 {
				return true
			}
		}
	}
	return false
}
