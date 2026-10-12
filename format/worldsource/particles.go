package worldsource

import (
	"fmt"
	"math"
	"slices"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldparticles"
)

const AlphaParticlesVersion = uint16(11)

type Particles struct {
	Version  uint16            `yaml:"version"`
	Presets  []ParticlePreset  `yaml:"presets"`
	Emitters []ParticleEmitter `yaml:"emitters,omitempty"`
}
type ParticleSpawn struct {
	Shape      string `yaml:"shape"`
	HalfExtent Vec3   `yaml:"half_extent,omitempty"`
}
type ParticleSprite struct {
	Frames   []string `yaml:"frames"`
	FPS      float64  `yaml:"fps,omitempty"`
	Playback string   `yaml:"playback"`
	Facing   string   `yaml:"facing"`
	Blend    string   `yaml:"blend"`
}
type ParticlePreset struct {
	Name                 string                     `yaml:"name"`
	Capacity             uint32                     `yaml:"capacity"`
	Emission             worldparticles.Emission    `yaml:"emission"`
	LifetimeSeconds      worldparticles.Range       `yaml:"lifetime_seconds"`
	Spawn                ParticleSpawn              `yaml:"spawn"`
	VelocityMin          Vec3                       `yaml:"velocity_min,omitempty"`
	VelocityMax          Vec3                       `yaml:"velocity_max,omitempty"`
	Acceleration         Vec3                       `yaml:"acceleration,omitempty"`
	Drag                 float64                    `yaml:"drag,omitempty"`
	Sprite               ParticleSprite             `yaml:"sprite"`
	SizeOverLife         []worldparticles.ScalarKey `yaml:"size_over_life"`
	ColorOverLife        []worldparticles.ColorKey  `yaml:"color_over_life"`
	RotationDegrees      worldparticles.Range       `yaml:"rotation_degrees,omitempty"`
	RotationSpeedDegrees worldparticles.Range       `yaml:"rotation_speed_degrees,omitempty"`
}
type ParticleEmitter struct {
	ID         string                  `yaml:"id"`
	Preset     string                  `yaml:"preset"`
	Position   Vec3                    `yaml:"position"`
	YawDegrees float64                 `yaml:"yaw_degrees,omitempty"`
	Scale      float64                 `yaml:"scale,omitempty"`
	Seed       uint32                  `yaml:"seed"`
	Playback   worldparticles.Playback `yaml:"playback"`
}

// CompileParticlePreset resolves every named frame, including particle-only
// textures. Source and runtime share complete analytical configuration validation.
func CompileParticlePreset(p ParticlePreset, textures map[string]uint32) (worldparticles.Preset, error) {
	return CompileParticlePresetForVersion(p, textures, worldparticles.Version)
}

// CompileParticlePresetForVersion resolves frames under the selected particle
// contract. The legacy CompileParticlePreset helper remains strict version 1.
func CompileParticlePresetForVersion(p ParticlePreset, textures map[string]uint32, version uint16) (worldparticles.Preset, error) {
	if _, err := worldparticles.FeatureForVersion(version); err != nil {
		return worldparticles.Preset{}, ErrVersion
	}
	if len(p.Sprite.Frames) > worldparticles.MaxFrames || len(p.Emission.Bursts) > worldparticles.MaxBursts ||
		len(p.SizeOverLife) > worldparticles.MaxCurveKeys ||
		len(p.ColorOverLife) > worldparticles.MaxCurveKeys {
		return worldparticles.Preset{}, ErrBounds
	}
	result := worldparticles.Preset{
		Name:            p.Name,
		Capacity:        p.Capacity,
		Emission:        p.Emission,
		LifetimeSeconds: p.LifetimeSeconds,
		Spawn: worldparticles.Spawn{
			Shape:      p.Spawn.Shape,
			HalfExtent: world.Vec3(p.Spawn.HalfExtent),
		},
		VelocityMin:  world.Vec3(p.VelocityMin),
		VelocityMax:  world.Vec3(p.VelocityMax),
		Acceleration: world.Vec3(p.Acceleration),
		Drag:         p.Drag,
		Sprite: worldparticles.Sprite{
			FPS:      p.Sprite.FPS,
			Playback: p.Sprite.Playback,
			Facing:   p.Sprite.Facing,
			Blend:    p.Sprite.Blend,
		},
		SizeOverLife:         slices.Clone(p.SizeOverLife),
		ColorOverLife:        slices.Clone(p.ColorOverLife),
		RotationDegrees:      p.RotationDegrees,
		RotationSpeedDegrees: p.RotationSpeedDegrees,
	}
	result.Emission.Bursts = slices.Clone(p.Emission.Bursts)
	for _, name := range p.Sprite.Frames {
		id := textures[name]
		if !validIdentifier(name) || id == 0 {
			return result, fmt.Errorf("particle frame %q: %w", name, ErrReference)
		}
		result.Sprite.Frames = append(result.Sprite.Frames, id)
	}
	if err := worldparticles.ValidatePresetForVersion(&result, version); err != nil {
		return result, fmt.Errorf("particle preset: %w: %w", err, ErrContent)
	}
	return result, nil
}
func validateParticles(d *Document) error {
	present := d.Particles != nil
	count := 0
	for _, r := range d.Rooms {
		count += len(r.ParticleEmitters)
	}
	for _, p := range d.Prefabs {
		count += len(p.ParticleEmitters)
		for _, r := range p.Rooms {
			count += len(r.ParticleEmitters)
		}
	}
	if !present && count == 0 {
		return nil
	}
	if d.Version < ParticlesVersion {
		return ErrVersion
	}
	if d.Particles == nil {
		return ErrReference
	}
	a := d.Particles
	if _, err := worldparticles.FeatureForVersion(a.Version); err != nil ||
		a.Version == worldparticles.AlphaVersion && d.Version < AlphaParticlesVersion {
		return ErrVersion
	}
	if len(a.Presets) == 0 || len(a.Presets) > worldparticles.MaxPresets || len(a.Emitters)+count > worldparticles.MaxEmitters {
		return ErrBounds
	}
	textures := map[string]uint32{}
	for _, p := range a.Presets {
		if len(p.Sprite.Frames) > worldparticles.MaxFrames {
			return ErrBounds
		}
		for _, name := range p.Sprite.Frames {
			if textures[name] == 0 {
				textures[name] = uint32(len(textures) + 1)
			}
		}
	}
	presets := map[string]worldparticles.Preset{}
	for _, p := range a.Presets {
		compiled, err := CompileParticlePresetForVersion(p, textures, a.Version)
		if err != nil {
			return err
		}
		if _, exists := presets[p.Name]; exists {
			return ErrIdentity
		}
		presets[p.Name] = compiled
	}
	emitters := func(items []ParticleEmitter) error {
		ids := map[string]bool{}
		for _, e := range items {
			p, ok := presets[e.Preset]
			if !ok || !validIdentifier(e.ID) || ids[e.ID] {
				return ErrReference
			}
			ids[e.ID] = true
			if !finite(e.YawDegrees) || math.Abs(e.YawDegrees) > 360000 {
				return ErrBounds
			}
			scale := e.Scale
			if scale == 0 {
				scale = 1
			}
			compiled := worldparticles.Emitter{
				ID:         e.ID,
				Preset:     e.Preset,
				Position:   world.Vec3(e.Position),
				YawDegrees: math.Remainder(e.YawDegrees, 360),
				Scale:      scale,
				Seed:       e.Seed,
				Playback:   e.Playback,
			}
			compiled.Bounds = worldparticles.ConservativeEmitterBounds(p, compiled)
			single := worldparticles.Document{
				Version:  a.Version,
				Presets:  []worldparticles.Preset{p},
				Emitters: []worldparticles.Emitter{compiled},
			}
			if err := worldparticles.Validate(&single); err != nil {
				return fmt.Errorf("particle emitter %q: %w: %w", e.ID, err, ErrContent)
			}
		}
		return nil
	}
	if err := emitters(a.Emitters); err != nil {
		return err
	}
	for _, r := range d.Rooms {
		if err := emitters(r.ParticleEmitters); err != nil {
			return err
		}
	}
	for _, p := range d.Prefabs {
		if err := emitters(p.ParticleEmitters); err != nil {
			return err
		}
		for _, r := range p.Rooms {
			if err := emitters(r.ParticleEmitters); err != nil {
				return err
			}
		}
	}
	return nil
}
