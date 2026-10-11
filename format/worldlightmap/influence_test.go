package worldlightmap

import (
	"bytes"
	"encoding/binary"
	"math"
	"slices"
	"strings"
	"testing"
)

func TestBoundedScopeSeparatesPixelSourcesFromCompletedSelection(t *testing.T) {
	d := directFixture(t)
	l, err := CompileReceivers(d, Options{PageSize: 512, AllStatic: true}, []int{0}, nil)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := BoundedInfluenceScope(&d, l, 0, 1)
	if err != nil || !slices.Equal(scope.LightIDs, []string{"red"}) || !slices.Equal(l.RuntimeBake.LightIDs, []string{"red", "blue"}) {
		t.Fatalf("local source scope modified completed recipe: %+v %v", scope, err)
	}
	if scope.Bounds[0] >= -1 || scope.Bounds[3] <= 5 {
		t.Fatal("direct shadow halo lacks distance/rounding guard")
	}
	localRange, err := OfflineRGBMRangeBounded(l, &d, 0, 1)
	if err != nil || math.Abs(localRange-3) > 1e-12 {
		t.Fatalf("distant source inflated local RGBM range: %g %v", localRange, err)
	}
	bounced, err := BoundedInfluenceScope(&d, l, 1, 1)
	if err != nil || !slices.Equal(bounced.LightIDs, []string{"red", "blue"}) || bounced.Bounds[3] <= 6 {
		t.Fatalf("bounce-reachable source or geometry was omitted: %+v %v", bounced, err)
	}
	unbounded, err := BoundedInfluenceScope(&d, l, 0, 0)
	if err != nil || !slices.Equal(unbounded.LightIDs, l.RuntimeBake.LightIDs) ||
		len(unbounded.SurfaceIndices) < len(scope.SurfaceIndices) {
		t.Fatalf("zero changed ordinary-component scope: %+v %v", unbounded, err)
	}
}

func TestBoundedPrebakeConditionalWireAndDistanceIdentity(t *testing.T) {
	d, l, direct := prebakeFixture(t)
	var bounded PrebakePair
	var encoded []byte
	for _, mode := range []string{"off", "low", "medium"} {
		rangeValue, err := OfflineRGBMRangeBounded(l, &d, 1, 3)
		if err != nil {
			t.Fatal(err)
		}
		bounded, err = NewOfflinePrebake(l, &d, direct.Image, OfflineBakeInputs{
			Samples: 16, Bounces: 1, Seed: 7, ReflectanceSHA256: strings.Repeat("a", 64), RGBMRange: rangeValue,
			Denoise: mode, MaxRayDistance: 3,
		})
		if err != nil || !strings.HasSuffix(bounded.Manifest.Producer, "@3") {
			t.Fatalf("bounded producer selection: %+v %v", bounded.Manifest, err)
		}
		encoded, err = EncodePrebake(bounded, l, &d)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := DecodePrebake(encoded, bounded.Image, l, &d)
		if err != nil || decoded.Manifest != bounded.Manifest ||
			binary.LittleEndian.Uint64(encoded[len(encoded)-8:]) != math.Float64bits(3) {
			t.Fatalf("bounded distance did not round trip: %v", err)
		}
	}
	for _, distance := range []float64{0, 2, -1, math.Inf(1), math.NaN(), MaxBakeRayDistance + 1} {
		changed := bounded
		changed.Manifest.MaxRayDistance = distance
		if changed.Validate(l, &d) == nil {
			t.Fatalf("tampered manifest distance accepted: %v", distance)
		}
	}
	// Recompute the payload header so the scan reaches the conditional field.
	missing := bytes.Clone(encoded[:len(encoded)-8])
	binary.LittleEndian.PutUint32(missing[8:], uint32(len(missing)-16))
	if _, err := DecodePrebakeManifest(missing); err == nil {
		t.Fatal("bounded producer accepted absent typed distance")
	}
	for _, mode := range []string{"off", "low", "medium"} {
		rangeValue, err := OfflineRGBMRange(l, &d, 1)
		if err != nil {
			t.Fatal(err)
		}
		pair, err := NewOfflinePrebake(l, &d, direct.Image, OfflineBakeInputs{
			Samples: 16, Bounces: 1, ReflectanceSHA256: strings.Repeat("a", 64), RGBMRange: rangeValue, Denoise: mode,
		})
		if err != nil {
			t.Fatal(err)
		}
		raw, err := EncodePrebake(pair, l, &d)
		if err != nil {
			t.Fatal(err)
		}
		raw = append(raw, make([]byte, 8)...)
		binary.LittleEndian.PutUint32(raw[8:], uint32(len(raw)-16))
		if _, err := DecodePrebakeManifest(raw); err == nil {
			t.Fatal("unbounded manifest accepted extra conditional distance")
		}
	}
}

func TestBoundedProducerRejectsInvalidDistanceAndRetainsZeroSemantics(t *testing.T) {
	for _, mode := range []string{"", "off", "low", "medium"} {
		old, err := OfflineDenoiseProducer(mode)
		zero, zeroErr := OfflineProducer(mode, 0)
		if err != nil || zeroErr != nil || old != zero {
			t.Fatal("zero did not preserve revision-2 producer")
		}
		for _, distance := range []float64{-1, math.NaN(), math.Inf(1), math.Inf(-1), MaxBakeRayDistance + 1} {
			if _, err := OfflineProducer(mode, distance); err == nil {
				t.Fatalf("invalid bounded distance accepted: %g", distance)
			}
		}
	}
}
