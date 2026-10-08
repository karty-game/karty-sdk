package world_test

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
)

func emissionDocument() world.Document {
	d := animationDocument()
	d.Emission = &world.Emission{Version: 1, Materials: []world.MaterialEmission{
		{Material: 7, Intensity: 128, Pulse: &world.EmissionPulse{Depth: .5, PeriodSeconds: 3}},
	}}
	return d
}

func TestEmissionCanonicalRoundTripAndMalformedFinalRecord(t *testing.T) {
	t.Parallel()
	d := lightingWorld()
	d.Emission = &world.Emission{Version: 1, Materials: []world.MaterialEmission{
		{Material: 1, Intensity: 128, Pulse: &world.EmissionPulse{Depth: .5, PeriodSeconds: 3}},
		{Material: 2, Intensity: 255},
	}}
	encoded, err := world.Encode(d)
	if err != nil {
		t.Fatal(err)
	}
	got, err := world.Decode(encoded)
	if err != nil || !reflect.DeepEqual(got, d) {
		t.Fatal("emission round trip drift", err)
	}
	for _, bad := range [][]byte{
		bytes.Replace(encoded, []byte(`"intensity":255`), []byte(`"intensity":256`), 1),
		bytes.Replace(encoded, []byte(`"intensity":255`), []byte(`"intensity":-1`), 1),
		bytes.Replace(encoded, []byte(`"material":2,"intensity":255`), []byte(`"material":99,"intensity":255`), 1),
		bytes.Replace(encoded, []byte(`"material":2,"intensity":255`), []byte(`"material":1,"intensity":255`), 1),
		bytes.Replace(encoded, []byte(`"intensity":255`), []byte(`"intensity":255,"extra":true`), 1),
		bytes.Replace(encoded, []byte(`"intensity":255`), []byte(`"intensity":255,"intensity":1`), 1),
	} {
		if _, err := world.Decode(bad); err == nil {
			t.Fatal("accepted malformed complete emission payload")
		}
	}
}

func TestEmissionCompleteValidationAndAnimationComposition(t *testing.T) {
	t.Parallel()
	d := emissionDocument()
	if err := world.ValidateEmission(&d); err != nil {
		t.Fatal(err)
	}
	if err := world.ValidateAnimations(&d); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*world.Document){
		"legacy":                func(d *world.Document) { d.Version = 2 },
		"unknown version":       func(d *world.Document) { d.Emission.Version++ },
		"empty":                 func(d *world.Document) { d.Emission.Materials = nil },
		"oversized":             func(d *world.Document) { d.Emission.Materials = make([]world.MaterialEmission, 197) },
		"zero material":         func(d *world.Document) { d.Emission.Materials[0].Material = 0 },
		"unused":                func(d *world.Document) { d.Emission.Materials[0].Material = 99 },
		"animation frame only":  func(d *world.Document) { d.Emission.Materials[0].Material = 4 },
		"duplicate final entry": func(d *world.Document) { d.Emission.Materials = append(d.Emission.Materials, d.Emission.Materials[0]) },
		"negative depth":        func(d *world.Document) { d.Emission.Materials[0].Pulse.Depth = -.1 },
		"excess depth":          func(d *world.Document) { d.Emission.Materials[0].Pulse.Depth = 1.1 },
		"nonfinite depth":       func(d *world.Document) { d.Emission.Materials[0].Pulse.Depth = math.NaN() },
		"zero period":           func(d *world.Document) { d.Emission.Materials[0].Pulse.PeriodSeconds = 0 },
		"excess period":         func(d *world.Document) { d.Emission.Materials[0].Pulse.PeriodSeconds = 86401 },
		"nonfinite period":      func(d *world.Document) { d.Emission.Materials[0].Pulse.PeriodSeconds = math.Inf(1) },
		"negative phase":        func(d *world.Document) { d.Emission.Materials[0].Pulse.PhaseSeconds = -1 },
	} {
		t.Run(name, func(t *testing.T) {
			d := emissionDocument()
			mutate(&d)
			if err := world.ValidateEmission(&d); err == nil {
				t.Fatal("accepted invalid emission payload")
			}
		})
	}
}

func TestEmissionIntensityQuantizationAndLegacyEncoding(t *testing.T) {
	t.Parallel()
	for _, value := range []float64{-1, 8.01, math.NaN(), math.Inf(1)} {
		if _, err := world.EncodeEmissionIntensity(value); err == nil {
			t.Fatal("invalid intensity accepted", value)
		}
	}
	for code := range 256 {
		value := float64(code) * 8 / 255
		got, err := world.EncodeEmissionIntensity(value)
		if err != nil || int(got) != code {
			t.Fatal("intensity code drift", code, got, err)
		}
	}
	encoded, _ := json.Marshal(world.Document{Version: 1})
	if bytes.Contains(encoded, []byte("emission")) {
		t.Fatal("legacy payload changed")
	}
}

func TestEmissionLightLinksValidateCompletePayload(t *testing.T) {
	t.Parallel()
	base := func() world.Document {
		d := lightingWorld()
		d.Lighting = &world.Lighting{Version: 1, Lights: []world.PointLight{{ID: "fill", Radius: 1}}}
		d.Emission = &world.Emission{
			Version: 1,
			Materials: []world.MaterialEmission{
				{
					Material:  1,
					Intensity: 32,
					Pulse:     &world.EmissionPulse{Depth: .8, PeriodSeconds: 3.2},
					Lights:    []string{d.Lighting.Lights[0].ID},
				},
			},
		}
		return d
	}
	d := base()
	encoded, err := world.Encode(d)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := world.Decode(encoded); err != nil || !reflect.DeepEqual(got, d) {
		t.Fatal("linked light round trip", err)
	}
	for _, mutate := range []func(*world.Document){
		func(d *world.Document) { d.Lighting = nil },
		func(d *world.Document) { d.Emission.Materials[0].Lights = []string{"missing"} },
		func(d *world.Document) { d.Emission.Materials[0].Lights = []string{""} },
		func(d *world.Document) {
			d.Emission.Materials[0].Lights = append(d.Emission.Materials[0].Lights, d.Emission.Materials[0].Lights[0])
		},
		func(d *world.Document) {
			d.Emission.Materials = append(d.Emission.Materials, world.MaterialEmission{Material: 2, Lights: d.Emission.Materials[0].Lights})
		},
		func(d *world.Document) { d.Emission.Materials[0].Lights = make([]string, 51) },
	} {
		d := base()
		mutate(&d)
		if err := world.ValidateEmission(&d); err == nil {
			t.Fatal("invalid linked light payload accepted")
		}
	}
}
