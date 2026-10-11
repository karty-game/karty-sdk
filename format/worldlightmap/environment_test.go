package worldlightmap

import (
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
)

func TestEnvironmentSkyReceiverBindingsAndCeilingSeams(t *testing.T) {
	t.Parallel()
	document := directFixture(t)
	document.Environment = &world.Environment{Version: 1, Preset: world.EnvironmentDaylight}
	for i := range document.Sectors {
		document.Sectors[i].Sky = true
		document.Sectors[i].Floor = world.Plane{}
		document.Sectors[i].Ceiling = world.Plane{C: float64(4 - i*2)}
	}
	document.Sectors[0].Walls[0].Sky = true
	surfaces, err := Surfaces(&document)
	if err != nil {
		t.Fatal(err)
	}
	if len(surfaces) != 12 {
		t.Fatalf("semantic binding count changed: %d", len(surfaces))
	}
	for _, surface := range surfaces {
		b := surface.Binding
		wantEmpty := b.Kind == "sector-ceiling" || b.Kind == "sector-wall" &&
			(b.Index == 0 && (b.Edge == 0 || b.Edge == 1) || b.Index == 1 && b.Edge == 3)
		if (len(surface.Polygons) == 0) != wantEmpty {
			t.Fatalf("binding %+v polygons=%d wantEmpty=%t", b, len(surface.Polygons), wantEmpty)
		}
	}
	layout, err := Compile(document, Options{Lights: []string{"red"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(layout.Bindings) != len(surfaces) {
		t.Fatal("empty sky bindings were removed")
	}
	for i, surface := range surfaces {
		if (len(surface.Polygons) == 0) != (layout.Bindings[i].Chart == -1) {
			t.Fatalf("binding %+v chart=%d", surface.Binding, layout.Bindings[i].Chart)
		}
	}
	encoded, err := Encode(layout, &document)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(encoded, &document); err != nil {
		t.Fatal(err)
	}
	// A sky-to-indoor lintel stays physical and receives a chart.
	document.Sectors[1].Sky = false
	surfaces, err = Surfaces(&document)
	if err != nil {
		t.Fatal(err)
	}
	if len(surfaces[3].Polygons) == 0 || len(surfaces[7].Polygons) == 0 {
		t.Fatal("indoor roof or exterior lintel disappeared")
	}
}

func TestEnvironmentGeometryDigestTracksSkyButNotPalette(t *testing.T) {
	t.Parallel()
	document := directFixture(t)
	legacy, err := GeometryDigest(&document)
	if err != nil {
		t.Fatal(err)
	}
	document.Environment = &world.Environment{Version: 1, Preset: world.EnvironmentSpace}
	unopened, err := GeometryDigest(&document)
	if err != nil || legacy != unopened {
		t.Fatalf("environment appearance changed receiver geometry digest: %v", err)
	}
	document.Sectors[0].Sky = true
	opened, err := GeometryDigest(&document)
	if err != nil || opened == legacy {
		t.Fatalf("sky ceiling missing from geometry identity: %v", err)
	}
	document.Sectors[0].Walls[0].Sky = true
	boundary, err := GeometryDigest(&document)
	if err != nil || boundary == opened {
		t.Fatalf("sky boundary missing from geometry identity: %v", err)
	}
	document.Environment.Palette = world.EnvironmentPaletteViolet
	palette, err := GeometryDigest(&document)
	if err != nil || palette != boundary {
		t.Fatalf("sky palette changed receiver geometry digest: %v", err)
	}
	// Recipes cannot silently select the directional sun as a point light.
	if _, err := Compile(document, Options{Lights: []string{"sun"}}); err == nil {
		t.Fatal("unsupported sun bake accepted")
	}
}
