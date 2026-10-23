package worldlightmap

import (
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestPrebakeArtifactRetainsResourceIdentityAndDefersSourceProof(t *testing.T) {
	document, layout, pair := prebakeFixture(t)
	for _, offline := range []bool{false, true} {
		candidate := pair
		if offline {
			var err error
			rangeBound, rangeErr := OfflineRGBMRange(layout, &document, 0)
			if rangeErr != nil {
				t.Fatal(rangeErr)
			}
			candidate, err = NewOfflinePrebake(
				layout,
				&document,
				pair.Image,
				OfflineBakeInputs{Samples: 1, ReflectanceSHA256: strings.Repeat("a", 64), RGBMRange: rangeBound},
			)
			if err != nil {
				t.Fatal(err)
			}
		}
		encoded, err := EncodePrebake(candidate, layout, &document)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := DecodePrebakeArtifact(encoded, candidate.Image, layout)
		if err != nil || !reflect.DeepEqual(decoded, candidate) {
			t.Fatal("compiled prebake changed", err)
		}
		// Provenance is intentionally not reconstructed from authored lights/rays.
		candidate.Manifest.BakeSHA256 = strings.Repeat("f", 64)
		if err := ValidatePrebakeArtifact(candidate, layout); err != nil {
			t.Fatal("runtime repeated bake source proof", err)
		}
		if err := candidate.Validate(layout, &document); err == nil {
			t.Fatal("offline bake source proof weakened")
		}
		for _, bad := range [][]byte{nil, encoded[:len(encoded)-1], append(slices.Clone(encoded), 0)} {
			if _, err := DecodePrebakeArtifact(bad, pair.Image, layout); err == nil {
				t.Fatal("malformed prebake manifest accepted")
			}
		}
	}
}

func TestPrebakeArtifactRejectsImageLayoutAndNumericBounds(t *testing.T) {
	_, layout, original := prebakeFixture(t)
	for name, change := range map[string]func(*PrebakePair){
		"schema":           func(p *PrebakePair) { p.Manifest.Schema = "other" },
		"encoding":         func(p *PrebakePair) { p.Manifest.Encoding = PointVisibilityEncoding },
		"layout identity":  func(p *PrebakePair) { p.Manifest.LayoutSHA256 = strings.Repeat("f", 64) },
		"image digest":     func(p *PrebakePair) { p.Manifest.ImageSHA256 = strings.Repeat("f", 64) },
		"image size":       func(p *PrebakePair) { p.Manifest.ImageBytes++ },
		"image dimensions": func(p *PrebakePair) { p.Manifest.Width++ },
		"image truncated": func(p *PrebakePair) {
			p.Image = p.Image[:len(p.Image)-1]
			p.Manifest.ImageBytes = len(p.Image)
			p.Manifest.ImageSHA256 = digest(p.Image)
		},
		"image colorspace": func(p *PrebakePair) {
			p.Image = slices.Clone(p.Image)
			p.Image[13] = 0
			p.Manifest.ImageSHA256 = digest(p.Image)
		},
		"range zero":         func(p *PrebakePair) { p.Manifest.RGBMRange = 0 },
		"range nan":          func(p *PrebakePair) { p.Manifest.RGBMRange = math.NaN() },
		"range gpu overflow": func(p *PrebakePair) { p.Manifest.RGBMRange = math.MaxFloat64 },
	} {
		t.Run(name, func(t *testing.T) {
			pair := original
			change(&pair)
			if ValidatePrebakeArtifact(pair, layout) == nil {
				t.Fatal("unsafe compiled resource accepted")
			}
		})
	}
	if ValidatePrebakeArtifact(original, Layout{}) == nil {
		t.Fatal("missing layout accepted")
	}
	changed := layout
	changed.RuntimeBake = nil
	if ValidatePrebakeArtifact(original, changed) == nil {
		t.Fatal("missing direct RNM recipe accepted")
	}
}
