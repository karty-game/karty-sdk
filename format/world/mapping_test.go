package world_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
)

func validSurfaceUV(count int) *world.SurfaceUV {
	mapping := &world.SurfaceUV{Projections: make([]world.UVProjection, count), Weights: make([]float64, count)}
	for index := range count {
		mapping.Projections[index] = world.UVProjection{U: world.UVPlane{X: 1, Offset: float64(index)}, V: world.UVPlane{Y: 1, Offset: -float64(index)}}
		mapping.Weights[index] = 1 / float64(count)
	}
	return mapping
}

func mappedWorld() world.Document {
	document := lightingWorld()
	document.MaterialMapping = &world.MaterialMapping{Version: world.MaterialMappingVersion}
	for i := range document.Sectors {
		document.Sectors[i].FloorUV = validSurfaceUV(3)
		document.Sectors[i].CeilingUV = validSurfaceUV(1)
		for j := range document.Sectors[i].Walls {
			document.Sectors[i].Walls[j].UV = validSurfaceUV(3)
		}
	}
	return document
}

func TestMaterialMappingCanonicalRoundTripAndOwnership(t *testing.T) {
	t.Parallel()
	document := mappedWorld()
	encoded, err := world.Encode(document)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(encoded, []byte(`"material_mapping":{"version":1}`)) || !bytes.Contains(encoded, []byte(`"floor_uv":{"projections":[{"u":{"x":1,"y":0,"z":0,"offset":0}`)) {
		t.Fatalf("canonical mapping fields missing: %s", encoded)
	}
	decoded, err := world.Decode(encoded)
	if err != nil || !reflect.DeepEqual(decoded, document) {
		t.Fatalf("roundtrip %+v: %v", decoded, err)
	}
	decoded.Sectors[0].FloorUV.Projections[2].V.Offset = 123
	decoded.Sectors[0].FloorUV.Weights[0] = 0
	if document.Sectors[0].FloorUV.Projections[2].V.Offset != -2 || document.Sectors[0].FloorUV.Weights[0] != 1.0/3 {
		t.Fatal("decoded mappings alias authored storage")
	}
}

func TestMaterialMappingLegacyWireAndVersionGates(t *testing.T) {
	t.Parallel()
	for _, version := range []uint16{1, 2, 3} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			t.Parallel()
			document := validWorld()
			if version == 3 {
				document = lightingWorld()
			}
			document.Version = version
			before, err := json.Marshal(document)
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := world.Encode(document)
			if err != nil || !bytes.Equal(encoded, before) || bytes.Contains(encoded, []byte(`"material_mapping"`)) || bytes.Contains(encoded, []byte(`"_uv"`)) {
				t.Fatalf("legacy wire changed %v", err)
			}
			decoded, err := world.Decode(encoded)
			if err != nil || !reflect.DeepEqual(decoded, document) {
				t.Fatalf("legacy decode changed %v", err)
			}
			document.MaterialMapping = &world.MaterialMapping{Version: 1}
			if version < 3 {
				if err := world.Validate(&document); !errors.Is(err, world.ErrVersion) {
					t.Fatalf("old version accepts marker: %v", err)
				}
			} else if err := world.Validate(&document); !errors.Is(err, world.ErrMaterialMapping) {
				t.Fatalf("marker accepted incomplete surface list: %v", err)
			}
			document.MaterialMapping = nil
			document.Sectors[0].FloorUV = validSurfaceUV(1)
			want := world.ErrMaterialMapping
			if version < 3 {
				want = world.ErrVersion
			}
			if err := world.Validate(&document); !errors.Is(err, want) {
				t.Fatalf("undeclared mapping error %v", err)
			}
		})
	}
}

func TestSurfaceUVCompleteShapeAndWeights(t *testing.T) {
	t.Parallel()
	if err := world.ValidateSurfaceUV(nil); !errors.Is(err, world.ErrMaterialMapping) {
		t.Fatal("nil mapping accepted")
	}
	for _, count := range []int{0, 1, 2, 3, 4, 50} {
		mapping := validSurfaceUV(count)
		err := world.ValidateSurfaceUV(mapping)
		if count == 1 || count == 3 {
			if err != nil {
				t.Fatal(err)
			}
		} else if !errors.Is(err, world.ErrMaterialMapping) {
			t.Fatalf("count%d accepted", count)
		}
	}
	mutations := map[string]func(*world.SurfaceUV){
		"missing weights":   func(s *world.SurfaceUV) { s.Weights = nil },
		"extra weight":      func(s *world.SurfaceUV) { s.Weights = append(s.Weights, 0) },
		"negative":          func(s *world.SurfaceUV) { s.Weights[2] = -.01 },
		"greater than one":  func(s *world.SurfaceUV) { s.Weights[2] = 1.01 },
		"NaN":               func(s *world.SurfaceUV) { s.Weights[2] = math.NaN() },
		"positive infinity": func(s *world.SurfaceUV) { s.Weights[2] = math.Inf(1) },
		"negative infinity": func(s *world.SurfaceUV) { s.Weights[2] = math.Inf(-1) },
		"all zero":          func(s *world.SurfaceUV) { clear(s.Weights) },
		"wrong sum":         func(s *world.SurfaceUV) { s.Weights[2] += .0001 },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			mapping := validSurfaceUV(3)
			mutate(mapping)
			if err := world.ValidateSurfaceUV(mapping); !errors.Is(err, world.ErrMaterialMapping) {
				t.Fatalf("invalid shape accepted: %v", err)
			}
		})
	}
	mapping := validSurfaceUV(3)
	mapping.Weights = []float64{0, 0, 1}
	if err := world.ValidateSurfaceUV(mapping); err != nil {
		t.Fatalf("zero unused projection weights rejected: %v", err)
	}
}

func TestSurfaceUVFiniteInclusiveProjectionBounds(t *testing.T) {
	t.Parallel()
	for projection := range 3 {
		for axis := range 2 {
			for component := range 4 {
				t.Run(fmt.Sprintf("%d/%d/%d", projection, axis, component), func(t *testing.T) {
					t.Parallel()
					for _, value := range []float64{-world.MaxUVProjectionValue, world.MaxUVProjectionValue, math.Nextafter(-world.MaxUVProjectionValue, math.Inf(-1)), math.Nextafter(world.MaxUVProjectionValue, math.Inf(1)), math.NaN(), math.Inf(-1), math.Inf(1)} {
						document := mappedWorld()
						mapping := document.Sectors[len(document.Sectors)-1].Walls[len(document.Sectors[len(document.Sectors)-1].Walls)-1].UV
						plane := &mapping.Projections[projection].U
						if axis == 1 {
							plane = &mapping.Projections[projection].V
						}
						fields := [4]*float64{&plane.X, &plane.Y, &plane.Z, &plane.Offset}
						*fields[component] = value
						err := world.Validate(&document)
						if math.Abs(value) <= world.MaxUVProjectionValue {
							if err != nil {
								t.Fatal(err)
							}
						} else {
							if !errors.Is(err, world.ErrMaterialMapping) {
								t.Fatalf("invalid last projection accepted: %v", err)
							}
							if _, err := world.Encode(document); !errors.Is(err, world.ErrMaterialMapping) {
								t.Fatalf("invalid mapping encoded: %v", err)
							}
						}
					}
				})
			}
		}
	}
}

func TestMaterialMappingRejectsEveryIncompleteSurface(t *testing.T) {
	t.Parallel()
	for i := range mappedWorld().Sectors {
		for surface := -2; surface < len(mappedWorld().Sectors[i].Walls); surface++ {
			document := mappedWorld()
			switch surface {
			case -2:
				document.Sectors[i].FloorUV = nil
			case -1:
				document.Sectors[i].CeilingUV = nil
			default:
				document.Sectors[i].Walls[surface].UV = nil
			}
			if err := world.Validate(&document); !errors.Is(err, world.ErrMaterialMapping) {
				t.Fatalf("missing sector%d surface%d: %v", i, surface, err)
			}
		}
	}
	for _, version := range []uint32{0, 2} {
		document := mappedWorld()
		document.MaterialMapping.Version = version
		if err := world.Validate(&document); !errors.Is(err, world.ErrVersion) {
			t.Fatalf("nestedversion%d: %v", version, err)
		}
	}
}

func TestMaterialMappingStrictCanonicalDecode(t *testing.T) {
	t.Parallel()
	encoded, err := world.Encode(mappedWorld())
	if err != nil {
		t.Fatal(err)
	}
	for name, pair := range map[string][2]string{
		"unknown marker":     {`"material_mapping":{"version":1}`, `"material_mapping":{"version":1,"mode":"planar"}`},
		"duplicate marker":   {`"material_mapping":{"version":1}`, `"material_mapping":{"version":1},"material_mapping":{"version":1}`},
		"unknown projection": {`"u":{"x":1`, `"extra":0,"u":{"x":1`},
		"missing coordinate": {`"u":{"x":1,"y":0,"z":0,"offset":0}`, `"u":{"x":1,"y":0,"offset":0}`},
		"missing plane":      {`"u":{"x":1,"y":0,"z":0,"offset":0},`, ``},
		"null plane":         {`"u":{"x":1,"y":0,"z":0,"offset":0}`, `"u":null`},
		"duplicate weights":  {`"weights":[1]`, `"weights":[1],"weights":[1]`},
		"wrong version":      {`"material_mapping":{"version":1}`, `"material_mapping":{"version":2}`},
		"null marker":        {`"material_mapping":{"version":1}`, `"material_mapping":null`},
		"number spelling":    {`"offset":0`, `"offset":0.0`},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			mutated := bytes.Replace(encoded, []byte(pair[0]), []byte(pair[1]), 1)
			if bytes.Equal(mutated, encoded) {
				t.Fatal("fixture did not mutate")
			}
			decoded, err := world.Decode(mutated)
			if err == nil || !reflect.DeepEqual(decoded, world.Document{}) {
				t.Fatalf("malformed mapping produced partial output: %v", err)
			}
		})
	}
	for offset := range len(encoded) {
		if _, err := world.Decode(encoded[:offset]); err == nil {
			t.Fatalf("truncated mapping accepted at%d", offset)
		}
	}
}
