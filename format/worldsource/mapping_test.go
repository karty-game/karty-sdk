package worldsource_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/karty-game/karty-sdk/format/worldsource"
)

func TestSourceUVLegacyDefaultsAndScopeVersionGates(t *testing.T) {
	t.Parallel()
	scopes := map[string]func(*worldsource.Document) **worldsource.UVSettings{
		"root":    func(d *worldsource.Document) **worldsource.UVSettings { return &d.UV },
		"floor":   func(d *worldsource.Document) **worldsource.UVSettings { return &d.Rooms[0].FloorUV },
		"ceiling": func(d *worldsource.Document) **worldsource.UVSettings { return &d.Rooms[0].CeilingUV },
		"wall":    func(d *worldsource.Document) **worldsource.UVSettings { return &d.Rooms[0].WallUV },
		"edge": func(d *worldsource.Document) **worldsource.UVSettings {
			return &d.Rooms[0].Boundary[len(d.Rooms[0].Boundary)-1].UV
		},
		"prefab floor":   func(d *worldsource.Document) **worldsource.UVSettings { return &d.Prefabs[0].Rooms[0].FloorUV },
		"prefab ceiling": func(d *worldsource.Document) **worldsource.UVSettings { return &d.Prefabs[0].Rooms[0].CeilingUV },
		"prefab wall":    func(d *worldsource.Document) **worldsource.UVSettings { return &d.Prefabs[0].Rooms[0].WallUV },
		"prefab edge": func(d *worldsource.Document) **worldsource.UVSettings {
			return &d.Prefabs[0].Rooms[0].Boundary[len(d.Prefabs[0].Rooms[0].Boundary)-1].UV
		},
	}
	if worldsource.Version != 7 || worldsource.MappingVersion != 5 || worldsource.LightingVersion != 4 {
		t.Fatal("source version/capability gate changed")
	}
	for _, version := range []uint16{1, 2, 3, 4, 5, 6} {
		document := validSource()
		document.Version = version
		if err := worldsource.Validate(&document); err != nil {
			t.Fatalf("absent UV broke source%d: %v", version, err)
		}
		encoded, err := json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{`"uv"`, `"floor_uv"`, `"ceiling_uv"`, `"wall_uv"`} {
			if bytes.Contains(encoded, []byte(field)) {
				t.Fatalf("absent UV changed source%d wire", version)
			}
		}
		for name, scope := range scopes {
			t.Run(fmt.Sprintf("%d/%s", version, name), func(t *testing.T) {
				t.Parallel()
				document := validSource()
				document.Version = version
				*scope(&document) = &worldsource.UVSettings{}
				err := worldsource.Validate(&document)
				if version < 5 {
					if !errors.Is(err, worldsource.ErrVersion) {
						t.Fatalf("old source accepts %s: %v", name, err)
					}
				} else if err != nil {
					t.Fatal(err)
				}
			})
		}
	}
	// Typed validation preserves inheritance and does not invent author fields.
	document := validSource()
	document.UV = &worldsource.UVSettings{}
	before, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := worldsource.Validate(&document); err != nil {
		t.Fatal(err)
	}
	after, err := json.Marshal(document)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("validation mutated inherited defaults")
	}
}

func TestSourceUVPointerOverridesAndFiniteBounds(t *testing.T) {
	t.Parallel()
	zero := 0.0
	settings := &worldsource.UVSettings{
		Mode:            worldsource.UVWrap,
		Anchor:          worldsource.UVTop,
		Scale:           &worldsource.Vec2{X: .001, Y: 1e6},
		Offset:          &worldsource.Vec2{X: -1e6, Y: 1e6},
		RotationDegrees: &zero,
	}
	encoded, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(encoded, []byte(`"rotation_degrees":0`)) {
		t.Fatal("explicit zero rotation override was omitted")
	}
	var decoded worldsource.UVSettings
	if err := json.Unmarshal(encoded, &decoded); err != nil || !reflect.DeepEqual(settings, &decoded) {
		t.Fatalf("pointer controls changed: %v", err)
	}
	if err := worldsource.ValidateUVSettings(settings); err != nil {
		t.Fatal(err)
	}
	if err := worldsource.ValidateUVSettings(nil); !errors.Is(err, worldsource.ErrUVMapping) {
		t.Fatal("nil controls accepted")
	}
	fields := map[string]struct {
		get     func(*worldsource.UVSettings) *float64
		valid   []float64
		invalid []float64
	}{
		"scale x": {
			func(s *worldsource.UVSettings) *float64 { return &s.Scale.X },
			[]float64{.001, 1e6},
			[]float64{0, -1, math.Nextafter(.001, 0), 1e6 + 1},
		},
		"scale y": {
			func(s *worldsource.UVSettings) *float64 { return &s.Scale.Y },
			[]float64{.001, 1e6},
			[]float64{0, -1, math.Nextafter(.001, 0), 1e6 + 1},
		},
		"offset x": {
			func(s *worldsource.UVSettings) *float64 { return &s.Offset.X },
			[]float64{-1e6, 0, 1e6},
			[]float64{-1e6 - 1, 1e6 + 1},
		},
		"offset y": {
			func(s *worldsource.UVSettings) *float64 { return &s.Offset.Y },
			[]float64{-1e6, 0, 1e6},
			[]float64{-1e6 - 1, 1e6 + 1},
		},
		"rotation": {
			func(s *worldsource.UVSettings) *float64 { return s.RotationDegrees },
			[]float64{-1e6, 0, 1e6},
			[]float64{-1e6 - 1, 1e6 + 1},
		},
	}
	for name, field := range fields {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for _, value := range append(append([]float64{}, field.valid...), append(field.invalid, math.NaN(), math.Inf(-1), math.Inf(1))...) {
				rotation := 0.0
				candidate := &worldsource.UVSettings{
					Scale:           &worldsource.Vec2{X: 1, Y: 1},
					Offset:          &worldsource.Vec2{},
					RotationDegrees: &rotation,
				}
				*field.get(candidate) = value
				document := validSource()
				last := &document.Prefabs[0].Rooms[0].Boundary[len(document.Prefabs[0].Rooms[0].Boundary)-1]
				last.UV = candidate
				valid := false
				for _, allowed := range field.valid {
					if value == allowed {
						valid = true
					}
				}
				err := worldsource.Validate(&document)
				if valid {
					if err != nil {
						t.Fatalf("inclusive %s=%v: %v", name, value, err)
					}
				} else if !errors.Is(err, worldsource.ErrUVMapping) {
					t.Fatalf("invalid last edge %s=%v accepted: %v", name, value, err)
				}
			}
		})
	}
}

func TestSourceUVModeAnchorContext(t *testing.T) {
	t.Parallel()
	for _, mode := range []worldsource.UVMode{"", worldsource.UVTriplanar, worldsource.UVPlanar, worldsource.UVWrap, "cylindrical"} {
		for _, anchor := range []worldsource.UVAnchor{"", worldsource.UVWorld, worldsource.UVTop, worldsource.UVBottom, "middle"} {
			t.Run(fmt.Sprintf("%s/%s", mode, anchor), func(t *testing.T) {
				t.Parallel()
				settings := &worldsource.UVSettings{Mode: mode, Anchor: anchor}
				known := mode != "cylindrical" && anchor != "middle"
				err := worldsource.ValidateUVSettings(settings)
				if known {
					if err != nil {
						t.Fatal(err)
					}
				} else if !errors.Is(err, worldsource.ErrUVMapping) {
					t.Fatalf("unknown settings accepted: %v", err)
				}
				for _, horizontal := range []bool{false, true} {
					document := validSource()
					if horizontal {
						document.Rooms[0].FloorUV = settings
						document.Rooms[0].CeilingUV = settings
					} else {
						document.UV = settings
						document.Rooms[0].WallUV = settings
						document.Rooms[0].Boundary[0].UV = settings
					}
					allowed := known &&
						(!horizontal || mode != worldsource.UVWrap && anchor != worldsource.UVTop && anchor != worldsource.UVBottom)
					err := worldsource.Validate(&document)
					if allowed {
						if err != nil {
							t.Fatalf("valid context rejected: %v", err)
						}
					} else if !errors.Is(err, worldsource.ErrUVMapping) {
						t.Fatalf("invalid context accepted: %v", err)
					}
				}
			})
		}
	}
}
