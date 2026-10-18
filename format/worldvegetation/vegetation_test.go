package worldvegetation_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldvegetation"
)

func fixture() worldvegetation.Document {
	return worldvegetation.Document{
		Version: worldvegetation.Version,
		Presets: []worldvegetation.Preset{
			{
				Name:          "grass",
				Texture:       17,
				Width:         worldvegetation.Range{Min: .2, Max: .4},
				Height:        worldvegetation.Range{Min: .3, Max: .7},
				WindAmplitude: .08,
				WindFrequency: .6,
				DrawDistance:  48,
			},
		},
		Layers: []worldvegetation.Layer{{Preset: "grass", Density: 4, Seed: 31, Surface: worldvegetation.SurfaceFloor}},
	}
}
func geometry() world.Document {
	return world.Document{
		Version: world.Version,
		Sectors: []world.Sector{
			{
				ID:              "field",
				SourceRoom:      "field",
				Floor:           world.Plane{},
				Ceiling:         world.Plane{C: 4},
				FloorMaterial:   1,
				CeilingMaterial: 1,
				Walls: []world.Wall{
					{Start: world.Vec2{}, End: world.Vec2{X: 10}, Portal: -1, Material: 1},
					{Start: world.Vec2{X: 10}, End: world.Vec2{X: 10, Y: 10}, Portal: -1, Material: 1},
					{Start: world.Vec2{X: 10, Y: 10}, End: world.Vec2{Y: 10}, Portal: -1, Material: 1},
					{Start: world.Vec2{Y: 10}, End: world.Vec2{}, Portal: -1, Material: 1},
				},
			},
		},
	}
}
func TestBinaryVegetationCanonicalAndBounded(t *testing.T) {
	d := fixture()
	data, err := worldvegetation.Encode(d)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := worldvegetation.Decode(data)
	if err != nil || !reflect.DeepEqual(d, decoded) {
		t.Fatalf("roundtrip %+v %v", decoded, err)
	}
	again, err := worldvegetation.Encode(decoded)
	if err != nil || !bytes.Equal(data, again) {
		t.Fatal("noncanonical roundtrip")
	}
	for end := range len(data) {
		if _, err := worldvegetation.Decode(data[:end]); err == nil {
			t.Fatalf("accepted truncation %d", end)
		}
	}
	for _, bad := range [][]byte{append(bytes.Clone(data), 0), []byte(`{"version":1}`)} {
		if _, err := worldvegetation.Decode(bad); err == nil {
			t.Fatal("accepted trailing/JSON")
		}
	}
	bad := bytes.Clone(data)
	binary.LittleEndian.PutUint32(bad[18:22], math.MaxUint32)
	if _, err := worldvegetation.Decode(bad); !errors.Is(err, worldvegetation.ErrBounds) {
		t.Fatalf("preset count %v", err)
	}
	if n := testing.AllocsPerRun(20, func() { _, _ = worldvegetation.Decode(bad) }); n > 1 {
		t.Fatalf("malformed preflight allocated %.1f", n)
	}
	// Forge the last array count: the complete scan must reject this before
	// materializing even the valid preset array preceding it.
	layerSize := 4 + 4 + len(d.Layers[0].Preset) + 8 + 4 + 4 + len(d.Layers[0].Surface)
	layerCountOffset := len(data) - layerSize - 4
	bad = bytes.Clone(data)
	binary.LittleEndian.PutUint32(bad[layerCountOffset:], math.MaxUint32)
	if _, err := worldvegetation.Decode(bad); !errors.Is(err, worldvegetation.ErrBounds) {
		t.Fatalf("late layer count: %v", err)
	}
	if n := testing.AllocsPerRun(20, func() { _, _ = worldvegetation.Decode(bad) }); n > 1 {
		t.Fatalf("late malformed preflight allocated %.1f", n)
	}
	bad = bytes.Clone(data)
	bad[4] = 2
	if _, err := worldvegetation.Decode(bad); !errors.Is(err, worldvegetation.ErrVersion) {
		t.Fatalf("header version %v", err)
	}
	bad = bytes.Clone(data)
	binary.LittleEndian.PutUint16(bad[16:18], 2)
	if _, err := worldvegetation.Decode(bad); !errors.Is(err, worldvegetation.ErrVersion) {
		t.Fatalf("document version %v", err)
	}
}
func TestCompleteVegetationValidation(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		change func(*worldvegetation.Document)
	}{
		{"duplicate preset", func(d *worldvegetation.Document) { d.Presets = append(d.Presets, d.Presets[0]) }},
		{"unused invalid preset", func(d *worldvegetation.Document) {
			p := d.Presets[0]
			p.Name = "unused"
			p.Width.Min = math.NaN()
			d.Presets = append(d.Presets, p)
		}},
		{"duplicate layer", func(d *worldvegetation.Document) { d.Layers = append(d.Layers, d.Layers[0]) }},
		{"unknown preset", func(d *worldvegetation.Document) { d.Layers[0].Preset = "missing" }},
		{"density overflow", func(d *worldvegetation.Document) { d.Layers[0].Density = worldvegetation.MaxDensity + .01 }},
		{"density nonfinite", func(d *worldvegetation.Document) { d.Layers[0].Density = math.Inf(1) }},
		{"surface missing", func(d *worldvegetation.Document) { d.Layers[0].Surface = "" }},
		{"invalid range", func(d *worldvegetation.Document) { d.Presets[0].Width.Min = 1 }},
		{"wind overflow", func(d *worldvegetation.Document) { d.Presets[0].WindAmplitude = worldvegetation.MaxWindAmplitude + 1 }},
		{"distance overflow", func(d *worldvegetation.Document) { d.Presets[0].DrawDistance = worldvegetation.MaxDrawDistance + 1 }},
		{"missing texture", func(d *worldvegetation.Document) { d.Presets[0].Texture = 0 }},
		{"empty layers", func(d *worldvegetation.Document) { d.Layers = nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			d := fixture()
			test.change(&d)
			if _, err := worldvegetation.Encode(d); err == nil {
				t.Fatal("accepted malformed document")
			}
		})
	}
	d := fixture()
	d.Layers[0].Density = 0
	if err := worldvegetation.Validate(&d); err != nil {
		t.Fatalf("disabled layer %v", err)
	}
	d = fixture()
	p := d.Presets[0]
	p.Name = "unused"
	p.Texture = 19
	d.Presets = append(d.Presets, p)
	if err := worldvegetation.ValidateTextures(&d, map[uint32]bool{17: true}); !errors.Is(err, worldvegetation.ErrIdentity) {
		t.Fatalf("unused texture skipped %v", err)
	}
	if err := worldvegetation.ValidateTextures(&d, map[uint32]bool{17: true, 19: true}); err != nil {
		t.Fatal(err)
	}
}
func TestVegetationSectorReferencesAndClearance(t *testing.T) {
	t.Parallel()
	d := fixture()
	w := geometry()
	if err := world.Validate(&w); err != nil {
		t.Fatal(err)
	}
	if err := worldvegetation.ValidateForWorld(&d, &w); err != nil {
		t.Fatal(err)
	}
	d.Layers[0].Surface = worldvegetation.SurfaceCeiling
	if err := worldvegetation.ValidateForWorld(&d, &w); err != nil {
		t.Fatal(err)
	}
	w.Sectors[0].Sky = true
	if err := worldvegetation.ValidateForWorld(&d, &w); !errors.Is(err, worldvegetation.ErrConfinement) {
		t.Fatalf("sky ceiling %v", err)
	}
	d.Layers[0].Surface = worldvegetation.SurfaceFloor
	if err := worldvegetation.ValidateForWorld(&d, &w); err != nil {
		t.Fatalf("sky floor %v", err)
	}
	d.Layers[0].Sector = math.MaxUint32
	if err := worldvegetation.ValidateForWorld(&d, &w); !errors.Is(err, worldvegetation.ErrConfinement) {
		t.Fatalf("sector overflow %v", err)
	}
	d = fixture()
	w = geometry()
	w.Sectors[0].Ceiling.C = .6
	if err := worldvegetation.ValidateForWorld(&d, &w); !errors.Is(err, worldvegetation.ErrConfinement) {
		t.Fatalf("vertical clearance %v", err)
	}
	w = geometry()
	w.Sectors[0].Floor = world.Plane{A: .1}
	w.Sectors[0].Ceiling = world.Plane{A: .1, C: 4}
	if err := worldvegetation.ValidateForWorld(&d, &w); err != nil {
		t.Fatalf("parallel sloping planes %v", err)
	}
	w.Sectors[0].Ceiling = world.Plane{A: .2, C: .71}
	if err := worldvegetation.ValidateForWorld(&d, &w); !errors.Is(err, worldvegetation.ErrConfinement) {
		t.Fatalf("slope footprint margin %v", err)
	}
}
func FuzzVegetationDecode(f *testing.F) {
	data, _ := worldvegetation.Encode(fixture())
	f.Add(data)
	shrub := fixture()
	shrub.Version = worldvegetation.GeometryVersion
	shrub.Presets[0].Geometry = worldvegetation.GeometryShrub
	encodedShrub, _ := worldvegetation.Encode(shrub)
	f.Add(encodedShrub)
	shrub.Version = worldvegetation.NormalVersion
	shrub.Presets[0].NormalTexture = 19
	encodedNormal, _ := worldvegetation.Encode(shrub)
	f.Add(encodedNormal)
	shrub.Version = worldvegetation.TreeVersion
	shrub.Presets[0].Geometry = worldvegetation.GeometryTree
	encodedTree, _ := worldvegetation.Encode(shrub)
	f.Add(encodedTree)
	shrub.Version = worldvegetation.PalmVersion
	shrub.Presets[0].Geometry = worldvegetation.GeometryPalm
	encodedPalm, _ := worldvegetation.Encode(shrub)
	f.Add(encodedPalm)
	f.Add([]byte("KWV1"))
	f.Fuzz(func(t *testing.T, data []byte) {
		d, err := worldvegetation.Decode(data)
		if err != nil {
			return
		}
		canonical, err := worldvegetation.Encode(d)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(canonical, data) {
			t.Fatal("noncanonical decode")
		}
	})
}
