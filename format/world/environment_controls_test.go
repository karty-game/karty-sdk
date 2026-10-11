package world_test

import (
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
)

func TestEnvironmentClockResolvesLightingAndPreservesStaticSun(t *testing.T) {
	t.Parallel()
	for _, preset := range []world.EnvironmentPreset{world.EnvironmentDaylight, world.EnvironmentSpace, world.EnvironmentCartoon, world.EnvironmentFantasy} {
		authored := &world.Environment{Version: 1, Preset: preset}
		base, err := world.ResolveEnvironment(authored)
		if err != nil {
			t.Fatal(err)
		}
		afternoon, err := world.ResolveEnvironmentSettings(authored, world.EnvironmentSettings{TimeOfDay: 15, CloudAmount: .35})
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(afternoon.SunDirection.X-base.SunDirection.X) > 1e-12 ||
			math.Abs(afternoon.SunDirection.Y-base.SunDirection.Y) > 1e-12 ||
			math.Abs(afternoon.SunDirection.Z-base.SunDirection.Z) > 1e-12 ||
			afternoon.SunIntensity != base.SunIntensity ||
			afternoon.SkyZenith != base.SkyZenith {
			t.Fatal("default clock changed the preset", preset)
		}
		noon, _ := world.ResolveEnvironmentSettings(authored, world.EnvironmentSettings{TimeOfDay: 12})
		night, _ := world.ResolveEnvironmentSettings(authored, world.EnvironmentSettings{TimeOfDay: 0, CloudAmount: 1})
		if noon.SunDirection.Z <= afternoon.SunDirection.Z || night.SunDirection.Z >= 0 || night.SunIntensity != 0 ||
			night.SunDiskColor != (world.Vec3{}) ||
			night.Ambient.Z >= noon.Ambient.Z ||
			night.SkyZenith.Z >= noon.SkyZenith.Z ||
			night.CloudAmount != 1 {
			t.Fatal("clock did not coordinate sky and illumination", preset, noon, night)
		}
		authored.Sun = &world.EnvironmentSun{Azimuth: 20, Elevation: 15}
		static, _ := world.ResolveEnvironment(authored)
		if static.SunDirection == base.SunDirection {
			t.Fatal("explicit sun lost")
		}
		live, _ := world.ResolveEnvironmentSettings(authored, world.EnvironmentSettings{TimeOfDay: 15, CloudAmount: .35})
		if live != afternoon {
			t.Fatal("live clock did not override explicit sun")
		}
		cloudOnly, _ := world.ResolveEnvironmentSettings(authored, world.EnvironmentSettings{TimeOfDay: 15, CloudAmount: 1})
		if cloudOnly.SunDirection != live.SunDirection || cloudOnly.SunIntensity != live.SunIntensity {
			t.Fatal("cloud coverage changed the sun")
		}
	}
}

func TestEnvironmentControlsCanonicalBoundsAndPaletteIndependence(t *testing.T) {
	t.Parallel()
	for _, settings := range []world.EnvironmentSettings{{0, 0}, {23.999, 1}, {15, .35}} {
		document := lightingWorld()
		document.Environment = &world.Environment{
			Version:     1,
			Preset:      world.EnvironmentFantasy,
			TimeOfDay:   &settings.TimeOfDay,
			CloudAmount: &settings.CloudAmount,
		}
		encoded, err := world.Encode(document)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := world.Decode(encoded)
		if err != nil || !reflect.DeepEqual(document, decoded) {
			t.Fatal("controls roundtrip", err)
		}
		base, _ := world.ResolveEnvironment(document.Environment)
		document.Environment.Palette = world.EnvironmentPaletteViolet
		violet, _ := world.ResolveEnvironment(document.Environment)
		if violet.SunDirection != base.SunDirection || violet.SunColor != base.SunColor || violet.SunIntensity != base.SunIntensity ||
			violet.Ambient != base.Ambient {
			t.Fatal("palette changed clock lighting")
		}
	}
	for _, value := range []float64{-1, 24, math.Inf(1), math.Inf(-1), math.NaN()} {
		settings := world.EnvironmentSettings{TimeOfDay: value, CloudAmount: .35}
		if !errors.Is(world.ValidateEnvironmentSettings(settings), world.ErrEnvironment) {
			t.Fatal("invalid time accepted", value)
		}
	}
	for _, value := range []float64{-.001, 1.001, math.Inf(1), math.Inf(-1), math.NaN()} {
		settings := world.EnvironmentSettings{TimeOfDay: 15, CloudAmount: value}
		if !errors.Is(world.ValidateEnvironmentSettings(settings), world.ErrEnvironment) {
			t.Fatal("invalid clouds accepted", value)
		}
	}
	timeOfDay := 15.0
	env := &world.Environment{Version: 1, Preset: world.EnvironmentDaylight, Sun: &world.EnvironmentSun{}, TimeOfDay: &timeOfDay}
	if err := world.ValidateEnvironment(env); !errors.Is(err, world.ErrEnvironment) {
		t.Fatal("ambiguous sun and clock accepted", err)
	}
	if _, err := world.ResolveEnvironmentSettings(nil, world.EnvironmentSettings{TimeOfDay: 15}); !errors.Is(err, world.ErrEnvironment) {
		t.Fatal("nil environment", err)
	}
}

func TestEnvironmentLiveResolutionAllocatesNothing(t *testing.T) {
	env := &world.Environment{Version: 1, Preset: world.EnvironmentDaylight}
	settings := world.EnvironmentSettings{TimeOfDay: 2, CloudAmount: 1}
	if n := testing.AllocsPerRun(100, func() {
		if _, err := world.ResolveEnvironmentSettings(env, settings); err != nil {
			t.Fatal(err)
		}
	}); n != 0 {
		t.Fatal("live environment allocates", n)
	}
}
