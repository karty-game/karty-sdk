package worldvegetation_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"reflect"
	"testing"

	"github.com/karty-game/karty-sdk/format/worldvegetation"
)

func TestShrubGeometryBinaryVersionAndCompatibility(t *testing.T) {
	legacy := fixture()
	legacyBytes, err := worldvegetation.Encode(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if string(legacyBytes[:4]) != "KWV1" {
		t.Fatal("legacy magic changed")
	}
	shrub := fixture()
	shrub.Version = worldvegetation.GeometryVersion
	shrub.Presets[0].Geometry = worldvegetation.GeometryShrub
	encoded, err := worldvegetation.Encode(shrub)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded[:4]) != "KWV2" || len(encoded) != len(legacyBytes)+len(shrub.Presets) {
		t.Fatal("v2 geometry layout changed")
	}
	decoded, err := worldvegetation.Decode(encoded)
	if err != nil || !reflect.DeepEqual(decoded, shrub) {
		t.Fatal("shrub roundtrip", err, decoded)
	}
	canonical, err := worldvegetation.Encode(decoded)
	if err != nil || !bytes.Equal(encoded, canonical) {
		t.Fatal("noncanonical shrub", err)
	}
	for end := range len(encoded) {
		if _, err := worldvegetation.Decode(encoded[:end]); err == nil {
			t.Fatal("accepted truncation", end)
		}
	}
	for _, test := range []struct {
		magic   string
		version uint16
	}{{"KWV1", 2}, {"KWV2", 1}, {"KWV2", 3}} {
		bad := bytes.Clone(encoded)
		copy(bad[:4], test.magic)
		binary.LittleEndian.PutUint16(bad[16:18], test.version)
		if _, err := worldvegetation.Decode(bad); err == nil {
			t.Fatal("accepted mismatched version", test)
		}
	}
	layerSize := 4 + 4 + len(shrub.Layers[0].Preset) + 8 + 4 + 4 + len(shrub.Layers[0].Surface)
	badCount := bytes.Clone(encoded)
	binary.LittleEndian.PutUint32(badCount[len(encoded)-layerSize-4:], ^uint32(0))
	if _, err := worldvegetation.Decode(badCount); !errors.Is(err, worldvegetation.ErrBounds) {
		t.Fatal("malformed late v2 layer count", err)
	}
	if allocations := testing.AllocsPerRun(20, func() { _, _ = worldvegetation.Decode(badCount) }); allocations > 1 {
		t.Fatal("v2 malformed preflight allocated", allocations)
	}
	shrub.Version = worldvegetation.Version
	if _, err := worldvegetation.Encode(shrub); !errors.Is(err, worldvegetation.ErrVersion) {
		t.Fatal("v1 accepted shrub", err)
	}
	shrub.Version = worldvegetation.GeometryVersion
	shrub.Presets[0].Geometry = worldvegetation.GeometryPalm + 1
	if _, err := worldvegetation.Encode(shrub); !errors.Is(err, worldvegetation.ErrVegetation) {
		t.Fatal("accepted unknown geometry", err)
	}
	for _, version := range []uint16{worldvegetation.Version, worldvegetation.GeometryVersion} {
		feature, err := worldvegetation.FeatureForVersion(version)
		if err != nil || feature != map[uint16]string{1: "world/vegetation@1", 2: "world/vegetation@2"}[version] {
			t.Fatal("wrong capability", feature, err)
		}
	}
}
