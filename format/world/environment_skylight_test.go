package world_test

import (
	"math"
	"reflect"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
)

func TestBrightDaylightChangesOnlySkyIllumination(t *testing.T) {
	t.Parallel()
	for _, hour := range []float64{0, 9, 12, 17} {
		settings := world.EnvironmentSettings{TimeOfDay: hour, CloudAmount: .35}
		base, err := world.ResolveEnvironmentSettings(&world.Environment{Version: 1, Preset: world.EnvironmentDaylight}, settings)
		if err != nil {
			t.Fatal(err)
		}
		bright, err := world.ResolveEnvironmentSettings(&world.Environment{Version: 1, Preset: world.EnvironmentDaylightBright}, settings)
		if err != nil {
			t.Fatal(err)
		}
		bright.Preset = base.Preset
		unscale := func(v world.Vec3) world.Vec3 { return world.Vec3{X: v.X / 4, Y: v.Y / 4, Z: v.Z / 4} }
		bright.Ambient = unscale(bright.Ambient)
		cube := &bright.AmbientCube
		for _, face := range []*world.Vec3{&cube.PositiveX, &cube.NegativeX, &cube.PositiveY, &cube.NegativeY, &cube.PositiveZ, &cube.NegativeZ} {
			*face = unscale(*face)
		}
		if !reflect.DeepEqual(base, bright) {
			t.Fatalf("hour %g: brighter sky changed sun, appearance or clock behavior", hour)
		}
	}
	day, _ := world.ResolveEnvironmentSettings(
		&world.Environment{Version: 1, Preset: world.EnvironmentDaylightBright},
		world.EnvironmentSettings{TimeOfDay: 12},
	)
	night, _ := world.ResolveEnvironmentSettings(
		&world.Environment{Version: 1, Preset: world.EnvironmentDaylightBright},
		world.EnvironmentSettings{TimeOfDay: 0},
	)
	if night.SunIntensity != 0 || night.Ambient.X >= day.Ambient.X/4 || math.Abs(day.Ambient.X-.4) > 1e-12 {
		t.Fatal("bright preset lost night scaling or its fixed sky intensity")
	}
}

func TestBrightDaylightRoundTripsWithoutChangingWorldEnvelope(t *testing.T) {
	t.Parallel()
	for _, backdrop := range []world.EnvironmentBackdrop{"", world.EnvironmentBackdropMountains} {
		document := lightingWorld()
		document.Environment = &world.Environment{Version: 1, Preset: world.EnvironmentDaylightBright, Backdrop: backdrop}
		encoded, err := world.Encode(document)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := world.Decode(encoded)
		if err != nil || !reflect.DeepEqual(document, decoded) {
			t.Fatal("bright daylight round trip", err)
		}
		magic := world.BinaryMagic
		if backdrop != "" {
			magic = world.SolidBinaryMagic
		}
		if string(encoded[:4]) != magic {
			t.Fatal("new preset changed the existing envelope")
		}
	}
}
