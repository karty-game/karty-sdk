package worldsource_test

import (
	"errors"
	"math"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldsky"
	"github.com/karty-game/karty-sdk/format/worldsource"
)

func TestSkySourceGateAndConfinedReferences(t *testing.T) {
	t.Parallel()
	if worldsource.SkyVersion != 12 || worldsource.Version < worldsource.VegetationTreeVersion {
		t.Fatal("source sky gate changed")
	}
	for version := uint16(1); version < worldsource.SkyVersion; version++ {
		document := validSource()
		document.Version = worldsource.FluidsVersion
		document.Version = version
		document.Environment = &worldsource.Environment{Preset: world.EnvironmentDaylight, Sky: &worldsource.Sky{}}
		if err := worldsource.Validate(&document); !errors.Is(err, worldsource.ErrVersion) {
			t.Fatalf("v%d accepted sky: %v", version, err)
		}
	}
	for _, reference := range []string{"../cloud.png", "/cloud.png", "cloud/../cloud.png", "./cloud.png", "a\\cloud.png", "cloud.png\x00", "C:/cloud.png", "cloud.jpg"} {
		document := validSource()
		document.Version = worldsource.FluidsVersion
		document.Environment = &worldsource.Environment{Preset: world.EnvironmentDaylight, Sky: &worldsource.Sky{CloudSparse: reference}}
		if err := worldsource.Validate(&document); !errors.Is(err, worldsource.ErrEnvironment) {
			t.Fatalf("accepted %q: %v", reference, err)
		}
	}
	document := validSource()
	document.Version = worldsource.FluidsVersion
	document.Environment = &worldsource.Environment{Preset: world.EnvironmentDaylight, Sky: &worldsource.Sky{CloudSparse: "sky/cloud.png"}}
	if err := worldsource.Validate(&document); err != nil {
		t.Fatal(err)
	}
	document.Environment.Sky = &worldsource.Sky{}
	if err := worldsource.Validate(&document); err != nil {
		t.Fatalf("image-free sky: %v", err)
	}
}

func TestSkyPartialSettingsPreserveDefaultsAndExplicitZero(t *testing.T) {
	t.Parallel()
	haze, zero := .2, 0.0
	sky := worldsource.Sky{Settings: &worldsource.SkySettings{HorizonHaze: &haze, CloudDenseScrollScale: &zero}}
	resolved := sky.ResolvedSettings()
	if resolved.CloudUVScale != worldsky.DefaultSettings().CloudUVScale || resolved.HorizonHaze != haze ||
		resolved.CloudDenseScrollScale != 0 || resolved.HorizonUVScale != 1 {
		t.Fatalf("settings merge: %+v", resolved)
	}
	document := validSource()
	document.Version = worldsource.FluidsVersion
	document.Environment = &worldsource.Environment{Preset: world.EnvironmentDaylight, Sky: &sky}
	if err := worldsource.Validate(&document); err != nil {
		t.Fatal(err)
	}
	haze = math.NaN()
	if err := worldsource.Validate(&document); !errors.Is(err, worldsource.ErrEnvironment) || !errors.Is(err, worldsky.ErrSettings) {
		t.Fatalf("invalid partial override: %v", err)
	}
}

func TestSkyHorizonScaleOverrideAndBounds(t *testing.T) {
	t.Parallel()
	for _, scale := range []float64{1.5, 4, 0, -1, 4.1, math.NaN(), math.Inf(1)} {
		sky := worldsource.Sky{Settings: &worldsource.SkySettings{HorizonUVScale: &scale}}
		document := validSource()
		document.Version = worldsource.FluidsVersion
		document.Environment = &worldsource.Environment{Preset: world.EnvironmentDaylight, Sky: &sky}
		err := worldsource.Validate(&document)
		if scale == 1.5 || scale == 4 {
			if err != nil || sky.ResolvedSettings().HorizonUVScale != scale {
				t.Fatalf("valid horizon scale %g: %v", scale, err)
			}
		} else if !errors.Is(err, worldsky.ErrSettings) {
			t.Fatalf("accepted invalid horizon scale %g: %v", scale, err)
		}
	}
}
