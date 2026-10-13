package worldlightmap

import (
	"bytes"
	"image"
	"slices"
	"strings"
	"testing"

	"github.com/karty-game/karty-sdk/codec/qoi"
	"github.com/karty-game/karty-sdk/format/world"
)

func environmentCatalogFixture(t testing.TB) (world.Document, Layout, EnvironmentVariants, []byte) {
	t.Helper()
	document := directFixture(t)
	document.Environment = &world.Environment{Version: 1, Preset: world.EnvironmentDaylight}
	layout, err := CompileReceivers(document, Options{PageSize: 512, TexelsPerUnit: 1, Padding: 2}, []int{0, 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	pixels := image.NewNRGBA(image.Rect(0, 0, 1536, 512))
	encoded, _, err := qoi.Encode(pixels, qoi.Options{Channels: qoi.ChannelsRGBA, Colorspace: qoi.ColorspaceLinear})
	if err != nil {
		t.Fatal(err)
	}
	layoutHash, _ := LayoutDigest(&layout)
	surfaceHash, _ := EnvironmentSurfaceDigest(&document)
	catalog := EnvironmentVariants{
		Schema:            EnvironmentVariantsSchema,
		GeometrySHA256:    layout.GeometrySHA256,
		LayoutSHA256:      layoutHash,
		SurfaceSHA256:     surfaceHash,
		ReflectanceSHA256: strings.Repeat("a", 64),
		Producer:          EnvironmentPrebakeProducer,
		Samples:           16,
		Bounces:           1,
		Seed:              1,
		Denoise:           "medium",
		Default:           2,
	}
	for _, clock := range []float64{0, 15} {
		for _, cloud := range []float64{0, 1} {
			rangeValue, _ := EnvironmentRGBMRange(document.Environment, world.EnvironmentSettings{TimeOfDay: clock, CloudAmount: cloud}, 1)
			catalog.Variants = append(catalog.Variants, EnvironmentVariant{
				TimeOfDay:   clock,
				CloudAmount: cloud,
				RGBMRange:   rangeValue,
				ContentSHA256: strings.Repeat(
					"b",
					64,
				),
				ImageSHA256:  digest(encoded),
				Size:         uint64(len(encoded) + 256),
				ImageSize:    uint64(len(encoded)),
				DecodedBytes: 512 * 512 * 12,
			})
		}
	}
	return document, layout, catalog, encoded
}

func TestEnvironmentCatalogRoundTripImageValidationAndSelection(t *testing.T) {
	t.Parallel()
	document, layout, catalog, image := environmentCatalogFixture(t)
	encoded, err := EncodeEnvironmentVariants(catalog, layout, &document)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := DecodeEnvironmentVariants(encoded, layout, &document)
	if err != nil || actual.Default != 2 || !slices.Equal(actual.Variants, catalog.Variants) {
		t.Fatalf("catalog roundtrip: %v", err)
	}
	if err := ValidateEnvironmentImage(actual.Variants[0], image, layout); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		time, cloud float64
		clear       int
	}{{15, .35, 2}, {23, .7, 0}, {8, 0, 2}, {3, 1, 0}} {
		clearIndex, overcast, blend, err := SelectEnvironmentVariants(
			&actual,
			world.EnvironmentSettings{TimeOfDay: test.time, CloudAmount: test.cloud},
		)
		if err != nil || clearIndex != test.clear || overcast != clearIndex+1 || blend != test.cloud {
			t.Fatalf("selection clock%g: %d/%d/%g %v", test.time, clearIndex, overcast, blend, err)
		}
	}
	corrupt := bytes.Clone(image)
	corrupt[len(corrupt)/2] ^= 1
	if ValidateEnvironmentImage(actual.Variants[0], corrupt, layout) == nil {
		t.Fatal("corrupt image accepted")
	}
}

func TestTimeOnlyEnvironmentCatalogKeepsFourStatesAcrossCloudChanges(t *testing.T) {
	t.Parallel()
	document, layout, catalog, _ := environmentCatalogFixture(t)
	base := catalog.Variants[0]
	catalog.Variants = nil
	catalog.Default = 2
	for _, clock := range []float64{0, 9, 12, 17} {
		variant := base
		variant.TimeOfDay = clock
		variant.RGBMRange, _ = EnvironmentRGBMRange(document.Environment, world.EnvironmentSettings{TimeOfDay: clock}, catalog.Bounces)
		catalog.Variants = append(catalog.Variants, variant)
	}
	encoded, err := EncodeEnvironmentVariants(catalog, layout, &document)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := DecodeEnvironmentVariants(encoded, layout, &document)
	if err != nil || actual.Default != 2 || !slices.Equal(actual.Variants, catalog.Variants) {
		t.Fatalf("time-only catalog roundtrip: %v", err)
	}
	for _, test := range []struct {
		time  float64
		index int
	}{{0, 0}, {9, 1}, {12, 2}, {17, 3}, {23, 0}, {10.5, 1}} {
		for _, cloud := range []float64{0, .35, .7, 1} {
			first, second, blend, err := SelectEnvironmentVariants(
				&actual,
				world.EnvironmentSettings{TimeOfDay: test.time, CloudAmount: cloud},
			)
			if err != nil || first != test.index || second != first || blend != 0 {
				t.Fatalf(
					"cloud changed time-only map selection: clock=%g cloud=%g result=%d/%d/%g err=%v",
					test.time, cloud, first, second, blend, err,
				)
			}
		}
	}
	for _, length := range []int{1, 3} {
		subset := catalog
		subset.Variants, subset.Default = catalog.Variants[:length], 0
		if _, err := EncodeEnvironmentVariants(subset, layout, &document); err != nil {
			t.Fatalf("valid unpaired record count%d rejected: %v", length, err)
		}
	}
	for _, mutate := range []func(*EnvironmentVariants){
		func(c *EnvironmentVariants) { c.Variants[1].TimeOfDay = c.Variants[0].TimeOfDay },
		func(c *EnvironmentVariants) { c.Variants[2].CloudAmount = 1 },
		func(c *EnvironmentVariants) { c.Variants[2].CloudAmount = .35 },
	} {
		invalid := catalog
		invalid.Variants = slices.Clone(catalog.Variants)
		mutate(&invalid)
		if _, err := EncodeEnvironmentVariants(invalid, layout, &document); err == nil {
			t.Fatal("duplicate or mixed time-only/weather records accepted")
		}
		if _, _, _, err := SelectEnvironmentVariants(&invalid, world.EnvironmentSettings{TimeOfDay: 12}); err == nil {
			t.Fatal("malformed records accepted by selector")
		}
	}
}

func TestEnvironmentCatalogRejectsStaleAndMalformedInputs(t *testing.T) {
	t.Parallel()
	document, layout, catalog, _ := environmentCatalogFixture(t)
	for _, mutate := range []func(*EnvironmentVariants){
		func(c *EnvironmentVariants) { c.Default = len(c.Variants) }, func(c *EnvironmentVariants) { c.Schema = "future" },
		func(c *EnvironmentVariants) { c.Producer = "unknown" }, func(c *EnvironmentVariants) { c.Variants = c.Variants[:3] },
		func(c *EnvironmentVariants) { c.Variants[1].TimeOfDay = 9 }, func(c *EnvironmentVariants) { c.Variants[0].CloudAmount = .5 },
		func(c *EnvironmentVariants) { c.Variants[0].DecodedBytes++ }, func(c *EnvironmentVariants) { c.Variants[0].RGBMRange++ },
		func(c *EnvironmentVariants) { c.Variants[0].Size = 0 }, func(c *EnvironmentVariants) { c.Variants[0].ImageSHA256 = "invalid" },
		func(c *EnvironmentVariants) { c.LayoutSHA256 = strings.Repeat("c", 64) },
	} {
		mutated := catalog
		mutated.Variants = slices.Clone(catalog.Variants)
		mutate(&mutated)
		if _, err := EncodeEnvironmentVariants(mutated, layout, &document); err == nil {
			t.Fatal("invalid catalog accepted")
		}
	}
	encoded, _ := EncodeEnvironmentVariants(catalog, layout, &document)
	for _, malformed := range [][]byte{append(bytes.Clone(encoded), 0), changeByte(encoded, 4, 2), changeByte(encoded, 6, 1), encoded[:16]} {
		if _, err := DecodeEnvironmentVariants(malformed, layout, &document); err == nil {
			t.Fatal("malformed binary accepted")
		}
	}
	document.Animations = &world.Animations{
		Version:   1,
		Presets:   []world.AnimationPreset{{Name: "glass", Kind: "liquid", Frequency: 1, Opacity: new(float64(.16))}},
		Materials: []world.MaterialAnimation{{Material: 1, Preset: "glass"}},
	}
	if _, err := DecodeEnvironmentVariants(encoded, layout, &document); err == nil {
		t.Fatal("changed surface opacity accepted stale variants")
	}
}

func TestEnvironmentSurfaceIdentityIgnoresActorOnlyAnimations(t *testing.T) {
	t.Parallel()
	document, layout, catalog, _ := environmentCatalogFixture(t)
	encoded, err := EncodeEnvironmentVariants(catalog, layout, &document)
	if err != nil {
		t.Fatal(err)
	}
	document.Contents = []world.Content{{ID: "animated-actor", SourceID: "animated-actor", Kind: "actor", Sector: 0,
		Position: world.Vec3{X: 1, Y: 1, Z: 1}, Actor: &world.Actor{Scale: world.Vec3{X: 1, Y: 1, Z: 1},
			Sprite: &world.Sprite{AssetID: 90, Facing: world.SpriteFixed, Alpha: world.SpriteCutout, Width: 1, Height: 1}}}}
	document.Animations = &world.Animations{
		Version:   1,
		Presets:   []world.AnimationPreset{{Name: "actor-flipbook", Kind: "flipbook", Frames: []uint32{90, 91}, FPS: 10}},
		Materials: []world.MaterialAnimation{{Material: 90, Preset: "actor-flipbook"}},
	}
	if err := world.Validate(&document); err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeEnvironmentVariants(encoded, layout, &document); err != nil {
		t.Fatalf("actor-only animation invalidated static variants: %v", err)
	}
	document.Animations.Presets[0].FPS = 20
	if _, err := DecodeEnvironmentVariants(encoded, layout, &document); err != nil {
		t.Fatalf("actor clock invalidated static variants: %v", err)
	}
}
