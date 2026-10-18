package worldparticles_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"reflect"
	"testing"

	"github.com/karty-game/karty-sdk/format/worldparticles"
)

func TestParticleVersionCapabilityAndLegacyHelpers(t *testing.T) {
	t.Parallel()
	if worldparticles.Version != 1 || worldparticles.Feature != "world/particles@1" || worldparticles.AlphaVersion != 2 {
		t.Fatal("legacy particle identifiers changed")
	}
	for _, test := range []struct {
		version uint16
		feature string
	}{
		{1, "world/particles@1"}, {2, "world/particles@2"},
	} {
		feature, err := worldparticles.FeatureForVersion(test.version)
		if err != nil || feature != test.feature {
			t.Fatalf("version capability %d: %s/%v", test.version, feature, err)
		}
	}
	for _, version := range []uint16{0, 3, 65535} {
		if _, err := worldparticles.FeatureForVersion(version); !errors.Is(err, worldparticles.ErrVersion) {
			t.Fatalf("unsupported version accepted: %d", version)
		}
	}
	preset := fixture().Presets[0]
	preset.Sprite.Blend = "alpha"
	if err := worldparticles.ValidatePreset(&preset); !errors.Is(err, worldparticles.ErrParticle) {
		t.Fatal("legacy helper accepted alpha", err)
	}
	if err := worldparticles.ValidatePresetForVersion(&preset, worldparticles.AlphaVersion); err != nil {
		t.Fatal(err)
	}
}

// Allocation preflight must run without parallel tests.
func TestAlphaParticleDocumentCanonicalVersionAndBounds(t *testing.T) {
	document := fixture()
	document.Version = worldparticles.AlphaVersion
	document.Presets[0].Sprite.Blend = "alpha"
	document.Presets[0].Sprite.Frames = []uint32{17}
	document.Presets[0].Sprite.FPS = 0
	encoded, err := worldparticles.Encode(document)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := worldparticles.Decode(encoded)
	if err != nil || !reflect.DeepEqual(document, decoded) {
		t.Fatalf("alpha roundtrip: %v", err)
	}
	again, err := worldparticles.Encode(decoded)
	if err != nil || !bytes.Equal(encoded, again) {
		t.Fatal("noncanonical alpha bytes", err)
	}
	for end := range len(encoded) {
		if _, err := worldparticles.Decode(encoded[:end]); err == nil {
			t.Fatalf("accepted alpha truncation %d", end)
		}
	}
	for _, version := range []uint16{worldparticles.Version, 3} {
		forged := bytes.Clone(encoded)
		binary.LittleEndian.PutUint16(forged[16:18], version)
		if _, err := worldparticles.Decode(forged); err == nil {
			t.Fatalf("unsupported/legacy alpha document accepted %d", version)
		}
	}
	bad := bytes.Clone(encoded)
	binary.LittleEndian.PutUint32(bad[18:22], uint32(worldparticles.MaxPresets+1))
	if _, err := worldparticles.Decode(bad); !errors.Is(err, worldparticles.ErrBounds) {
		t.Fatal("alpha count preflight lost", err)
	}
	if allocations := testing.AllocsPerRun(20, func() { _, _ = worldparticles.Decode(bad) }); allocations > 1 {
		t.Fatalf("alpha invalid preflight allocated %.0f times", allocations)
	}
	for _, blend := range []string{"additive", "cutout"} {
		document.Presets[0].Sprite.Blend = blend
		if err := worldparticles.Validate(&document); err != nil {
			t.Fatal("v2 lost legacy blend", err)
		}
	}
}
