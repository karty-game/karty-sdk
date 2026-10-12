package worldsource_test

import (
	"errors"
	"testing"

	"github.com/karty-game/karty-sdk/format/worldparticles"
	"github.com/karty-game/karty-sdk/format/worldsource"
)

func alphaSourcePreset() worldsource.ParticlePreset {
	return worldsource.ParticlePreset{Name: "smoke", Capacity: 1,
		Emission:        worldparticles.Emission{Bursts: []worldparticles.Burst{{Count: 1}}},
		LifetimeSeconds: worldparticles.Range{Min: .2, Max: .5}, Spawn: worldsource.ParticleSpawn{Shape: "point"},
		Sprite:        worldsource.ParticleSprite{Frames: []string{"smoke"}, Playback: "once", Facing: "camera-facing", Blend: "alpha"},
		SizeOverLife:  []worldparticles.ScalarKey{{Age: 0, Value: .1}, {Age: 1, Value: .2}},
		ColorOverLife: []worldparticles.ColorKey{{Age: 0, RGBA: [4]float64{.1, .1, .1, 0}}, {Age: 1}},
	}
}

func TestAlphaParticlesRequireSource11AndVersionedCompiler(t *testing.T) {
	t.Parallel()
	preset := alphaSourcePreset()
	textures := map[string]uint32{"smoke": 1}
	if _, err := worldsource.CompileParticlePreset(preset, textures); !errors.Is(err, worldsource.ErrContent) {
		t.Fatal("legacy source helper accepted alpha", err)
	}
	if _, err := worldsource.CompileParticlePresetForVersion(preset, textures, worldparticles.AlphaVersion); err != nil {
		t.Fatal(err)
	}
	premultiplied := preset
	premultiplied.Sprite.Blend = "premultiplied"
	if _, err := worldsource.CompileParticlePreset(premultiplied, textures); !errors.Is(err, worldsource.ErrContent) {
		t.Fatal("legacy source helper accepted premultiplied", err)
	}
	if _, err := worldsource.CompileParticlePresetForVersion(premultiplied, textures, worldparticles.AlphaVersion); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		source, particles uint16
		blend             string
		ok                bool
	}{
		{10, 1, "additive", true}, {11, 1, "additive", true}, {10, 2, "additive", false},
		{10, 2, "alpha", false}, {11, 1, "alpha", false}, {11, 2, "alpha", true}, {11, 3, "additive", false},
		{10, 2, "premultiplied", false}, {11, 1, "premultiplied", false}, {11, 2, "premultiplied", true},
	} {
		document := validSource()
		document.Version = test.source
		selected := preset
		selected.Sprite.Blend = test.blend
		document.Particles = &worldsource.Particles{
			Version: test.particles,
			Presets: []worldsource.ParticlePreset{selected},
			Emitters: []worldsource.ParticleEmitter{
				{ID: "smoke", Preset: "smoke", Position: worldsource.Vec3{Z: 1}, Playback: worldparticles.Playback{Mode: "once"}},
			},
		}
		err := worldsource.Validate(&document)
		if (err == nil) != test.ok {
			t.Fatalf("source%d/particles%d/%s: %v", test.source, test.particles, test.blend, err)
		}
	}
}
