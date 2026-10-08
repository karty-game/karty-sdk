package worldlightmap

import (
	"reflect"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
)

func TestVisualAnimationsPreserveStaticBakeLayout(t *testing.T) {
	d := directFixture(t)
	before, err := Compile(d, Options{Lights: []string{"red"}})
	if err != nil {
		t.Fatal(err)
	}
	beforeDigest, err := OfflineSurfaceDigest(&d)
	if err != nil {
		t.Fatal(err)
	}
	material := d.Sectors[0].FloorMaterial
	d.Animations = &world.Animations{
		Version: 1,
		Presets: []world.AnimationPreset{
			{Name: "liquid", Kind: "liquid", Flow: world.Vec2{X: .02}, Amplitude: .03, Frequency: 2, Speed: .4},
		},
		Materials: []world.MaterialAnimation{{Material: material, Preset: "liquid"}},
	}
	after, err := Compile(d, Options{Lights: []string{"red"}})
	if err != nil {
		t.Fatal(err)
	}
	afterDigest, err := OfflineSurfaceDigest(&d)
	if err != nil {
		t.Fatal(err)
	}
	if beforeDigest != afterDigest {
		t.Fatal("visual animation changed static offline surface digest")
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("visual surface animation changed static bake geometry or recipe")
	}
	d.Animations.Presets[0].Speed = 2
	again, err := Compile(d, Options{Lights: []string{"red"}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, again) {
		t.Fatal("animation timing changed static bake recipe")
	}
}
