package worldlightmap

import (
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
)

func TestRuntimeMotionCannotBeBaked(t *testing.T) {
	document := directFixture(t)
	document.Lighting.Lights[1].Motion = &world.LightMotion{Version: 1, Offset: world.Vec3{X: 1}, PeriodSeconds: 12}
	if _, err := Compile(document, Options{Lights: []string{"red"}}); err != nil {
		t.Fatal(err)
	}
	for _, options := range []Options{{Light: "blue"}, {Lights: []string{"red", "blue"}}} {
		if _, err := Compile(document, options); err == nil {
			t.Fatal("moving light accepted in static bake")
		}
	}
}
