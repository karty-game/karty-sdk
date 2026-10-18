package worldvegetation_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"reflect"
	"testing"

	"github.com/karty-game/karty-sdk/format/worldvegetation"
)

func TestTreeVersionAndLegacyNormalLayout(t *testing.T) {
	tree := fixture()
	tree.Version = worldvegetation.TreeVersion
	tree.Presets[0].Geometry = worldvegetation.GeometryTree
	tree.Presets[0].NormalTexture = 19
	encoded, err := worldvegetation.Encode(tree)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded[:4]) != "KWV4" {
		t.Fatal("tree binary magic")
	}
	decoded, err := worldvegetation.Decode(encoded)
	if err != nil || !reflect.DeepEqual(tree, decoded) {
		t.Fatal("tree roundtrip", decoded, err)
	}
	canonical, err := worldvegetation.Encode(decoded)
	if err != nil || !bytes.Equal(encoded, canonical) {
		t.Fatal("noncanonical tree", err)
	}
	for end := range len(encoded) {
		if _, err := worldvegetation.Decode(encoded[:end]); err == nil {
			t.Fatal("accepted tree truncation", end)
		}
	}
	layerSize := 4 + 4 + len(tree.Layers[0].Preset) + 8 + 4 + 4 + len(tree.Layers[0].Surface)
	bad := bytes.Clone(encoded)
	binary.LittleEndian.PutUint32(bad[len(encoded)-layerSize-4:], ^uint32(0))
	if _, err := worldvegetation.Decode(bad); !errors.Is(err, worldvegetation.ErrBounds) {
		t.Fatal("late tree count", err)
	}
	if allocations := testing.AllocsPerRun(20, func() { _, _ = worldvegetation.Decode(bad) }); allocations > 1 {
		t.Fatal("malformed tree preflight allocated", allocations)
	}
	for _, version := range []uint16{worldvegetation.Version, worldvegetation.GeometryVersion, worldvegetation.NormalVersion} {
		tree.Version = version
		tree.Presets[0].NormalTexture = 0
		if _, err := worldvegetation.Encode(tree); !errors.Is(err, worldvegetation.ErrVersion) {
			t.Fatal("legacy accepted tree", version, err)
		}
	}
	tree.Version = worldvegetation.TreeVersion
	tree.Presets[0].Geometry = worldvegetation.GeometryPalm + 1
	if _, err := worldvegetation.Encode(tree); !errors.Is(err, worldvegetation.ErrVegetation) {
		t.Fatal("unknown tree geometry accepted", err)
	}
	tree.Presets[0].Geometry = worldvegetation.GeometryShrub
	tree.Presets[0].NormalTexture = 19
	normalLayout, err := worldvegetation.Encode(tree)
	if err != nil {
		t.Fatal(err)
	}
	tree.Version = worldvegetation.NormalVersion
	previous, err := worldvegetation.Encode(tree)
	if err != nil {
		t.Fatal(err)
	}
	if string(previous[:4]) != "KWV3" || !bytes.Equal(previous[18:], normalLayout[18:]) {
		t.Fatal("v3/v4 record layout changed")
	}
	old, err := worldvegetation.Decode(previous)
	if err != nil || !reflect.DeepEqual(old, tree) {
		t.Fatal("legacy v3 normals changed", err)
	}
	tree.Version = worldvegetation.TreeVersion
	unused := tree.Presets[0]
	unused.Name = "unused tree"
	unused.Geometry = worldvegetation.GeometryTree
	tree.Presets = append(tree.Presets, unused)
	tree.Version = worldvegetation.NormalVersion
	if _, err := worldvegetation.Encode(tree); !errors.Is(err, worldvegetation.ErrVersion) {
		t.Fatal("unused legacy tree accepted", err)
	}
	feature, err := worldvegetation.FeatureForVersion(worldvegetation.TreeVersion)
	if err != nil || feature != "world/vegetation@4" {
		t.Fatal("tree capability", feature, err)
	}
}
