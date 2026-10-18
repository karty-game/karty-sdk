package world_test

import (
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
)

func TestEnvironmentPresetsCanonicalAndPaletteIndependentLighting(t *testing.T) {
	t.Parallel()
	for _, preset := range []world.EnvironmentPreset{world.EnvironmentDaylight, world.EnvironmentSpace, world.EnvironmentCartoon, world.EnvironmentFantasy} {
		t.Run(string(preset), func(t *testing.T) {
			t.Parallel()
			base, err := world.ResolveEnvironment(&world.Environment{Version: 1, Preset: preset})
			if err != nil {
				t.Fatal(err)
			}
			length := math.Sqrt(
				base.SunDirection.X*base.SunDirection.X + base.SunDirection.Y*base.SunDirection.Y + base.SunDirection.Z*base.SunDirection.Z,
			)
			if math.Abs(length-1) > 1e-12 || base.SunIntensity <= 0 || base.SunIntensity > 1 {
				t.Fatalf("invalid default sun: %+v", base)
			}
			for _, color := range []world.Vec3{base.SkyZenith, base.SkyHorizon, base.SkyNadir, base.CloudColor, base.SunDiskColor,
				base.SunColor, base.Ambient, base.AmbientCube.PositiveZ, base.AmbientCube.NegativeZ} {
				for _, value := range []float64{color.X, color.Y, color.Z} {
					if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
						t.Fatalf("unbounded preset color: %+v", color)
					}
				}
			}
			for _, palette := range []world.EnvironmentPalette{"", world.EnvironmentPaletteDefault, world.EnvironmentPaletteViolet, world.EnvironmentPaletteTwilight} {
				document := lightingWorld()
				document.Environment = &world.Environment{Version: 1, Preset: preset, Palette: palette}
				document.Sectors[1].Sky = true
				document.Sectors[1].Walls[0].Sky = true
				encoded, err := world.Encode(document)
				if err != nil {
					t.Fatal(err)
				}
				decoded, err := world.Decode(encoded)
				if err != nil || !reflect.DeepEqual(document, decoded) {
					t.Fatalf("canonical roundtrip: %v", err)
				}
				resolved, err := world.ResolveEnvironment(decoded.Environment)
				if err != nil || resolved.SunDirection != base.SunDirection || resolved.SunColor != base.SunColor ||
					resolved.SunIntensity != base.SunIntensity ||
					resolved.Ambient != base.Ambient ||
					resolved.AmbientCube != base.AmbientCube {
					t.Fatalf("palette changed illumination: %+v err=%v", resolved, err)
				}
				if palette == world.EnvironmentPaletteDefault && resolved != base {
					t.Fatal("explicit default palette differs from omitted palette")
				}
			}
		})
	}
}

func TestEnvironmentDirectionAndZeroAllocationResolution(t *testing.T) {
	for _, test := range []struct {
		sun  world.EnvironmentSun
		want world.Vec3
	}{
		{world.EnvironmentSun{}, world.Vec3{X: 1}},
		{world.EnvironmentSun{Azimuth: 90}, world.Vec3{Y: 1}},
		{world.EnvironmentSun{Azimuth: 180}, world.Vec3{X: -1}},
		{world.EnvironmentSun{Azimuth: 270}, world.Vec3{Y: -1}},
		{world.EnvironmentSun{Elevation: 90}, world.Vec3{Z: 1}},
	} {
		environment := &world.Environment{Version: 1, Preset: world.EnvironmentDaylight, Sun: &test.sun}
		resolved, err := world.ResolveEnvironment(environment)
		if err != nil || math.Abs(resolved.SunDirection.X-test.want.X) > 1e-12 || math.Abs(resolved.SunDirection.Y-test.want.Y) > 1e-12 ||
			math.Abs(resolved.SunDirection.Z-test.want.Z) > 1e-12 {
			t.Fatalf("sun %+v direction %+v err=%v", test.sun, resolved.SunDirection, err)
		}
	}
	environment := &world.Environment{Version: 1, Preset: world.EnvironmentFantasy}
	if allocations := testing.AllocsPerRun(100, func() {
		if _, err := world.ResolveEnvironment(environment); err != nil {
			t.Fatal(err)
		}
	}); allocations != 0 {
		t.Fatalf("preset resolution allocated %g times", allocations)
	}
}

func TestEnvironmentVersionBoundsAndOpaqueSkyBoundary(t *testing.T) {
	t.Parallel()
	if world.EnvironmentVersion != 1 || world.Version != 3 || world.FeatureEnvironment != "world/environment@1" {
		t.Fatal("environment version contract changed")
	}
	if err := world.ValidateEnvironment(nil); !errors.Is(err, world.ErrEnvironment) {
		t.Fatal(err)
	}
	for name, test := range map[string]struct {
		mutate func(*world.Document)
		want   error
	}{
		"missing payload version":       {func(d *world.Document) { d.Environment.Version = 0 }, world.ErrVersion},
		"future payload version":        {func(d *world.Document) { d.Environment.Version = world.ExplicitEnvironmentVersion + 1 }, world.ErrVersion},
		"unknown preset":                {func(d *world.Document) { d.Environment.Preset = "ocean" }, world.ErrEnvironment},
		"missing preset":                {func(d *world.Document) { d.Environment.Preset = "" }, world.ErrEnvironment},
		"unknown palette":               {func(d *world.Document) { d.Environment.Palette = "pink" }, world.ErrEnvironment},
		"legacy world":                  {func(d *world.Document) { d.Version = 1 }, world.ErrVersion},
		"actor world":                   {func(d *world.Document) { d.Version = 2 }, world.ErrVersion},
		"sky without environment":       {func(d *world.Document) { d.Environment = nil; d.Sectors[0].Sky = true }, world.ErrEnvironment},
		"sky boundary in indoor sector": {func(d *world.Document) { d.Sectors[0].Walls[0].Sky = true }, world.ErrEnvironment},
		"sky boundary portal":           {func(d *world.Document) { d.Sectors[0].Sky = true; d.Sectors[0].Walls[1].Sky = true }, world.ErrEnvironment},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			document := lightingWorld()
			document.Environment = &world.Environment{Version: 1, Preset: world.EnvironmentDaylight}
			test.mutate(&document)
			if _, err := world.Encode(document); !errors.Is(err, test.want) {
				t.Fatalf("Encode error=%v want=%v", err, test.want)
			}
		})
	}
	for _, field := range []string{"azimuth", "elevation"} {
		bad := []float64{-1, math.NaN(), math.Inf(1), math.Inf(-1)}
		if field == "azimuth" {
			bad = append(bad, 360)
		} else {
			bad = append(bad, math.Nextafter(90, 100))
		}
		for _, value := range bad {
			environment := &world.Environment{
				Version: 1,
				Preset:  world.EnvironmentDaylight,
				Sun:     &world.EnvironmentSun{Azimuth: 135, Elevation: 40},
			}
			if field == "azimuth" {
				environment.Sun.Azimuth = value
			} else {
				environment.Sun.Elevation = value
			}
			if _, err := world.ResolveEnvironment(environment); !errors.Is(err, world.ErrEnvironment) {
				t.Fatalf("accepted %s=%v: %v", field, value, err)
			}
		}
	}
}

func TestEnvironmentStrictDecodeAndAbsentLegacyEncoding(t *testing.T) {
	t.Parallel()
	d := lightingWorld()
	d.Environment = &world.Environment{
		Version: 1,
		Preset:  world.EnvironmentCartoon,
		Sun:     &world.EnvironmentSun{Azimuth: 135, Elevation: 40},
	}
	d.Sectors[0].Sky = true
	encoded, err := world.Encode(d)
	if err != nil {
		t.Fatal(err)
	}
	assertBinaryMalformed(t, encoded)
	for version := uint16(1); version <= world.Version; version++ {
		legacy := validWorld()
		if version == world.Version {
			legacy = lightingWorld()
		}
		legacy.Version = version
		encoded, err := world.Encode(legacy)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := world.Decode(encoded)
		if err != nil || decoded.Environment != nil || !reflect.DeepEqual(legacy, decoded) {
			t.Fatalf("v%d roundtrip: %v", version, err)
		}
	}
}
