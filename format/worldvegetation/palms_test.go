package worldvegetation_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"reflect"
	"testing"

	"github.com/karty-game/karty-sdk/format/worldvegetation"
)

func TestPalmVersionAndLegacyLayouts(t *testing.T) {
	palm := fixture()
	palm.Version = worldvegetation.PalmVersion
	palm.Presets[0].Geometry = worldvegetation.GeometryPalm
	palm.Presets[0].NormalTexture = 19
	encoded, err := worldvegetation.Encode(palm)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded[:4]) != "KWV5" {
		t.Fatal("palm magic")
	}
	decoded, err := worldvegetation.Decode(encoded)
	if err != nil || !reflect.DeepEqual(palm, decoded) {
		t.Fatal("palm roundtrip", err)
	}
	canonical, err := worldvegetation.Encode(decoded)
	if err != nil || !bytes.Equal(encoded, canonical) {
		t.Fatal("palm noncanonical", err)
	}
	for end := range len(encoded) {
		if _, err := worldvegetation.Decode(encoded[:end]); err == nil {
			t.Fatal("accepted truncation", end)
		}
	}
	for _, version := range []uint16{1, 2, 3, 4} {
		palm.Version = version
		palm.Presets[0].NormalTexture = 0
		if _, err := worldvegetation.Encode(palm); !errors.Is(err, worldvegetation.ErrVersion) {
			t.Fatal("legacy palm", version, err)
		}
	}
	palm.Version = worldvegetation.PalmVersion
	palm.Presets[0].Geometry = worldvegetation.GeometryTree
	modern, err := worldvegetation.Encode(palm)
	if err != nil {
		t.Fatal(err)
	}
	palm.Version = worldvegetation.TreeVersion
	legacy, err := worldvegetation.Encode(palm)
	if err != nil || !bytes.Equal(modern[18:], legacy[18:]) {
		t.Fatal("v4/v5 record layout changed", err)
	}
	old, err := worldvegetation.Decode(legacy)
	if err != nil || !reflect.DeepEqual(old, palm) {
		t.Fatal("v4 decode changed", err)
	}
	mismatch := bytes.Clone(modern)
	copy(mismatch[:4], "KWV4")
	if _, err := worldvegetation.Decode(mismatch); !errors.Is(err, worldvegetation.ErrVersion) {
		t.Fatal("magic/version mismatch accepted", err)
	}
	unused := palm.Presets[0]
	unused.Name = "unused palm"
	unused.Geometry = worldvegetation.GeometryPalm
	palm.Presets = append(palm.Presets, unused)
	if err := worldvegetation.Validate(&palm); !errors.Is(err, worldvegetation.ErrVersion) {
		t.Fatal("unused legacy palm", err)
	}
	palm.Version = worldvegetation.PalmVersion
	palm.Presets[1].Geometry = worldvegetation.GeometryPalm + 1
	if err := worldvegetation.Validate(&palm); !errors.Is(err, worldvegetation.ErrVegetation) {
		t.Fatal("unused unknown geometry", err)
	}
	feature, err := worldvegetation.FeatureForVersion(worldvegetation.PalmVersion)
	if err != nil || feature != "world/vegetation@5" {
		t.Fatal("palm capability", feature, err)
	}
	// Invalid late counts must fail the full preflight before typed allocations.
	layerSize := 4 + 4 + len(decoded.Layers[0].Preset) + 8 + 4 + 4 + len(decoded.Layers[0].Surface)
	bad := bytes.Clone(encoded)
	binary.LittleEndian.PutUint32(bad[len(encoded)-layerSize-4:], ^uint32(0))
	if _, err := worldvegetation.Decode(bad); !errors.Is(err, worldvegetation.ErrBounds) {
		t.Fatal("late count", err)
	}
	if allocations := testing.AllocsPerRun(20, func() { _, _ = worldvegetation.Decode(bad) }); allocations > 1 {
		t.Fatal("malformed preflight allocated", allocations)
	}
}
