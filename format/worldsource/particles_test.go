package worldsource_test

import (
	"errors"
	"testing"

	"github.com/karty-game/karty-sdk/format/worldparticles"
	"github.com/karty-game/karty-sdk/format/worldsource"
)

func TestParticleSourceCompilerRejectsUnboundedSlicesBeforeCloning(t *testing.T) {
	t.Parallel()
	for _, preset := range []worldsource.ParticlePreset{
		{Sprite: worldsource.ParticleSprite{Frames: make([]string, worldparticles.MaxFrames+1)}},
		{Emission: worldparticles.Emission{Bursts: make([]worldparticles.Burst, worldparticles.MaxBursts+1)}},
		{SizeOverLife: make([]worldparticles.ScalarKey, worldparticles.MaxCurveKeys+1)},
		{ColorOverLife: make([]worldparticles.ColorKey, worldparticles.MaxCurveKeys+1)},
	} {
		if _, err := worldsource.CompileParticlePreset(preset, nil); !errors.Is(err, worldsource.ErrBounds) {
			t.Fatalf("unbounded source list accepted: %v", err)
		}
	}
}

func TestParticleSourceVersionAndAbsentCompatibility(t *testing.T) {
	t.Parallel()
	if worldsource.Version != 12 || worldsource.ParticlesVersion != 10 {
		t.Fatal("particle source version changed")
	}
	for version := uint16(1); version <= worldsource.Version; version++ {
		document := validSource()
		document.Version = version
		if err := worldsource.Validate(&document); err != nil {
			t.Fatalf("absent particles broke version %d: %v", version, err)
		}
		if version >= 10 {
			continue
		}
		document.Particles = &worldsource.Particles{Version: 1}
		if err := worldsource.Validate(&document); !errors.Is(err, worldsource.ErrVersion) {
			t.Fatalf("old version %d accepted particles: %v", version, err)
		}
	}
}

func TestParticleOnlyPrefabIsACompleteSourceDefinition(t *testing.T) {
	t.Parallel()
	document := validSource()
	document.Particles = &worldsource.Particles{
		Version: worldparticles.Version,
		Presets: []worldsource.ParticlePreset{{
			Name: "effect", Capacity: 1,
			Emission:        worldparticles.Emission{Bursts: []worldparticles.Burst{{Count: 1}}},
			LifetimeSeconds: worldparticles.Range{Min: .2, Max: .5},
			Spawn:           worldsource.ParticleSpawn{Shape: "point"},
			Sprite: worldsource.ParticleSprite{
				Frames: []string{"spark"}, Playback: "once", Facing: "camera-facing", Blend: "additive",
			},
			SizeOverLife:  []worldparticles.ScalarKey{{Age: 0, Value: .1}, {Age: 1}},
			ColorOverLife: []worldparticles.ColorKey{{Age: 0, RGBA: [4]float64{1, 1, 1, 1}}, {Age: 1}},
		}},
	}
	document.Prefabs = append(document.Prefabs, worldsource.Prefab{
		ID: "effect-only",
		ParticleEmitters: []worldsource.ParticleEmitter{{
			ID: "spark", Preset: "effect", Position: worldsource.Vec3{Z: 1}, Playback: worldparticles.Playback{Mode: "once"},
		}},
	})
	if err := worldsource.Validate(&document); err != nil {
		t.Fatalf("particle-only prefab required unrelated geometry/content: %v", err)
	}
	document.Prefabs[len(document.Prefabs)-1].ParticleEmitters = nil
	if err := worldsource.Validate(&document); !errors.Is(err, worldsource.ErrBounds) {
		t.Fatalf("empty prefab was accepted: %v", err)
	}
}
