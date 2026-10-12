package worldsource_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldsource"
)

func TestEnvironmentSourceVersionAndAbsentCompatibility(t *testing.T) {
	t.Parallel()
	if worldsource.EnvironmentVersion != 9 || worldsource.Version != 11 {
		t.Fatal("source environment version changed")
	}
	for version := uint16(1); version < worldsource.EnvironmentVersion; version++ {
		document := validSource()
		document.Version = version
		if err := worldsource.Validate(&document); err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(document)
		if err != nil || bytes.Contains(encoded, []byte(`"environment"`)) || bytes.Contains(encoded, []byte(`"sky"`)) {
			t.Fatalf("legacy source encoding changed: %v", err)
		}
		document.Environment = &worldsource.Environment{Preset: world.EnvironmentDaylight}
		if err := worldsource.Validate(&document); !errors.Is(err, worldsource.ErrVersion) {
			t.Fatalf("source v%d accepted environment: %v", version, err)
		}
		document.Environment = nil
		document.Prefabs[0].Rooms[0].Sky = true
		if err := worldsource.Validate(&document); !errors.Is(err, worldsource.ErrVersion) {
			t.Fatalf("source v%d accepted prefab sky: %v", version, err)
		}
		document.Prefabs[0].Rooms[0].Sky = false
		document.Rooms[0].Boundary[0].Sky = true
		if err := worldsource.Validate(&document); !errors.Is(err, worldsource.ErrVersion) {
			t.Fatalf("source v%d accepted boundary sky: %v", version, err)
		}
	}
}

func TestEnvironmentSourceRoundTripAndCompleteValidation(t *testing.T) {
	t.Parallel()
	document := validSource()
	document.Environment = &worldsource.Environment{Preset: world.EnvironmentFantasy, Palette: world.EnvironmentPaletteViolet,
		Sun: &world.EnvironmentSun{Azimuth: 230, Elevation: 35}}
	document.Rooms[0].Sky = true
	document.Rooms[0].Boundary[0].Sky = true
	document.Prefabs[0].Rooms[0].Sky = true
	if err := worldsource.Validate(&document); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	var decoded worldsource.Document
	if err := json.Unmarshal(encoded, &decoded); err != nil || !reflect.DeepEqual(document, decoded) {
		t.Fatalf("roundtrip: %v", err)
	}
	for name, test := range map[string]struct {
		mutate func(*worldsource.Document)
		want   error
	}{
		"missing environment":       {func(d *worldsource.Document) { d.Environment = nil }, worldsource.ErrEnvironment},
		"unknown preset":            {func(d *worldsource.Document) { d.Environment.Preset = "bad" }, worldsource.ErrEnvironment},
		"unknown palette":           {func(d *worldsource.Document) { d.Environment.Palette = "bad" }, worldsource.ErrEnvironment},
		"azimuth nan":               {func(d *worldsource.Document) { d.Environment.Sun.Azimuth = math.NaN() }, worldsource.ErrEnvironment},
		"elevation inf":             {func(d *worldsource.Document) { d.Environment.Sun.Elevation = math.Inf(1) }, worldsource.ErrEnvironment},
		"azimuth above bound":       {func(d *worldsource.Document) { d.Environment.Sun.Azimuth = 360 }, worldsource.ErrEnvironment},
		"elevation below bound":     {func(d *worldsource.Document) { d.Environment.Sun.Elevation = -1 }, worldsource.ErrEnvironment},
		"boundary in indoor room":   {func(d *worldsource.Document) { d.Rooms[0].Sky = false }, worldsource.ErrEnvironment},
		"connected sky edge":        {func(d *worldsource.Document) { d.Rooms[0].Boundary[1].Sky = true }, worldsource.ErrConnection},
		"connected prefab sky edge": {func(d *worldsource.Document) { d.Prefabs[0].Rooms[0].Boundary[3].Sky = true }, worldsource.ErrConnection},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var invalid worldsource.Document
			if err := json.Unmarshal(encoded, &invalid); err != nil {
				t.Fatal(err)
			}
			test.mutate(&invalid)
			if err := worldsource.Validate(&invalid); !errors.Is(err, test.want) {
				t.Fatalf("validation error=%v want=%v", err, test.want)
			}
		})
	}
	// Resolve through another prefab's exported port, rather than only checking
	// the direct room endpoint in the current scope.
	inner := decoded.Prefabs[0]
	inner.Rooms[0].Boundary[3].Sky = true
	decoded.Prefabs = []worldsource.Prefab{inner, {
		ID: "wrapper", Instances: []worldsource.Instance{{ID: "inner", Prefab: "alcove"}},
		Ports: []worldsource.Port{{ID: "entrance", Endpoint: worldsource.Endpoint{Instance: "inner", Port: "entrance"}}},
	}}
	decoded.Instances[0].Prefab = "wrapper"
	if err := worldsource.Validate(&decoded); !errors.Is(err, worldsource.ErrConnection) {
		t.Fatalf("nested prefab sky edge accepted: %v", err)
	}
}
