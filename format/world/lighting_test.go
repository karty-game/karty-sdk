package world_test

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
)

func TestLightingCanonicalRoundTrip(t *testing.T) {
	t.Parallel()

	document := lightingWorld()
	document.Lighting = validLighting()
	encoded, err := world.Encode(document)
	if err != nil {
		t.Fatal(err)
	}
	want := `,"lighting":{"version":1,"ambient":{"x":0.1,"y":0.2,"z":0.3},"lights":[{"id":"lamp","position":{"x":2,"y":3,"z":4},"color":{"x":1,"y":0.5,"z":0},"radius":8}]}}`
	if !bytes.HasSuffix(encoded, []byte(want)) {
		t.Fatalf("lighting encoding changed: %s", encoded)
	}
	decoded, err := world.Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, document) {
		t.Fatalf("round trip changed document: %+v", decoded)
	}
}

func TestLightingAbsentPreservesAllCompiledVersions(t *testing.T) {
	t.Parallel()

	if world.Version != 3 || world.LightingVersion != 1 || world.MaxLights != 50 {
		t.Fatal("world/lighting@1 changed its version or bounds")
	}
	for _, version := range []uint16{1, 2, 3} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			t.Parallel()
			document := validWorld()
			if version == world.Version {
				document = lightingWorld()
			}
			document.Version = version
			// This is the complete pre-lighting public layout, in wire order.
			legacy := struct {
				Version  uint16          `json:"version"`
				Sectors  []world.Sector  `json:"sectors"`
				Contents []world.Content `json:"contents"`
			}{document.Version, document.Sectors, document.Contents}
			want, err := json.Marshal(legacy)
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := world.Encode(document)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(encoded, want) || bytes.Contains(encoded, []byte(`"lighting"`)) {
				t.Fatalf("legacy v%d wire changed: %s", version, encoded)
			}
			decoded, err := world.Decode(want)
			if err != nil || decoded.Lighting != nil {
				t.Fatalf("legacy decode changed: lighting=%+v err=%v", decoded.Lighting, err)
			}
			if version < world.Version {
				document.Lighting = validLighting()
				if err := world.Validate(&document); !errors.Is(err, world.ErrVersion) {
					t.Fatalf("v%d accepted lighting: %v", version, err)
				}
				payload, err := json.Marshal(document)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := world.Decode(payload); !errors.Is(err, world.ErrVersion) {
					t.Fatalf("v%d decoded lighting: %v", version, err)
				}
			}
		})
	}
}

func TestLightingBoundsOrderAndGlobalCoordinates(t *testing.T) {
	t.Parallel()

	for _, count := range []int{0, 1, 49, 50, 51} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			t.Parallel()
			document := lightingWorld()
			document.Lighting = &world.Lighting{
				Version: world.LightingVersion, Ambient: world.Vec3{X: 1},
				Lights: make([]world.PointLight, count),
			}
			for index := range document.Lighting.Lights {
				document.Lighting.Lights[index] = world.PointLight{
					ID:       fmt.Sprintf("lamp-%02d", count-index),
					Position: world.Vec3{X: -world.MaxCoordinate, Y: world.MaxCoordinate, Z: -world.MaxCoordinate},
					Color:    world.Vec3{X: 1, Z: 1}, Radius: world.MaxCoordinate,
				}
			}
			if count > world.MaxLights {
				if err := world.Validate(&document); !errors.Is(err, world.ErrBounds) {
					t.Fatalf("count %d: %v", count, err)
				}
				payload, err := json.Marshal(document)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := world.Decode(payload); !errors.Is(err, world.ErrBounds) {
					t.Fatalf("decoded count %d: %v", count, err)
				}
				return
			}
			encoded, err := world.Encode(document)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := world.Decode(encoded)
			if err != nil || !reflect.DeepEqual(decoded.Lighting, document.Lighting) {
				t.Fatalf("global lights or authored order changed: %+v, %v", decoded.Lighting, err)
			}
		})
	}
	lighting := validLighting()
	lighting.Lights = nil
	if err := world.ValidateLighting(lighting); err != nil {
		t.Fatalf("ambient-only lighting: %v", err)
	}
	lighting.Lights = []world.PointLight{{ID: strings.Repeat("x", world.MaxIdentifierBytes), Radius: world.MinLightRadius}}
	if err := world.ValidateLighting(lighting); err != nil {
		t.Fatalf("inclusive identifier and minimum radius bounds: %v", err)
	}
}

func TestLightingRejectsInvalidPayloads(t *testing.T) {
	t.Parallel()

	if err := world.ValidateLighting(nil); !errors.Is(err, world.ErrLighting) {
		t.Fatalf("nil lighting error = %v", err)
	}
	tests := map[string]struct {
		mutate func(*world.Lighting)
		want   error
	}{
		"missing version": {func(l *world.Lighting) { l.Version = 0 }, world.ErrVersion},
		"future version":  {func(l *world.Lighting) { l.Version = 2 }, world.ErrVersion},
		"empty ID":        {func(l *world.Lighting) { l.Lights[0].ID = "" }, world.ErrIdentity},
		"long ID": {func(l *world.Lighting) {
			l.Lights[0].ID = strings.Repeat("x", world.MaxIdentifierBytes+1)
		}, world.ErrIdentity},
		"non UTF-8 ID": {func(l *world.Lighting) { l.Lights[0].ID = string([]byte{0xff}) }, world.ErrIdentity},
		"duplicate ID": {func(l *world.Lighting) { l.Lights = append(l.Lights, l.Lights[0]) }, world.ErrIdentity},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			lighting := validLighting()
			test.mutate(lighting)
			assertInvalidLighting(t, lighting, test.want)
		})
	}
	fields := map[string]struct {
		value func(*world.Lighting) *float64
		bad   []float64
	}{
		"ambient x": {func(l *world.Lighting) *float64 { return &l.Ambient.X }, []float64{-0.01, 1.01}},
		"ambient y": {func(l *world.Lighting) *float64 { return &l.Ambient.Y }, []float64{-0.01, 1.01}},
		"ambient z": {func(l *world.Lighting) *float64 { return &l.Ambient.Z }, []float64{-0.01, 1.01}},
		"color x":   {func(l *world.Lighting) *float64 { return &l.Lights[0].Color.X }, []float64{-0.01, 1.01}},
		"color y":   {func(l *world.Lighting) *float64 { return &l.Lights[0].Color.Y }, []float64{-0.01, 1.01}},
		"color z":   {func(l *world.Lighting) *float64 { return &l.Lights[0].Color.Z }, []float64{-0.01, 1.01}},
		"position x": {func(l *world.Lighting) *float64 { return &l.Lights[0].Position.X },
			[]float64{-world.MaxCoordinate - 1, world.MaxCoordinate + 1}},
		"position y": {func(l *world.Lighting) *float64 { return &l.Lights[0].Position.Y },
			[]float64{-world.MaxCoordinate - 1, world.MaxCoordinate + 1}},
		"position z": {func(l *world.Lighting) *float64 { return &l.Lights[0].Position.Z },
			[]float64{-world.MaxCoordinate - 1, world.MaxCoordinate + 1}},
		"radius": {
			func(l *world.Lighting) *float64 { return &l.Lights[0].Radius },
			[]float64{0, -1, math.SmallestNonzeroFloat64, math.Nextafter(world.MinLightRadius, 0), world.MaxCoordinate + 1},
		},
	}
	for name, field := range fields {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for _, value := range append(field.bad, math.NaN(), math.Inf(-1), math.Inf(1)) {
				lighting := validLighting()
				*field.value(lighting) = value
				assertInvalidLighting(t, lighting, world.ErrLighting)
				// Finite malformed payloads must also be rejected on the decode path.
				if !math.IsNaN(value) && !math.IsInf(value, 0) {
					document := lightingWorld()
					document.Lighting = lighting
					encoded, err := json.Marshal(document)
					if err != nil {
						t.Fatal(err)
					}
					if _, err := world.Decode(encoded); !errors.Is(err, world.ErrLighting) {
						t.Fatalf("decoded %s=%v: %v", name, value, err)
					}
				}
			}
		})
	}
}

func TestLightingStrictDecode(t *testing.T) {
	t.Parallel()

	document := lightingWorld()
	document.Lighting = validLighting()
	encoded, err := world.Encode(document)
	if err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string][2]string{
		"unknown lighting field": {`"lighting":{"version":1`, `"lighting":{"shadow":true,"version":1`},
		"unknown light field":    {`"id":"lamp"`, `"id":"lamp","intensity":1`},
		"duplicate version":      {`"lighting":{"version":1`, `"lighting":{"version":1,"version":1`},
		"duplicate ID field":     {`"id":"lamp"`, `"id":"lamp","id":"lamp"`},
		"duplicate ambient":      {`"ambient":{"x":0.1,"y":0.2,"z":0.3}`, `"ambient":{"x":0.1,"y":0.2,"z":0.3},"ambient":{"x":0.1,"y":0.2,"z":0.3}`},
		"missing version":        {`"lighting":{"version":1,`, `"lighting":{`},
		"missing ambient":        {`"ambient":{"x":0.1,"y":0.2,"z":0.3},`, ``},
		"missing lights":         {`,"lights":[{"id":"lamp","position":{"x":2,"y":3,"z":4},"color":{"x":1,"y":0.5,"z":0},"radius":8}]`, ``},
		"missing color":          {`"color":{"x":1,"y":0.5,"z":0},`, ``},
		"missing coordinate":     {`"position":{"x":2,"y":3,"z":4}`, `"position":{"x":2,"y":3}`},
		"null ambient":           {`"ambient":{"x":0.1,"y":0.2,"z":0.3}`, `"ambient":null`},
		"unknown version":        {`"lighting":{"version":1`, `"lighting":{"version":2`},
		"duplicate lights":       {`"lights":[`, `"lights":[{"id":"lamp","position":{"x":2,"y":3,"z":4},"color":{"x":1,"y":0.5,"z":0},"radius":8},`},
		"number spelling":        {`"radius":8`, `"radius":8.0`},
		"NaN":                    {`"radius":8`, `"radius":NaN`},
		"overflow":               {`"radius":8`, `"radius":1e999`},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			payload := bytes.Replace(encoded, []byte(change[0]), []byte(change[1]), 1)
			if bytes.Equal(payload, encoded) {
				t.Fatal("test did not alter the payload")
			}
			if _, err := world.Decode(payload); err == nil {
				t.Fatalf("decoded malformed lighting: %s", payload)
			}
		})
	}
	for size := range len(encoded) {
		if _, err := world.Decode(encoded[:size]); err == nil {
			t.Fatalf("accepted lighting truncation at %d", size)
		}
	}
	document.Lighting = nil
	absent, err := world.Encode(document)
	if err != nil {
		t.Fatal(err)
	}
	null := append(bytes.Clone(absent[:len(absent)-1]), []byte(`,"lighting":null}`)...)
	if _, err := world.Decode(null); !errors.Is(err, world.ErrCanonical) {
		t.Fatalf("explicit null silently treated as absent: %v", err)
	}
}

func FuzzDecodeLighting(f *testing.F) {
	document := lightingWorld()
	document.Lighting = validLighting()
	encoded, err := world.Encode(document)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(encoded)
	f.Fuzz(func(t *testing.T, payload []byte) {
		decoded, err := world.Decode(payload)
		if err != nil {
			return
		}
		canonical, err := world.Encode(decoded)
		if err != nil || !bytes.Equal(canonical, payload) {
			t.Fatalf("lighting decode accepted a noncanonical payload: %v", err)
		}
	})
}

func assertInvalidLighting(t *testing.T, lighting *world.Lighting, want error) {
	t.Helper()
	if err := world.ValidateLighting(lighting); !errors.Is(err, want) {
		t.Fatalf("ValidateLighting() = %v, want %v", err, want)
	}
	document := lightingWorld()
	document.Lighting = lighting
	if err := world.Validate(&document); !errors.Is(err, want) {
		t.Fatalf("Validate() = %v, want %v", err, want)
	}
	if _, err := world.Encode(document); !errors.Is(err, want) {
		t.Fatalf("Encode() = %v, want %v", err, want)
	}
}

func lightingWorld() world.Document {
	document := validWorld()
	document.Version = world.Version
	document.Sectors[0].Walls[1].PortalWall = 4
	document.Sectors[1].Walls[3].PortalWall = 2
	return document
}

func validLighting() *world.Lighting {
	return &world.Lighting{
		Version: world.LightingVersion, Ambient: world.Vec3{X: .1, Y: .2, Z: .3},
		Lights: []world.PointLight{{
			ID: "lamp", Position: world.Vec3{X: 2, Y: 3, Z: 4},
			Color: world.Vec3{X: 1, Y: .5}, Radius: 8,
		}},
	}
}

func TestLightingDirectionalAmbientAndActorsContract(t *testing.T) {
	t.Parallel()
	lighting := validLighting()
	lighting.Actors = true
	lighting.AmbientCube = &world.AmbientCube{
		PositiveX: world.Vec3{X: 1}, NegativeX: world.Vec3{Y: 1},
		PositiveY: world.Vec3{Z: 1}, NegativeY: world.Vec3{X: 1, Y: 1},
		PositiveZ: world.Vec3{Y: 1, Z: 1}, NegativeZ: world.Vec3{X: 1, Z: 1},
	}
	encoded, err := json.Marshal(lighting)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(encoded, []byte(`"actors":true`)) || !bytes.Contains(encoded, []byte(`"negative_z":{"x":1,"y":0,"z":1}`)) {
		t.Fatalf("optional contract fields missing: %s", encoded)
	}
	var decoded world.Lighting
	if err := json.Unmarshal(encoded, &decoded); err != nil || !reflect.DeepEqual(lighting, &decoded) {
		t.Fatalf("contract roundtrip %v", err)
	}
	if err := world.ValidateLighting(lighting); err != nil {
		t.Fatal(err)
	}
	faces := []*world.Vec3{
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
				if err := world.ValidateLighting(lighting); !errors.Is(err, world.ErrLighting) {
					t.Fatalf("face%d component%d value%v: %v", faceIndex, componentIndex, value, err)
				}
			}
			*component = original
		}
	}
}
