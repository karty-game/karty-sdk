package worldlightmap

import (
	"fmt"
	"slices"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
)

func directFixture(t testing.TB) world.Document {
	t.Helper()
	document := fixture(t)
	document.Version = world.Version
	document.Sectors[0].Walls[1].PortalWall = 4
	document.Sectors[1].Walls[3].PortalWall = 2
	document.Lighting = &world.Lighting{Version: 1, Lights: []world.PointLight{
		{ID: "red", Position: world.Vec3{X: 2, Y: 2, Z: 2}, Color: world.Vec3{X: 1}, Radius: 10},
		{ID: "blue", Position: world.Vec3{X: 6, Y: 2, Z: 2}, Color: world.Vec3{Z: 1}, Radius: 10},
	}}
	return document
}

func TestDirectRecipeOrderedSelectionAndOwnership(t *testing.T) {
	document := directFixture(t)
	selected := []string{"blue", "red"}
	layout, err := Compile(document, Options{Lights: selected, ShadowSize: 128})
	if err != nil {
		t.Fatal(err)
	}
	selected[0] = "caller-mutated"
	recipe := layout.RuntimeBake
	if recipe == nil || recipe.Encoding != DirectRNMEncoding || recipe.LightID != "" ||
		!slices.Equal(recipe.LightIDs, []string{"blue", "red"}) || recipe.ShadowSize != 128 {
		t.Fatalf("ordered independently owned recipe: %+v", recipe)
	}
	encoded, err := Encode(layout, &document)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(encoded, &document)
	if err != nil || !slices.Equal(decoded.RuntimeBake.LightIDs, recipe.LightIDs) {
		t.Fatalf("round trip: %v", err)
	}
	first, err := Compile(document, Options{Light: "red", ShadowSize: 128})
	if err != nil {
		t.Fatal(err)
	}
	old, err := Encode(first, &document)
	decodedFirst, decodeErr := Decode(old, &document)
	if err != nil || decodeErr != nil || decodedFirst.RuntimeBake.Encoding != PointVisibilityEncoding ||
		decodedFirst.RuntimeBake.LightID != "red" {
		t.Fatalf("existing visibility recipe changed: %v", err)
	}
}

func TestDirectRecipeRejectsWholeMalformedSelection(t *testing.T) {
	for name, options := range map[string]Options{
		"empty":            {Lights: []string{}},
		"both":             {Light: "red", Lights: []string{"blue"}},
		"last unknown":     {Lights: []string{"red", "missing"}},
		"last duplicate":   {Lights: []string{"red", "blue", "red"}},
		"empty identifier": {Lights: []string{"red", ""}},
		"too many":         {Lights: make([]string, MaxBakeLights+1)},
		"auto and single":  {AllStatic: true, Light: "red"},
		"auto and list":    {AllStatic: true, Lights: []string{"red"}},
		"shadow below":     {Lights: []string{"red"}, ShadowSize: 31},
		"shadow above":     {Lights: []string{"red"}, ShadowSize: 513},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Compile(directFixture(t), options); err == nil {
				t.Fatal("malformed selection accepted")
			}
		})
	}
	document := directFixture(t)
	document.Lighting.Lights[1].Position.Z = 100
	if _, err := Compile(document, Options{Lights: []string{"red"}}); err != nil {
		t.Fatalf("unselected outside emitter rejected: %v", err)
	}
	if _, err := Compile(document, Options{Lights: []string{"red", "blue"}}); err == nil {
		t.Fatal("outside final selected emitter accepted")
	}
}

func TestDirectRecipeCanonicalFields(t *testing.T) {
	document := directFixture(t)
	layout, err := Compile(document, Options{Lights: []string{"red", "blue"}})
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*RuntimeBake){
		"nil selection": func(r *RuntimeBake) { r.LightIDs = nil },
		"unknown":       func(r *RuntimeBake) { r.LightIDs = []string{"red", "missing"} }, "duplicate": func(r *RuntimeBake) { r.LightIDs = []string{"red", "red"} },
		"both selectors": func(r *RuntimeBake) { r.LightID = "red" }, "encoding": func(r *RuntimeBake) { r.Encoding = "direct-rnm3@2" }, "shadow": func(r *RuntimeBake) { r.ShadowSize = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			bad := layout
			recipe := *layout.RuntimeBake
			bad.RuntimeBake = &recipe
			mutate(&recipe)
			encoded, err := encodeLayoutBinary(bad)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Decode(encoded, &document); err == nil {
				t.Fatal("malformed recipe accepted")
			}
		})
	}
	point, err := Compile(document, Options{Light: "red"})
	if err != nil {
		t.Fatal(err)
	}
	for _, ids := range [][]string{{}, {"blue"}} {
		point.RuntimeBake.LightIDs = ids
		encoded, err := encodeLayoutBinary(point)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Decode(encoded, &document); err == nil {
			t.Fatal("point recipe accepted directional selectors")
		}
	}
}

func TestDirectRecipeMaximumSelection(t *testing.T) {
	document := directFixture(t)
	ids := []string{"red", "blue"}
	for index := len(ids); index < MaxBakeLights; index++ {
		id := fmt.Sprintf("lamp-%d", index)
		ids = append(ids, id)
		light := document.Lighting.Lights[0]
		light.ID = id
		document.Lighting.Lights = append(document.Lighting.Lights, light)
	}
	layout, err := Compile(document, Options{Lights: ids, ShadowSize: 32})
	if err != nil || !slices.Equal(layout.RuntimeBake.LightIDs, ids) {
		t.Fatalf("maximum bounded selection rejected: %v", err)
	}
}
