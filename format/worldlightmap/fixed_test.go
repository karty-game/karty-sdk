package worldlightmap

import (
	"bytes"
	"encoding/binary"
	"math"
	"strings"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
)

func fixedFormatFixture(t *testing.T) (world.Document, Layout, FixedLighting, []byte) {
	t.Helper()
	document, layout, _, image := environmentCatalogFixture(t)
	environment, err := world.ResolveEnvironment(document.Environment)
	if err != nil {
		t.Fatal(err)
	}
	surface, err := EnvironmentSurfaceDigest(&document)
	if err != nil {
		t.Fatal(err)
	}
	manifest := FixedLighting{
		Samples:           2,
		Bounces:           1,
		RGBMRange:         4,
		ReflectanceSHA256: strings.Repeat("a", 64),
		InputSHA256:       strings.Repeat("b", 64),
		SurfaceSHA256:     surface,
		Lights: []CompiledLight{
			{
				ID:        "@sun",
				Kind:      "sun",
				Direction: environment.SunDirection,
				Color:     scale(environment.SunColor, environment.SunIntensity*(1-environment.CloudAmount)),
			},
		},
		Probes: []LightingProbe{{Sector: 0, Direct: [2]ProbeDirect{{Light: 0, Visibility: 1}, {Light: -1}}}},
	}
	manifest, err = NewFixedLighting(layout, &document, image, manifest)
	if err != nil {
		t.Fatal(err)
	}
	return document, layout, manifest, image
}

func TestFixedFormatRejectsMalformedWireAndOwnsDecodedSlices(t *testing.T) {
	_, _, manifest, _ := fixedFormatFixture(t)
	encoded, err := EncodeFixedLighting(manifest)
	if err != nil {
		t.Fatal(err)
	}
	at := 16
	for range 10 {
		size := int(binary.LittleEndian.Uint32(encoded[at:]))
		at += 4 + size
	}
	at += 5*4 + 8
	at += 4 + int(binary.LittleEndian.Uint32(encoded[at:]))
	for _, bits := range []uint64{math.Float64bits(math.NaN()), math.Float64bits(math.Inf(1)), uint64(1) << 63} {
		broken := bytes.Clone(encoded)
		binary.LittleEndian.PutUint64(broken[at:], bits)
		if _, err := DecodeFixedLighting(broken); err == nil {
			t.Fatal("noncanonical/nonfinite scalar accepted")
		}
	}
	broken := bytes.Clone(encoded)
	binary.LittleEndian.PutUint32(broken[at+8:], MaxCompiledLights+2)
	if _, err := DecodeFixedLighting(broken); err == nil {
		t.Fatal("excess source count accepted before allocation")
	}
	decoded, err := DecodeFixedLighting(encoded)
	if err != nil {
		t.Fatal(err)
	}
	clear(encoded)
	if decoded.Lights[0].ID != "@sun" || decoded.Probes[0].Direct[0].Visibility != 1 {
		t.Fatal("decoded resources borrow wire backing")
	}
}

func TestFixedFormatSourceProofAndDirectAccountingBounds(t *testing.T) {
	document, layout, manifest, image := fixedFormatFixture(t)
	manifest.Lights[0].Color.X += 1
	if err := ValidateFixedLighting(manifest, layout, &document); err == nil {
		t.Fatal("altered compiled sun passed source proof")
	}
	_, _, manifest, _ = fixedFormatFixture(t)
	manifest.Probes[0].Direct[1] = manifest.Probes[0].Direct[0]
	if _, err := EncodeFixedLighting(manifest); err == nil {
		t.Fatal("duplicate direct contribution accepted")
	}
	manifest.Probes[0].Direct[1] = ProbeDirect{Light: -1, Visibility: 1}
	if _, err := EncodeFixedLighting(manifest); err == nil {
		t.Fatal("empty direct contribution carries energy")
	}
	manifest.Probes[0].Direct[1] = ProbeDirect{Light: -1}
	image = bytes.Clone(image)
	image[len(image)/2] ^= 1
	if err := ValidateFixedLightingArtifact(manifest, image, layout); err == nil {
		t.Fatal("changed image accepted")
	}
}

func TestFixedProducerRevisionPreservesArtifactReadability(t *testing.T) {
	document, layout, manifest, image := fixedFormatFixture(t)
	for _, producer := range []string{"cpu-fixed-rnm3@1", "cpu-fixed-rnm3@2", FixedProducer} {
		manifest.Producer = producer
		encoded, err := EncodeFixedLighting(manifest)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := DecodeFixedLightingArtifact(encoded, image, layout)
		if err != nil || decoded.Producer != producer {
			t.Fatalf("completed producer %s artifact rejected: %v", producer, err)
		}
		rebound, err := NewFixedLighting(layout, &document, image, decoded)
		if err != nil || rebound.Producer != producer {
			t.Fatalf("completed producer %s relabeled when binding image: %v", producer, err)
		}
	}
	manifest.Producer = "cpu-fixed-rnm3@4"
	if _, err := EncodeFixedLighting(manifest); err == nil {
		t.Fatal("unknown fixed producer accepted")
	}
}
