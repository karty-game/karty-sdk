package worldsource_test

import (
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldsource"
)

func TestExplicitEnvironmentSourceReplacesPrototypeSelectors(t *testing.T) {
	t.Parallel()
	d := validSource()
	d.Version = worldsource.ExplicitEnvironmentVersion
	d.Environment = &worldsource.Environment{
		Lighting: &world.EnvironmentLighting{SunColor: world.Vec3{X: 1}, SunIntensity: 2, Ambient: world.Vec3{Y: .5}},
	}
	if err := worldsource.Validate(&d); err != nil {
		t.Fatal(err)
	}
	for _, preset := range []world.EnvironmentPreset{world.EnvironmentDaylight, world.EnvironmentDaylightBright, world.EnvironmentSpace, world.EnvironmentCartoon, world.EnvironmentFantasy} {
		d.Environment.Preset = preset
		if err := worldsource.Validate(&d); err == nil {
			t.Fatal("new source accepted prototype", preset)
		}
	}
	d.Environment.Preset = ""
	d.Version = worldsource.FluidsVersion
	if err := worldsource.Validate(&d); err == nil {
		t.Fatal("legacy source accepted appearance")
	}
	d.Environment = &worldsource.Environment{Preset: world.EnvironmentFantasy}
	if err := worldsource.Validate(&d); err != nil {
		t.Fatal("released source stopped decoding", err)
	}
}
