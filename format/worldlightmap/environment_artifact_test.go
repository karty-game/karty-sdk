package worldlightmap

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestEnvironmentArtifactRoundTripAndSourceProofBoundary(t *testing.T) {
	t.Parallel()
	document, layout, catalog, image := environmentCatalogFixture(t)
	encoded, err := EncodeEnvironmentVariants(catalog, layout, &document)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeEnvironmentVariantsArtifact(encoded, layout)
	if err != nil || !reflect.DeepEqual(decoded, catalog) {
		t.Fatalf("artifact roundtrip differs: %v", err)
	}
	if err := ValidateEnvironmentImage(decoded.Variants[decoded.Default], image, layout); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range [][]byte{nil, encoded[:len(encoded)-1], append(slices.Clone(encoded), 0)} {
		if _, err := DecodeEnvironmentVariantsArtifact(invalid, layout); err == nil {
			t.Fatal("accepted malformed catalog")
		}
	}
	// Source provenance is approved by the builder's whole-file catalog hash;
	// the runtime catalog checker intentionally does not re-prove that geometry.
	catalog.SurfaceSHA256 = strings.Repeat("a", 64)
	if err := ValidateEnvironmentVariantsArtifact(&catalog, layout); err != nil {
		t.Fatal(err)
	}
	if err := ValidateEnvironmentVariants(&catalog, layout, &document); err == nil {
		t.Fatal("builder source oracle lost surface identity check")
	}
}

func TestEnvironmentArtifactRejectsUnsafeReferencesAndBounds(t *testing.T) {
	t.Parallel()
	_, layout, original, _ := environmentCatalogFixture(t)
	cases := map[string]func(*EnvironmentVariants){
		"default":       func(c *EnvironmentVariants) { c.Default = len(c.Variants) },
		"empty":         func(c *EnvironmentVariants) { c.Variants = nil },
		"file size":     func(c *EnvironmentVariants) { c.Variants[0].Size = 0 },
		"image size":    func(c *EnvironmentVariants) { c.Variants[0].ImageSize = c.Variants[0].Size },
		"decoded bytes": func(c *EnvironmentVariants) { c.Variants[0].DecodedBytes++ },
		"image hash":    func(c *EnvironmentVariants) { c.Variants[0].ImageSHA256 = "invalid" },
		"range":         func(c *EnvironmentVariants) { c.Variants[0].RGBMRange = 257 },
		"layout":        func(c *EnvironmentVariants) { c.LayoutSHA256 = strings.Repeat("0", 64) },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			catalog := original
			catalog.Variants = slices.Clone(original.Variants)
			change(&catalog)
			if err := ValidateEnvironmentVariantsArtifact(&catalog, layout); err == nil {
				t.Fatal("accepted invalid runtime artifact")
			}
		})
	}
	if err := ValidateEnvironmentVariantsArtifact(nil, layout); err == nil {
		t.Fatal("nil catalog accepted")
	}
	if err := ValidateEnvironmentVariantsArtifact(&original, Layout{}); err == nil {
		t.Fatal("missing layout accepted")
	}
}
