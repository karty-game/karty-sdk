package worldlightmap

import (
	"reflect"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
)

func TestLiquidRuntimeSettingsPreserveBakeIdentity(t *testing.T) {
	d := directFixture(t)
	material := d.Sectors[0].FloorMaterial
	d.StaticSolids = &world.StaticSolids{
		Version: 1,
		Items: []world.Solid{
			{
				ID:             "pool",
				Footprint:      []world.Vec2{{X: .5, Y: .5}, {X: 1.5, Y: .5}, {X: 1.5, Y: 1.5}, {X: .5, Y: 1.5}},
				Top:            world.Plane{C: 1},
				SideMaterial:   material,
				TopMaterial:    material,
				BottomMaterial: material,
			},
		},
	}
	d.Animations = &world.Animations{
		Version:   1,
		Presets:   []world.AnimationPreset{{Name: "liquid", Kind: "liquid", Frequency: 1}},
		Materials: []world.MaterialAnimation{{Material: material, Preset: "liquid"}},
	}
	before, err := Compile(d, Options{Lights: []string{"red"}})
	if err != nil {
		t.Fatal(err)
	}
	digest, err := OfflineSurfaceDigest(&d)
	if err != nil {
		t.Fatal(err)
	}
	env, err := EnvironmentSurfaceDigest(&d)
	if err != nil {
		t.Fatal(err)
	}
	d.Fluids = &world.Fluids{Version: 1, Items: []world.Fluid{{ID: "water", Solids: []string{"pool"}, Density: 45}}}
	after, err := Compile(d, Options{Lights: []string{"red"}})
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("fluids changed receiver recipe", err)
	}
	actual, err := OfflineSurfaceDigest(&d)
	if err != nil || actual != digest {
		t.Fatal("fluids invalidated static bake", err)
	}
	actual, err = EnvironmentSurfaceDigest(&d)
	if err != nil || actual != env {
		t.Fatal("fluids invalidated environment bake", err)
	}
	d.Fluids.Items[0].Solids = append(d.Fluids.Items[0].Solids, "missing")
	if _, err := OfflineSurfaceDigest(&d); err == nil {
		t.Fatal("digest ignored invalid fluid metadata")
	}
}
