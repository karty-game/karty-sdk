package worldlightmap

import (
	"bytes"
	"strings"
	"testing"
)

func TestOfflinePrebakeVersionIdentityAndDirectCompatibility(t *testing.T) {
	document, layout, direct := prebakeFixture(t)
	rangeBound, err := OfflineRGBMRange(layout, &document, 1)
	if err != nil || rangeBound != 11.7 {
		t.Fatalf("one-bounce range: %v %v", rangeBound, err)
	}
	inputs := OfflineBakeInputs{Samples: 16, Bounces: 1, Seed: 1, ReflectanceSHA256: strings.Repeat("a", 64), RGBMRange: rangeBound}
	pair, err := NewOfflinePrebake(layout, &document, direct.Image, inputs)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := EncodePrebake(pair, layout, &document)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodePrebake(encoded, pair.Image, layout, &document)
	if err != nil || decoded.Manifest != pair.Manifest || pair.Manifest.Algorithm != 2 {
		t.Fatalf("offline round trip: %v", err)
	}
	directEncoded, err := EncodePrebake(direct, layout, &document)
	if err != nil || bytes.Contains(directEncoded, []byte("reflectance")) || bytes.Contains(directEncoded, []byte("producer")) {
		t.Fatal("direct manifest gained optional offline fields")
	}
	// Geometry-only layouts do not identify albedo/mapping. Offline transport
	// separately binds material identity even when the receiver charts are valid.
	document.Sectors[0].FloorMaterial++
	if err := Validate(&layout, &document); err != nil {
		t.Fatalf("material edit changed geometry-only layout: %v", err)
	}
	if err := pair.Validate(layout, &document); err == nil {
		t.Fatal("offline material edit accepted with stale atlas")
	}
	if err := direct.Validate(layout, &document); err != nil {
		t.Fatalf("material-independent direct bake invalidated: %v", err)
	}
}

func TestOfflinePrebakeRejectsUnknownProducerAndTamperedInputs(t *testing.T) {
	document, layout, direct := prebakeFixture(t)
	rangeBound, err := OfflineRGBMRange(layout, &document, 1)
	if err != nil {
		t.Fatal(err)
	}
	pair, err := NewOfflinePrebake(layout, &document, direct.Image, OfflineBakeInputs{
		Samples: 16, Bounces: 1, Seed: 1, ReflectanceSHA256: strings.Repeat("a", 64), RGBMRange: rangeBound,
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*PrebakeManifest){
		"producer":          func(m *PrebakeManifest) { m.Producer = "unknown" },
		"samples":           func(m *PrebakeManifest) { m.Samples++ },
		"bounces":           func(m *PrebakeManifest) { m.Bounces++ },
		"seed":              func(m *PrebakeManifest) { m.Seed++ },
		"surface":           func(m *PrebakeManifest) { m.SurfaceSHA256 = strings.Repeat("b", 64) },
		"reflectance":       func(m *PrebakeManifest) { m.ReflectanceSHA256 = strings.Repeat("b", 64) },
		"uppercase":         func(m *PrebakeManifest) { m.ReflectanceSHA256 = strings.Repeat("A", 64) },
		"range":             func(m *PrebakeManifest) { m.RGBMRange++ },
		"excessive samples": func(m *PrebakeManifest) { m.Samples = 257 },
	} {
		t.Run(name, func(t *testing.T) {
			changed := pair
			mutate(&changed.Manifest)
			if err := changed.Validate(layout, &document); err == nil {
				t.Fatal("invalid offline manifest accepted")
			}
		})
	}
}
