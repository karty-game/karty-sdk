package worldlightmap

import (
	"bytes"
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
	if !bytes.Contains(encoded, []byte(`"runtime_bake":{"encoding":"direct-rnm3@1","light_ids":["blue","red"],"shadow_size":128}`)) {
		t.Fatalf("unexpected recipe encoding: %s", encoded)
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
	if err != nil || !bytes.Contains(old, []byte(`"runtime_bake":{"encoding":"point-visibility@1","light_id":"red","shadow_size":128}`)) {
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
		"too many":         {Lights: []string{"red", "blue", "a", "b", "c", "d", "e", "f", "g"}},
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
	encoded, err := Encode(layout, &document)
	if err != nil {
		t.Fatal(err)
	}
	const ids = `"light_ids":["red","blue"]`
	for name, replacement := range map[string]string{
		"null": `"light_ids":null`, "empty": `"light_ids":[]`,
		"unknown":            `"light_ids":["red","missing"]`,
		"duplicate value":    `"light_ids":["red","red"]`,
		"duplicate field":    ids + `,` + ids,
		"both selectors":     ids + `,"light_id":"red"`,
		"empty old selector": ids + `,"light_id":""`,
		"null old selector":  ids + `,"light_id":null`,
		"unknown field":      ids + `,"range":8`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode(bytes.Replace(encoded, []byte(ids), []byte(replacement), 1), &document); err == nil {
				t.Fatal("noncanonical or malformed recipe accepted")
			}
		})
	}
	if _, err := Decode(bytes.Replace(encoded, []byte(ids+`,`), nil, 1), &document); err == nil {
		t.Fatal("missing light selection accepted")
	}
	for name, mutation := range map[string][2]string{
		"missing encoding": {`"encoding":"direct-rnm3@1",`, ``},
		"null encoding":    {`"encoding":"direct-rnm3@1"`, `"encoding":null`},
		"unknown encoding": {`"direct-rnm3@1"`, `"direct-rnm3@2"`},
		"missing shadow":   {`,"shadow_size":512`, ``},
		"null shadow":      {`"shadow_size":512`, `"shadow_size":null`},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode(bytes.Replace(encoded, []byte(mutation[0]), []byte(mutation[1]), 1), &document); err == nil {
				t.Fatal("incomplete or unsupported directional recipe accepted")
			}
		})
	}
	point, err := Compile(document, Options{Light: "red"})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err = Encode(point, &document)
	if err != nil {
		t.Fatal(err)
	}
	for _, extra := range []string{`,"light_ids":null`, `,"light_ids":[]`, `,"light_ids":["blue"]`} {
		if _, err := Decode(
			bytes.Replace(encoded, []byte(`"light_id":"red"`), []byte(`"light_id":"red"`+extra), 1),
			&document,
		); err == nil {
			t.Fatal("visibility recipe accepted directional selector")
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
