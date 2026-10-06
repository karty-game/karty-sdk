package worldsource_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldsource"
)

func TestLightingSourceVersionAndAbsentCompatibility(t *testing.T) {
	t.Parallel()
	if worldsource.Version != 6 || worldsource.LightingVersion != 4 || worldsource.MaxLights != 50 {
		t.Fatal("static lighting source version or bound changed")
	}
	for _, version := range []uint16{1, 2, 3, 4, 5, 6} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			t.Parallel()
			document := validSource()
			document.Version = version
			if err := worldsource.Validate(&document); err != nil {
				t.Fatalf("source v%d without lighting: %v", version, err)
			}
			legacy := struct {
				Version     uint16                   `json:"version"`
				Rooms       []worldsource.Room       `json:"rooms"`
				Prefabs     []worldsource.Prefab     `json:"prefabs"`
				Instances   []worldsource.Instance   `json:"instances"`
				Connections []worldsource.Connection `json:"connections"`
			}{document.Version, document.Rooms, document.Prefabs, document.Instances, document.Connections}
			want, err := json.Marshal(legacy)
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(document)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(encoded, want) || bytes.Contains(encoded, []byte(`"lighting"`)) {
				t.Fatalf("absent lighting changed source encoding: %s", encoded)
			}
			document.Lighting = validSourceLighting()
			err = worldsource.Validate(&document)
			if version < 4 && !errors.Is(err, worldsource.ErrVersion) {
				t.Fatalf("source v%d accepted lighting: %v", version, err)
			}
			if version >= 4 && err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLightingSourceBoundsAndGlobalOrder(t *testing.T) {
	t.Parallel()
	for _, count := range []int{0, 1, 50, 51} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			t.Parallel()
			document := validSource()
			document.Lighting = validSourceLighting()
			document.Lighting.Lights = make([]worldsource.PointLight, count)
			for index := range document.Lighting.Lights {
				document.Lighting.Lights[index] = worldsource.PointLight{
					ID:       fmt.Sprintf("light-%02d", count-index),
					Position: worldsource.Vec3{X: -worldsource.MaxCoordinate, Y: worldsource.MaxCoordinate, Z: -worldsource.MaxCoordinate},
					Color:    worldsource.Vec3{X: 1, Z: 1}, Radius: worldsource.MinLightRadius,
				}
			}
			before, err := json.Marshal(document.Lighting)
			if err != nil {
				t.Fatal(err)
			}
			err = worldsource.Validate(&document)
			if count > worldsource.MaxLights {
				if !errors.Is(err, worldsource.ErrBounds) {
					t.Fatalf("51 lights: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			after, err := json.Marshal(document.Lighting)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("validation changed global positions or authored order")
			}
			encoded, err := json.Marshal(document)
			if err != nil {
				t.Fatal(err)
			}
			var decoded worldsource.Document
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				t.Fatal(err)
			}
			if err := worldsource.Validate(&decoded); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(decoded.Lighting, document.Lighting) {
				t.Fatal("source lighting round trip changed")
			}
		})
	}
	document := validSource()
	document.Lighting = validSourceLighting()
	document.Lighting.Lights[0].ID = strings.Repeat("x", worldsource.MaxIdentifierBytes)
	document.Lighting.Lights[0].Radius = worldsource.MaxCoordinate
	if err := worldsource.Validate(&document); err != nil {
		t.Fatalf("inclusive maxima: %v", err)
	}
	document.Lighting.Lights = nil
	if err := worldsource.Validate(&document); err != nil {
		t.Fatalf("ambient-only lighting: %v", err)
	}
}

func TestLightingSourceRejectsInvalidValues(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		mutate func(*worldsource.Lighting)
		want   error
	}{
		"zero version":   {func(l *worldsource.Lighting) { l.Version = 0 }, worldsource.ErrVersion},
		"future version": {func(l *worldsource.Lighting) { l.Version = 2 }, worldsource.ErrVersion},
		"empty ID":       {func(l *worldsource.Lighting) { l.Lights[0].ID = "" }, worldsource.ErrIdentity},
		"duplicate ID":   {func(l *worldsource.Lighting) { l.Lights = append(l.Lights, l.Lights[0]) }, worldsource.ErrIdentity},
		"long ID": {
			func(l *worldsource.Lighting) { l.Lights[0].ID = strings.Repeat("x", worldsource.MaxIdentifierBytes+1) },
			worldsource.ErrIdentity,
		},
		"invalid UTF-8 ID": {func(l *worldsource.Lighting) { l.Lights[0].ID = string([]byte{0xff}) }, worldsource.ErrIdentity},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			document := validSource()
			document.Lighting = validSourceLighting()
			test.mutate(document.Lighting)
			if err := worldsource.Validate(&document); !errors.Is(err, test.want) {
				t.Fatalf("Validate() = %v, want %v", err, test.want)
			}
		})
	}
	fields := map[string]struct {
		value func(*worldsource.Lighting) *float64
		bad   []float64
	}{
		"ambient x": {func(l *worldsource.Lighting) *float64 { return &l.Ambient.X }, []float64{-0.01, 1.01}},
		"ambient y": {func(l *worldsource.Lighting) *float64 { return &l.Ambient.Y }, []float64{-0.01, 1.01}},
		"ambient z": {func(l *worldsource.Lighting) *float64 { return &l.Ambient.Z }, []float64{-0.01, 1.01}},
		"color x":   {func(l *worldsource.Lighting) *float64 { return &l.Lights[0].Color.X }, []float64{-0.01, 1.01}},
		"color y":   {func(l *worldsource.Lighting) *float64 { return &l.Lights[0].Color.Y }, []float64{-0.01, 1.01}},
		"color z":   {func(l *worldsource.Lighting) *float64 { return &l.Lights[0].Color.Z }, []float64{-0.01, 1.01}},
		"position x": {
			func(l *worldsource.Lighting) *float64 { return &l.Lights[0].Position.X },
			[]float64{-worldsource.MaxCoordinate - 1, worldsource.MaxCoordinate + 1},
		},
		"position y": {
			func(l *worldsource.Lighting) *float64 { return &l.Lights[0].Position.Y },
			[]float64{-worldsource.MaxCoordinate - 1, worldsource.MaxCoordinate + 1},
		},
		"position z": {
			func(l *worldsource.Lighting) *float64 { return &l.Lights[0].Position.Z },
			[]float64{-worldsource.MaxCoordinate - 1, worldsource.MaxCoordinate + 1},
		},
		"radius": {
			func(l *worldsource.Lighting) *float64 { return &l.Lights[0].Radius },
			[]float64{0, -1, math.SmallestNonzeroFloat64, math.Nextafter(worldsource.MinLightRadius, 0), worldsource.MaxCoordinate + 1},
		},
	}
	for name, field := range fields {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for _, value := range append(field.bad, math.NaN(), math.Inf(-1), math.Inf(1)) {
				document := validSource()
				document.Lighting = validSourceLighting()
				*field.value(document.Lighting) = value
				if err := worldsource.Validate(&document); !errors.Is(err, worldsource.ErrLighting) {
					t.Fatalf("%s=%v: %v", name, value, err)
				}
			}
		})
	}
}

func TestLightingSourceHasNoPrefabAuthoring(t *testing.T) {
	t.Parallel()
	for _, scope := range []any{worldsource.Prefab{}, worldsource.Room{}, worldsource.Instance{}} {
		if _, ok := reflect.TypeOf(scope).FieldByName("Lighting"); ok {
			t.Fatalf("%T permits local lighting", scope)
		}
		decoder := json.NewDecoder(strings.NewReader(`{"lighting":{"version":1,"ambient":{"x":0,"y":0,"z":0},"lights":[]}}`))
		decoder.DisallowUnknownFields()
		value := reflect.New(reflect.TypeOf(scope)).Interface()
		if err := decoder.Decode(value); err == nil {
			t.Fatalf("strict decoder accepted lighting in %T", scope)
		}
	}
}

func validSourceLighting() *worldsource.Lighting {
	return &worldsource.Lighting{
		Version: world.LightingVersion,
		Ambient: worldsource.Vec3{X: .1, Y: .2, Z: .3},
		Lights: []worldsource.PointLight{
			{ID: "lamp", Position: worldsource.Vec3{X: 2, Y: 3, Z: 4}, Color: worldsource.Vec3{X: 1, Y: .5}, Radius: 8},
		},
	}
}

func TestLightingDirectionalAmbientAndActorsContract(t *testing.T) {
	t.Parallel()
	lighting := validSourceLighting()
	lighting.Actors = true
	lighting.AmbientCube = &worldsource.AmbientCube{
		PositiveX: worldsource.Vec3{X: 1}, NegativeX: worldsource.Vec3{Y: 1},
		PositiveY: worldsource.Vec3{Z: 1}, NegativeY: worldsource.Vec3{X: 1, Y: 1},
		PositiveZ: worldsource.Vec3{Y: 1, Z: 1}, NegativeZ: worldsource.Vec3{X: 1, Z: 1},
	}
	encoded, err := json.Marshal(lighting)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(encoded, []byte(`"actors":true`)) || !bytes.Contains(encoded, []byte(`"negative_z":{"x":1,"y":0,"z":1}`)) {
		t.Fatalf("optional contract fields missing: %s", encoded)
	}
	var decoded worldsource.Lighting
	if err := json.Unmarshal(encoded, &decoded); err != nil || !reflect.DeepEqual(lighting, &decoded) {
		t.Fatalf("contract roundtrip %v", err)
	}
	document := validSource()
	document.Lighting = lighting
	if err := worldsource.Validate(&document); err != nil {
		t.Fatal(err)
	}
	faces := []*worldsource.Vec3{
		&lighting.AmbientCube.PositiveX,
		&lighting.AmbientCube.NegativeX,
		&lighting.AmbientCube.PositiveY,
		&lighting.AmbientCube.NegativeY,
		&lighting.AmbientCube.PositiveZ,
		&lighting.AmbientCube.NegativeZ,
	}
	for faceIndex, face := range faces {
		for componentIndex, component := range []*float64{&face.X, &face.Y, &face.Z} {
			original := *component
			for _, value := range []float64{-0.001, 1.001, math.NaN(), math.Inf(-1), math.Inf(1)} {
				*component = value
				if err := worldsource.Validate(&document); !errors.Is(err, worldsource.ErrLighting) {
					t.Fatalf("face%d component%d value%v: %v", faceIndex, componentIndex, value, err)
				}
			}
			*component = original
		}
	}
}
