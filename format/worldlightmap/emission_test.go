package worldlightmap

import (
	"reflect"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
)

func TestEmissionPreservesStaticBakeRecipeAndSurfaceDigest(t *testing.T) {
	t.Parallel()
	d := directFixture(t)
	before, err := Compile(d, Options{Lights: []string{"red"}})
	if err != nil {
		t.Fatal(err)
	}
	digest, err := OfflineSurfaceDigest(&d)
	if err != nil {
		t.Fatal(err)
	}
	d.Emission = &world.Emission{
		Version: 1,
		Materials: []world.MaterialEmission{
			{Material: d.Sectors[0].FloorMaterial, Intensity: 255, Pulse: &world.EmissionPulse{Depth: 1, PeriodSeconds: 3}},
		},
	}
	after, err := Compile(d, Options{Lights: []string{"red"}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := OfflineSurfaceDigest(&d)
	if err != nil || got != digest || !reflect.DeepEqual(before, after) {
		t.Fatal("visual emission changed static transport", err)
	}
}

func TestEmissionPulsingLightsCannotBeBaked(t *testing.T) {
	t.Parallel()
	d := directFixture(t)
	d.Emission = &world.Emission{
		Version: 1,
		Materials: []world.MaterialEmission{
			{
				Material:  d.Sectors[0].FloorMaterial,
				Intensity: 32,
				Pulse:     &world.EmissionPulse{Depth: .8, PeriodSeconds: 3.2},
				Lights:    []string{"blue"},
			},
		},
	}
	if _, err := Compile(d, Options{Lights: []string{"red"}}); err != nil {
		t.Fatal(err)
	}
	for _, options := range []Options{{Light: "blue"}, {Lights: []string{"red", "blue"}}} {
		if _, err := Compile(d, options); err == nil {
			t.Fatal("pulsing light accepted in static bake")
		}
	}
}
