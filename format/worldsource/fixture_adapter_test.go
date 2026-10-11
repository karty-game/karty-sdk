package worldsource_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/karty-game/karty-sdk/format/worldsource"
)

// The SDK has no authoring parser dependency. Its fixture is a JSON-compatible
// YAML subset, so this test-only adapter maps YAML snake_case keys onto typed Go
// names. It does not add JSON tags or a runtime source-format decoder.
func normalizeFixtureKeys(t *testing.T, encoded []byte) []byte {
	t.Helper()
	var value any
	if err := json.Unmarshal(encoded, &value); err != nil {
		t.Fatal(err)
	}
	var normalize func(any) any
	normalize = func(v any) any {
		switch v := v.(type) {
		case map[string]any:
			out := make(map[string]any, len(v))
			for key, child := range v {
				out[strings.ReplaceAll(key, "_", "")] = normalize(child)
			}
			return out
		case []any:
			for i, child := range v {
				v[i] = normalize(child)
			}
			return v
		default:
			return v
		}
	}
	result, err := json.Marshal(normalize(value))
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestAuthoringYAMLVocabularySurvivesRuntimeTagRemoval(t *testing.T) {
	for _, field := range []struct {
		typ       reflect.Type
		name, tag string
	}{
		{reflect.TypeFor[worldsource.Room](), "FloorMaterial", "floor_material"},
		{reflect.TypeFor[worldsource.Lighting](), "AmbientCube", "ambient_cube,omitempty"},
		{reflect.TypeFor[worldsource.AmbientCube](), "NegativeZ", "negative_z"},
		{reflect.TypeFor[worldsource.UVSettings](), "RotationDegrees", "rotation_degrees,omitempty"},
		{reflect.TypeFor[worldsource.Environment](), "TimeOfDay", "time_of_day,omitempty"},
	} {
		f, ok := field.typ.FieldByName(field.name)
		if !ok || f.Tag.Get("yaml") != field.tag {
			t.Fatalf("%s.%s YAML vocabulary changed", field.typ, field.name)
		}
	}
}
