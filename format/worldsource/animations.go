package worldsource

import (
	"errors"
	"fmt"

	sdkworld "github.com/karty-game/karty-sdk/format/world"
)

type Animations struct {
	Version   uint32              `yaml:"version"`
	Presets   []AnimationPreset   `yaml:"presets"`
	Materials []MaterialAnimation `yaml:"materials,omitempty"`
}

type AnimationPreset struct {
	Name             string   `yaml:"name"`
	Kind             string   `yaml:"kind"`
	Frames           []string `yaml:"frames,omitempty"`
	FPS              float64  `yaml:"fps,omitempty"`
	IntervalSeconds  float64  `yaml:"interval_seconds,omitempty"`
	Flow             Vec2     `yaml:"flow,omitempty"`
	Amplitude        float64  `yaml:"amplitude,omitempty"`
	SurfaceAmplitude float64  `yaml:"surface_amplitude,omitempty"`
	Opacity          *float64 `yaml:"opacity,omitempty"`
	PixelSize        uint32   `yaml:"pixel_size,omitempty"`
	Frequency        float64  `yaml:"frequency,omitempty"`
	Speed            float64  `yaml:"speed,omitempty"`
	Axis             Vec3     `yaml:"axis,omitempty"`
	Pivot            Vec3     `yaml:"pivot,omitempty"`
	Offset           Vec3     `yaml:"offset,omitempty"`
	PeriodSeconds    float64  `yaml:"period_seconds,omitempty"`
}

type AnimationBinding struct {
	Preset       string  `yaml:"preset"`
	PhaseSeconds float64 `yaml:"phase_seconds,omitempty"`
}

type MaterialAnimation struct {
	Material     string  `yaml:"material"`
	Preset       string  `yaml:"preset"`
	PhaseSeconds float64 `yaml:"phase_seconds,omitempty"`
}

// CompileAnimationPreset converts one authored preset using the level's declared
// texture IDs. All frames must resolve, including frames used only in animations.
func CompileAnimationPreset(p AnimationPreset, textures map[string]uint32) (sdkworld.AnimationPreset, error) {
	if len(p.Frames) > sdkworld.MaxAnimationFrames {
		return sdkworld.AnimationPreset{}, ErrBounds
	}
	result := sdkworld.AnimationPreset{
		Name:             p.Name,
		Kind:             p.Kind,
		FPS:              p.FPS,
		IntervalSeconds:  p.IntervalSeconds,
		Flow:             sdkworld.Vec2(p.Flow),
		Amplitude:        p.Amplitude,
		SurfaceAmplitude: p.SurfaceAmplitude,
		PixelSize:        p.PixelSize,
		Frequency:        p.Frequency,
		Speed:            p.Speed,
		Axis:             sdkworld.Vec3(p.Axis),
		Pivot:            sdkworld.Vec3(p.Pivot),
		Offset:           sdkworld.Vec3(p.Offset),
		PeriodSeconds:    p.PeriodSeconds,
	}
	if p.Opacity != nil {
		result.Opacity = new(*p.Opacity)
	}
	for _, name := range p.Frames {
		id := textures[name]
		if id == 0 || !validIdentifier(name) {
			return sdkworld.AnimationPreset{}, fmt.Errorf("animation %q frame %q: %w", p.Name, name, ErrReference)
		}
		result.Frames = append(result.Frames, id)
	}
	if err := sdkworld.ValidateAnimationPreset(&result); err != nil {
		return sdkworld.AnimationPreset{}, sourceAnimationError(err)
	}
	return result, nil
}

func validateAnimations(d *Document) error {
	compiled := sdkworld.Document{Version: sdkworld.Version}
	textures := map[string]uint32{}
	texture := func(name string) uint32 {
		if !validIdentifier(name) {
			return 0
		}
		if textures[name] == 0 {
			textures[name] = uint32(len(textures) + 1)
		}
		return textures[name]
	}
	if d.Animations != nil {
		if d.Version < AnimationsVersion {
			return ErrVersion
		}
		a := d.Animations
		compiled.Animations = &sdkworld.Animations{Version: a.Version}
		if len(a.Presets) > sdkworld.MaxAnimationPresets || len(a.Materials) > sdkworld.MaxMaterialAnimations {
			return ErrBounds
		}
		for _, p := range a.Presets {
			if len(p.Frames) > sdkworld.MaxAnimationFrames {
				return ErrBounds
			}
			for _, name := range p.Frames {
				texture(name)
			}
			preset, err := CompileAnimationPreset(p, textures)
			if err != nil {
				return err
			}
			compiled.Animations.Presets = append(compiled.Animations.Presets, preset)
		}
		for _, b := range a.Materials {
			compiled.Animations.Materials = append(
				compiled.Animations.Materials,
				sdkworld.MaterialAnimation{Material: texture(b.Material), Preset: b.Preset, PhaseSeconds: b.PhaseSeconds},
			)
		}
	}
	contents := func(items []Content) error {
		for _, c := range items {
			if c.Actor == nil || c.Actor.Animation == nil {
				continue
			}
			if d.Version < AnimationsVersion {
				return ErrVersion
			}
			actor := &sdkworld.Actor{
				Animation: &sdkworld.AnimationBinding{Preset: c.Actor.Animation.Preset, PhaseSeconds: c.Actor.Animation.PhaseSeconds},
			}
			if c.Actor.Sprite != nil {
				actor.Sprite = &sdkworld.Sprite{Facing: sdkworld.SpriteFacing(c.Actor.Sprite.Facing)}
			}
			compiled.Contents = append(compiled.Contents, sdkworld.Content{ID: c.ID, Actor: actor})
		}
		return nil
	}
	if err := contents(d.Contents); err != nil {
		return err
	}
	for _, r := range d.Rooms {
		if err := contents(r.Contents); err != nil {
			return err
		}
	}
	for _, p := range d.Prefabs {
		if err := contents(p.Contents); err != nil {
			return err
		}
		for _, r := range p.Rooms {
			if err := contents(r.Contents); err != nil {
				return err
			}
		}
	}
	for _, id := range textures {
		compiled.Sectors = append(compiled.Sectors, sdkworld.Sector{FloorMaterial: id})
	}
	return sourceAnimationError(sdkworld.ValidateAnimations(&compiled))
}

func sourceAnimationError(err error) error {
	if err == nil {
		return nil
	}
	sentinel := ErrContent
	switch {
	case errors.Is(err, sdkworld.ErrVersion):
		sentinel = ErrVersion
	case errors.Is(err, sdkworld.ErrBounds):
		sentinel = ErrBounds
	case errors.Is(err, sdkworld.ErrIdentity):
		sentinel = ErrIdentity
	}
	return fmt.Errorf("animation: %w: %w", err, sentinel)
}
