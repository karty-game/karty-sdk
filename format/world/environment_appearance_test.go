package world_test

import (
	"bytes"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

func explicitEnvironment() *world.Environment {
	return &world.Environment{
		Version: world.ExplicitEnvironmentVersion,
		Appearance: &world.EnvironmentAppearance{
			Lighting: world.EnvironmentLighting{
				SunColor:     world.Vec3{X: .2, Y: .5, Z: 1},
				SunIntensity: 2,
				Ambient:      world.Vec3{X: .4, Y: .2, Z: .8},
			},
			Colors: world.DefaultEnvironmentColors(),
		},
	}
}
func TestExplicitEnvironmentRoundTripOwnershipAndBakeIdentity(t *testing.T) {
	t.Parallel()
	d := lightingWorld()
	d.Environment = explicitEnvironment()
	encoded, err := world.Encode(d)
	if err != nil || !bytes.HasPrefix(encoded, []byte("KWR4")) {
		t.Fatal("explicit environment encoding", err)
	}
	decoded, err := world.Decode(encoded)
	if err != nil || !reflect.DeepEqual(decoded, d) {
		t.Fatal("roundtrip", err)
	}
	resolved, err := world.ResolveEnvironment(decoded.Environment)
	if err != nil || resolved.SunColor != d.Environment.Appearance.Lighting.SunColor || resolved.SunIntensity != 2 ||
		resolved.Ambient.Z != .8 {
		t.Fatal("explicit lighting replaced", resolved, err)
	}
	for n := range len(encoded) {
		if _, err := world.Decode(encoded[:n]); err == nil {
			t.Fatal("truncated KWR4", n)
		}
	}
	for _, magic := range []string{"KWR1", "KWR2", "KWR3"} {
		bad := bytes.Clone(encoded)
		copy(bad, magic)
		if _, err := world.Decode(bad); err == nil {
			t.Fatal("appearance reinterpreted as legacy", magic)
		}
	}
	before, err := worldlightmap.EnvironmentSurfaceDigest(&d)
	if err != nil {
		t.Fatal(err)
	}
	d.Environment.Appearance.Lighting.SunIntensity = 3
	after, err := worldlightmap.EnvironmentSurfaceDigest(&d)
	if err != nil || before == after {
		t.Fatal("lighting omitted from bake identity", err)
	}
	d.Environment = explicitEnvironment()
	d.Environment.Appearance.Lighting.AmbientCube = &world.AmbientCube{
		PositiveX: world.Vec3{X: 2},
		NegativeX: world.Vec3{Y: 3},
		PositiveZ: world.Vec3{Z: 4},
	}
	owned := world.CloneEnvironmentAppearance(d.Environment.Appearance)
	owned.Lighting.AmbientCube.PositiveX.X = 1
	if d.Environment.Appearance.Lighting.AmbientCube.PositiveX.X != 2 {
		t.Fatal("cube aliases source")
	}
	clock, err := world.ResolveEnvironmentSettings(d.Environment, world.EnvironmentSettings{TimeOfDay: 15})
	if err != nil || clock.AmbientCube.PositiveX.X != 2 || clock.AmbientCube.PositiveX.Y != 0 || clock.AmbientCube.NegativeX.Y != 3 {
		t.Fatal("clock replaced custom cube", clock, err)
	}
}
func TestExplicitEnvironmentRejectsLegacySelectorsAndUnboundedValues(t *testing.T) {
	t.Parallel()
	for _, mutate := range []func(*world.Environment){func(e *world.Environment) { e.Preset = world.EnvironmentFantasy }, func(e *world.Environment) { e.Palette = world.EnvironmentPaletteViolet }, func(e *world.Environment) { e.Backdrop = world.EnvironmentBackdropCity }, func(e *world.Environment) { e.Appearance = nil }, func(e *world.Environment) { e.Appearance.Lighting.SunIntensity = math.NaN() }, func(e *world.Environment) { e.Appearance.Lighting.SunIntensity = 17 }, func(e *world.Environment) { e.Appearance.Lighting.SunColor.X = 2 }, func(e *world.Environment) { e.Appearance.Colors.Cloud.Z = -1 }} {
		e := explicitEnvironment()
		mutate(e)
		if err := world.ValidateEnvironment(e); !errors.Is(err, world.ErrEnvironment) {
			t.Fatal("invalid appearance accepted", err)
		}
	}
	legacy := &world.Environment{Version: 1, Preset: world.EnvironmentFantasy}
	before, err := world.ResolveEnvironment(legacy)
	if err != nil || before.SunIntensity != .68 {
		t.Fatal("released preset changed", err)
	}
	legacy.Appearance = explicitEnvironment().Appearance
	if err := world.ValidateEnvironment(legacy); err == nil {
		t.Fatal("legacy appearance extension accepted")
	}
}
