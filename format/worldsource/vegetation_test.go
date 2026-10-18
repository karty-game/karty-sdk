package worldsource_test

import (
	"errors"
	"testing"

	"github.com/karty-game/karty-sdk/format/worldsource"
	"github.com/karty-game/karty-sdk/format/worldvegetation"
)

func vegetationSource() worldsource.Document {
	d := validSource()
	d.Version = worldsource.VegetationVersion
	d.Vegetation = &worldsource.Vegetation{
		Version: worldvegetation.Version,
		Presets: []worldsource.VegetationPreset{
			{
				Name:          "grass",
				Texture:       "grass",
				Width:         worldvegetation.Range{Min: .2, Max: .4},
				Height:        worldvegetation.Range{Min: .3, Max: .7},
				WindAmplitude: .08,
				WindFrequency: .6,
				DrawDistance:  48,
			},
		},
	}
	d.Rooms[0].VegetationLayers = []worldsource.VegetationLayer{
		{Preset: "grass", Density: 4, Seed: 31, Surface: worldvegetation.SurfaceFloor},
	}
	return d
}
func TestVegetationSourceVersionAndTextureResolution(t *testing.T) {
	t.Parallel()
	if worldsource.Version < worldsource.VegetationTreeVersion || worldsource.VegetationVersion != 13 {
		t.Fatal("vegetation source gate changed")
	}
	d := vegetationSource()
	if err := worldsource.Validate(&d); err != nil {
		t.Fatal(err)
	}
	for version := uint16(1); version < worldsource.VegetationVersion; version++ {
		d.Version = version
		if err := worldsource.Validate(&d); !errors.Is(err, worldsource.ErrVersion) {
			t.Fatalf("v%d vegetation gate %v", version, err)
		}
	}
	d = vegetationSource()
	p, err := worldsource.CompileVegetationPreset(d.Vegetation.Presets[0], map[string]uint32{"grass": 17})
	if err != nil || p.Texture != 17 || p.Name != "grass" {
		t.Fatalf("compile %+v %v", p, err)
	}
	if _, err := worldsource.CompileVegetationPreset(d.Vegetation.Presets[0], nil); !errors.Is(err, worldsource.ErrReference) {
		t.Fatalf("unresolved texture %v", err)
	}
	d.Rooms[0].VegetationLayers = nil
	if err := worldsource.Validate(&d); err != nil {
		t.Fatalf("unused library %v", err)
	}
	d.Vegetation.Presets = append(d.Vegetation.Presets, d.Vegetation.Presets[0])
	if err := worldsource.Validate(&d); !errors.Is(err, worldvegetation.ErrIdentity) {
		t.Fatalf("unused duplicate library %v", err)
	}
}
func TestVegetationSourceCompleteReferencesAndSkyGate(t *testing.T) {
	t.Parallel()
	d := vegetationSource()
	d.Vegetation = nil
	if err := worldsource.Validate(&d); !errors.Is(err, worldsource.ErrReference) {
		t.Fatalf("missing library %v", err)
	}
	d = vegetationSource()
	d.Rooms[0].VegetationLayers[0].Preset = "unknown"
	if err := worldsource.Validate(&d); !errors.Is(err, worldvegetation.ErrIdentity) {
		t.Fatalf("unknown preset %v", err)
	}
	d = vegetationSource()
	d.Rooms[0].VegetationLayers[0].Surface = worldvegetation.SurfaceCeiling
	d.Rooms[0].Sky = true
	if err := worldsource.Validate(&d); !errors.Is(err, worldsource.ErrContent) {
		t.Fatalf("sky ceiling %v", err)
	}
	d = vegetationSource()
	d.Rooms[0].VegetationLayers[0].Surface = ""
	if err := worldsource.Validate(&d); !errors.Is(err, worldvegetation.ErrVegetation) {
		t.Fatalf("implicit surface %v", err)
	}
	d = vegetationSource()
	room := d.Rooms[0]
	room.VegetationLayers = []worldsource.VegetationLayer{{Preset: "missing", Surface: worldvegetation.SurfaceFloor}}
	d.Prefabs = []worldsource.Prefab{{ID: "vegetated", Rooms: []worldsource.Room{room}}}
	if err := worldsource.Validate(&d); !errors.Is(err, worldvegetation.ErrIdentity) {
		t.Fatalf("unused prefab layer skipped: %v", err)
	}
	d = vegetationSource()
	d.Vegetation.Version = 2
	if err := worldsource.Validate(&d); !errors.Is(err, worldsource.ErrVersion) {
		t.Fatalf("resource version %v", err)
	}
}

func TestShrubSourceVersionAndGeometryGate(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		source, vegetation uint16
		geometry           string
		valid              bool
	}{
		{13, 1, "", true}, {14, 1, "", true}, {13, 2, "shrub", false}, {14, 2, "shrub", true},
		{14, 2, "crossed_cards", true}, {14, 2, "", true}, {14, 1, "shrub", false}, {14, 1, "crossed_cards", false}, {14, 2, "tree", false}, {14, 3, "", false},
	} {
		d := vegetationSource()
		d.Version = test.source
		d.Vegetation.Version = test.vegetation
		d.Vegetation.Presets[0].Geometry = test.geometry
		err := worldsource.Validate(&d)
		if (err == nil) != test.valid {
			t.Fatalf("source%d/vegetation%d/%s: %v", test.source, test.vegetation, test.geometry, err)
		}
	}
	d := vegetationSource()
	p := d.Vegetation.Presets[0]
	p.Geometry = "shrub"
	if _, err := worldsource.CompileVegetationPreset(p, map[string]uint32{"grass": 17}); !errors.Is(err, worldsource.ErrVersion) {
		t.Fatal("legacy compiler accepted shrub", err)
	}
	compiled, err := worldsource.CompileVegetationPresetForVersion(p, map[string]uint32{"grass": 17}, worldvegetation.GeometryVersion)
	if err != nil || compiled.Geometry != worldvegetation.GeometryShrub {
		t.Fatal("shrub compiler", compiled, err)
	}
}

func TestNormalMapSourceAndResourceVersionGates(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		source, vegetation uint16
		normal             string
		valid              bool
	}{
		{15, 3, "grass-normal", true}, {15, 3, "", true}, {14, 3, "grass-normal", false}, {14, 3, "", false},
		{15, 2, "grass-normal", false}, {15, 1, "grass-normal", false}, {15, 2, "", true}, {15, 1, "", true}, {15, 3, string([]byte{255}), false},
	} {
		document := vegetationSource()
		document.Version = test.source
		document.Vegetation.Version = test.vegetation
		document.Vegetation.Presets[0].NormalTexture = test.normal
		err := worldsource.Validate(&document)
		if (err == nil) != test.valid {
			t.Fatalf("source%d/vegetation%d/normal%q: %v", test.source, test.vegetation, test.normal, err)
		}
	}
	preset := vegetationSource().Vegetation.Presets[0]
	preset.NormalTexture = "grass-normal"
	if _, err := worldsource.CompileVegetationPresetForVersion(
		preset,
		map[string]uint32{"grass": 17},
		worldvegetation.NormalVersion,
	); !errors.Is(
		err,
		worldsource.ErrReference,
	) {
		t.Fatal("undeclared normal accepted", err)
	}
	compiled, err := worldsource.CompileVegetationPresetForVersion(
		preset,
		map[string]uint32{"grass": 17, "grass-normal": 19},
		worldvegetation.NormalVersion,
	)
	if err != nil || compiled.NormalTexture != 19 {
		t.Fatal("normal texture resolution", compiled, err)
	}
}

func TestTreeSourceAndGeometryVersionGates(t *testing.T) {
	t.Parallel()
	if worldsource.VegetationTreeVersion != 16 {
		t.Fatal("historical tree source gate changed")
	}
	for _, test := range []struct {
		source, vegetation uint16
		geometry           string
		valid              bool
	}{
		{16, 4, "tree", true}, {15, 4, "tree", false}, {16, 3, "tree", false}, {16, 2, "tree", false},
		{16, 4, "shrub", true}, {16, 4, "crossed_cards", true}, {16, 4, "", true}, {16, 4, "conifer", false},
		{16, 3, "shrub", true}, {16, 2, "shrub", true}, {16, 1, "", true},
	} {
		document := vegetationSource()
		document.Version = test.source
		document.Vegetation.Version = test.vegetation
		document.Vegetation.Presets[0].Geometry = test.geometry
		err := worldsource.Validate(&document)
		if (err == nil) != test.valid {
			t.Fatalf("source%d/vegetation%d/%s: %v", test.source, test.vegetation, test.geometry, err)
		}
	}
	preset := vegetationSource().Vegetation.Presets[0]
	preset.Geometry = "tree"
	preset.NormalTexture = "grass-normal"
	compiled, err := worldsource.CompileVegetationPresetForVersion(
		preset,
		map[string]uint32{"grass": 17, "grass-normal": 19},
		worldvegetation.TreeVersion,
	)
	if err != nil || compiled.Geometry != worldvegetation.GeometryTree || compiled.NormalTexture != 19 {
		t.Fatal("tree compilation", compiled, err)
	}
}

func TestPalmSourceAndGeometryVersionGates(t *testing.T) {
	t.Parallel()
	if worldsource.VegetationPalmVersion != 20 || worldsource.CompactRoomVersion != 19 {
		t.Fatal("source gates changed")
	}
	for _, test := range []struct {
		source, vegetation uint16
		geometry           string
		valid              bool
	}{
		{20, 5, "palm", true}, {19, 5, "palm", false}, {18, 5, "palm", false},
		{20, 4, "palm", false}, {20, 3, "palm", false}, {20, 2, "palm", false}, {20, 1, "palm", false},
		{20, 5, "tree", true}, {20, 5, "shrub", true}, {20, 5, "crossed_cards", true}, {20, 5, "", true}, {20, 5, "coconut", false},
		{20, 4, "tree", true}, {19, 4, "tree", true},
	} {
		document := vegetationSource()
		document.Version = test.source
		document.Vegetation.Version = test.vegetation
		document.Vegetation.Presets[0].Geometry = test.geometry
		err := worldsource.Validate(&document)
		if (err == nil) != test.valid {
			t.Fatalf("source%d/vegetation%d/%s: %v", test.source, test.vegetation, test.geometry, err)
		}
	}
	preset := vegetationSource().Vegetation.Presets[0]
	preset.Geometry = "palm"
	preset.NormalTexture = "palm-normal"
	compiled, err := worldsource.CompileVegetationPresetForVersion(
		preset,
		map[string]uint32{"grass": 17, "palm-normal": 19},
		worldvegetation.PalmVersion,
	)
	if err != nil || compiled.Geometry != worldvegetation.GeometryPalm || compiled.NormalTexture != 19 {
		t.Fatal("palm compilation", compiled, err)
	}
	document := vegetationSource()
	document.Version = 19
	document.Vegetation.Version = 5
	document.Rooms[0].VegetationLayers = nil
	if err := worldsource.Validate(&document); !errors.Is(err, worldsource.ErrVersion) {
		t.Fatal("unused v5 library bypassed source gate", err)
	}
}
