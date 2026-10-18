package worldvegetation_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"reflect"
	"testing"

	"github.com/karty-game/karty-sdk/format/worldvegetation"
)

func TestNormalMapVersionedBinaryAndCompleteValidation(t *testing.T) {
	document := fixture()
	document.Version = worldvegetation.NormalVersion
	document.Presets[0].Geometry = worldvegetation.GeometryShrub
	document.Presets[0].NormalTexture = 19
	encoded, err := worldvegetation.Encode(document)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded[:4]) != "KWV3" {
		t.Fatal("normal map magic")
	}
	decoded, err := worldvegetation.Decode(encoded)
	if err != nil || !reflect.DeepEqual(document, decoded) {
		t.Fatal("normal map roundtrip", decoded, err)
	}
	canonical, err := worldvegetation.Encode(decoded)
	if err != nil || !bytes.Equal(encoded, canonical) {
		t.Fatal("normal map canonical roundtrip", err)
	}
	for end := range len(encoded) {
		if _, err := worldvegetation.Decode(encoded[:end]); err == nil {
			t.Fatal("accepted v3 truncation", end)
		}
	}
	bad := append(bytes.Clone(encoded), 0)
	if _, err := worldvegetation.Decode(bad); err == nil {
		t.Fatal("accepted trailing v3 bytes")
	}
	layerSize := 4 + 4 + len(document.Layers[0].Preset) + 8 + 4 + 4 + len(document.Layers[0].Surface)
	bad = bytes.Clone(encoded)
	binary.LittleEndian.PutUint32(bad[len(encoded)-layerSize-4:], ^uint32(0))
	if _, err := worldvegetation.Decode(bad); !errors.Is(err, worldvegetation.ErrBounds) {
		t.Fatal("late v3 count", err)
	}
	if allocations := testing.AllocsPerRun(20, func() { _, _ = worldvegetation.Decode(bad) }); allocations > 1 {
		t.Fatal("malformed v3 preflight allocated", allocations)
	}
	if err := worldvegetation.ValidateTextures(&document, map[uint32]bool{17: true}); !errors.Is(err, worldvegetation.ErrIdentity) {
		t.Fatal("missing normal texture accepted", err)
	}
	if err := worldvegetation.ValidateTextures(&document, map[uint32]bool{17: true, 19: true}); err != nil {
		t.Fatal(err)
	}
	unused := document.Presets[0]
	unused.Name = "unused"
	unused.NormalTexture = 20
	document.Presets = append(document.Presets, unused)
	if err := worldvegetation.ValidateTextures(
		&document,
		map[uint32]bool{17: true, 19: true},
	); !errors.Is(
		err,
		worldvegetation.ErrIdentity,
	) {
		t.Fatal("unused missing normal texture accepted", err)
	}
	document.Presets = document.Presets[:1]
	for _, version := range []uint16{worldvegetation.Version, worldvegetation.GeometryVersion} {
		document.Version = version
		document.Presets[0].Geometry = worldvegetation.GeometryCrossedCards
		if _, err := worldvegetation.Encode(document); !errors.Is(err, worldvegetation.ErrVersion) {
			t.Fatal("legacy accepted normal map", version, err)
		}
		document.Presets[0].NormalTexture = 0
		legacy, err := worldvegetation.Encode(document)
		if err != nil {
			t.Fatal(err)
		}
		withoutNormal, err := worldvegetation.Decode(legacy)
		if err != nil || withoutNormal.Presets[0].NormalTexture != 0 {
			t.Fatal("legacy default normal changed", err)
		}
		document.Presets[0].NormalTexture = 19
	}
	feature, err := worldvegetation.FeatureForVersion(worldvegetation.NormalVersion)
	if err != nil || feature != "world/vegetation@3" {
		t.Fatal("normal capability", feature, err)
	}
}
