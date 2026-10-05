package world_test

import (
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
)

func TestLightMotionCanonicalRoundTripAndBounds(t *testing.T) {
	document := lightingWorld()
	document.Version = world.Version
	document.Lighting = validLighting()
	document.Lighting.Lights[0].Motion = &world.LightMotion{Version: 1, Offset: world.Vec3{X: 12, Z: .8}, PeriodSeconds: 12}
	encoded, err := world.Encode(document)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := world.Decode(encoded)
	if err != nil || !reflect.DeepEqual(decoded.Lighting, document.Lighting) {
		t.Fatalf("motion round trip: %v", err)
	}
	for name, mutate := range map[string]func(*world.LightMotion){
		"version":           func(m *world.LightMotion) { m.Version = 2 },
		"zero period":       func(m *world.LightMotion) { m.PeriodSeconds = 0 },
		"short period":      func(m *world.LightMotion) { m.PeriodSeconds = .099 },
		"long period":       func(m *world.LightMotion) { m.PeriodSeconds = 3601 },
		"nan period":        func(m *world.LightMotion) { m.PeriodSeconds = math.NaN() },
		"infinite offset":   func(m *world.LightMotion) { m.Offset.Y = math.Inf(1) },
		"endpoint overflow": func(m *world.LightMotion) { m.Offset.X = world.MaxCoordinate },
	} {
		t.Run(name, func(t *testing.T) {
			lighting := validLighting()
			lighting.Lights[0].Position.X = 2
			lighting.Lights[0].Motion = &world.LightMotion{Version: 1, PeriodSeconds: 12}
			mutate(lighting.Lights[0].Motion)
			if err := world.ValidateLighting(lighting); !errors.Is(err, world.ErrLighting) {
				t.Fatalf("invalid motion accepted: %v", err)
			}
		})
	}
}
