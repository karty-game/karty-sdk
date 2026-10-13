package world_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"reflect"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
)

func TestBackdropPreservesExistingBinaryEnvelopes(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		document world.Document
		hash     string
	}{
		{lightingWorld(), "016f1a4306051de49b66efe47b9384a88cb05dd575171dc912affc79c5236986"},
		{solidRendererWorld(), "d77aed01aedbc853d21bf3e86b1f94e568a09db3b9f42735d8ce7c01890a0fb7"},
	} {
		encoded, err := world.Encode(test.document)
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprintf("%x", sha256.Sum256(encoded)) != test.hash {
			t.Fatal("omitted backdrop changed existing binary bytes")
		}
	}
}

func TestOmittedBackdropKeepsFrozenBaseEnvironmentBytes(t *testing.T) {
	t.Parallel()
	document := lightingWorld()
	document.Environment = &world.Environment{Version: 1, Preset: world.EnvironmentDaylight}
	encoded, err := world.Encode(document)
	if err != nil {
		t.Fatal(err)
	}
	// Existing optional flag, version, preset, palette and three omitted controls.
	tail, err := hex.DecodeString("0101000000080000006461796c6967687400000000000000")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(encoded, []byte(world.BinaryMagic)) || !bytes.HasSuffix(encoded, tail) {
		t.Fatal("base Environment field layout changed")
	}
}

func TestBackdropDefaultsAndOverridesPreserveLightingAndLiveControls(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		preset   world.EnvironmentPreset
		backdrop world.EnvironmentBackdrop
	}{
		{world.EnvironmentDaylight, world.EnvironmentBackdropMountains}, {world.EnvironmentFantasy, world.EnvironmentBackdropMountains},
		{world.EnvironmentCartoon, world.EnvironmentBackdropCity}, {world.EnvironmentSpace, world.EnvironmentBackdropCity},
	} {
		environment := &world.Environment{Version: 1, Preset: test.preset}
		base, err := world.ResolveEnvironment(environment)
		if err != nil || base.Backdrop != test.backdrop {
			t.Fatal("preset backdrop default", test, err)
		}
		for _, backdrop := range []world.EnvironmentBackdrop{world.EnvironmentBackdropNone, world.EnvironmentBackdropMountains, world.EnvironmentBackdropCity} {
			environment.Backdrop = backdrop
			resolved, err := world.ResolveEnvironment(environment)
			if err != nil {
				t.Fatal(err)
			}
			if resolved.Backdrop != backdrop || resolved.SunDirection != base.SunDirection || resolved.AmbientCube != base.AmbientCube {
				t.Fatal("decorative layer changed illumination")
			}
			live, err := world.ResolveEnvironmentSettings(environment, world.EnvironmentSettings{TimeOfDay: 0, CloudAmount: 1})
			if err != nil || live.Backdrop != backdrop || live.SunIntensity != 0 || live.CloudAmount != 1 {
				t.Fatal("clock/cloud override lost authored backdrop", err)
			}
		}
	}
	environment := &world.Environment{Version: 1, Preset: world.EnvironmentDaylight, Backdrop: "forest"}
	if err := world.ValidateEnvironment(environment); err == nil {
		t.Fatal("unknown backdrop accepted")
	}
}

func TestBackdropOnlyAndCombinedKWR2RoundTrips(t *testing.T) {
	t.Parallel()
	for _, backdrop := range []world.EnvironmentBackdrop{world.EnvironmentBackdropNone, world.EnvironmentBackdropMountains, world.EnvironmentBackdropCity} {
		for _, document := range []world.Document{lightingWorld(), solidRendererWorld()} {
			document.Environment = &world.Environment{Version: 1, Preset: world.EnvironmentDaylight, Backdrop: backdrop}
			encoded, err := world.Encode(document)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.HasPrefix(encoded, []byte(world.SolidBinaryMagic)) {
				t.Fatal("explicit selector used old envelope")
			}
			decoded, err := world.Decode(encoded)
			if err != nil || !reflect.DeepEqual(document, decoded) {
				t.Fatal("backdrop round trip", err)
			}
			rebuilt, err := world.Encode(decoded)
			if err != nil || !bytes.Equal(encoded, rebuilt) {
				t.Fatal("noncanonical backdrop envelope", err)
			}
		}
	}
}

func resizeBackdropFixture(encoded []byte) []byte {
	binary.LittleEndian.PutUint32(encoded[8:12], uint32(len(encoded)-16))
	return encoded
}

func TestBackdropRejectsIncompleteNoncanonicalAndTrailingExtensions(t *testing.T) {
	t.Parallel()
	document := lightingWorld()
	document.Environment = &world.Environment{Version: 1, Preset: world.EnvironmentDaylight, Backdrop: world.EnvironmentBackdropNone}
	encoded, err := world.Encode(document)
	if err != nil {
		t.Fatal(err)
	}
	inputs := [][]byte{
		resizeBackdropFixture(bytes.Clone(encoded[:len(encoded)-1])),
		resizeBackdropFixture(append(bytes.Clone(encoded), 2)),
		resizeBackdropFixture(append(bytes.Clone(encoded), 0)),
		append([]byte(world.BinaryMagic), encoded[4:]...),
	}
	for _, selector := range []byte{0, 4, 255} {
		bad := bytes.Clone(encoded)
		bad[len(bad)-1] = selector
		inputs = append(inputs, bad)
	}
	solidOnly, err := world.Encode(solidRendererWorld())
	if err != nil {
		t.Fatal(err)
	}
	inputs = append(inputs, resizeBackdropFixture(append(solidOnly, 1))) // selector requires environment
	for _, input := range inputs {
		if decoded, err := world.Decode(input); err == nil || len(decoded.Sectors) != 0 || decoded.Environment != nil {
			t.Fatal("malformed backdrop published prefix", err)
		}
	}
	for length := range len(encoded) {
		if _, err := world.Decode(encoded[:length]); err == nil {
			t.Fatal("truncated backdrop accepted", length)
		}
	}
}
