package worldlightmap

import (
	"fmt"
	"math"
	"slices"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
)

func TestAutomaticStaticSelectionExcludesMotionAndNonzeroPulse(t *testing.T) {
	document := directFixture(t)
	steady := document.Lighting.Lights[0]
	steady.ID = "zero-depth"
	document.Lighting.Lights = append(document.Lighting.Lights, steady)
	document.Lighting.Lights[1].Motion = &world.LightMotion{Version: 1, PeriodSeconds: 1}
	document.Emission = &world.Emission{Version: 1, Materials: []world.MaterialEmission{
		{
			Material: document.Sectors[0].FloorMaterial,
			Pulse:    &world.EmissionPulse{Depth: 0, PeriodSeconds: 1},
			Lights:   []string{"zero-depth"},
		},
		{Material: document.Sectors[0].CeilingMaterial, Pulse: &world.EmissionPulse{Depth: .5, PeriodSeconds: 1}, Lights: []string{"red"}},
	}}
	ids, err := StaticLightIDs(&document)
	if err != nil || !slices.Equal(ids, []string{"zero-depth"}) {
		t.Fatalf("eligible sources: %v %v", ids, err)
	}
	layout, err := Compile(document, Options{AllStatic: true})
	if err != nil || !slices.Equal(layout.RuntimeBake.LightIDs, ids) {
		t.Fatalf("automatic recipe: %+v %v", layout.RuntimeBake, err)
	}
	ids[0] = "caller mutation"
	if layout.RuntimeBake.LightIDs[0] != "zero-depth" || document.Lighting.Lights[2].ID != "zero-depth" {
		t.Fatal("static selection aliases caller or source storage")
	}
	layoutOnly, err := Compile(document, Options{})
	if err != nil || layoutOnly.RuntimeBake != nil {
		t.Fatal("omitted bake policy changed layout-only behavior", err)
	}
}

func TestAutomaticEmptyStaticSelectionIsExplicitCoveredBlackRecipe(t *testing.T) {
	document := directFixture(t)
	for i := range document.Lighting.Lights {
		document.Lighting.Lights[i].Motion = &world.LightMotion{Version: 1, PeriodSeconds: 1}
	}
	layout, err := Compile(document, Options{AllStatic: true})
	if err != nil || layout.RuntimeBake == nil || layout.RuntimeBake.LightIDs == nil || len(layout.RuntimeBake.LightIDs) != 0 {
		t.Fatalf("empty automatic recipe: %+v %v", layout.RuntimeBake, err)
	}
	encoded, err := Encode(layout, &document)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(encoded, &document)
	if err != nil || decoded.RuntimeBake.LightIDs == nil {
		t.Fatal("explicit empty recipe lost in binary", err)
	}
	if value, err := OfflineRGBMRange(layout, &document, 2); err != nil || value != 1 {
		t.Fatalf("covered black range: %g %v", value, err)
	}
	document.Lighting = nil
	if _, err := Compile(document, Options{AllStatic: true}); err == nil {
		t.Fatal("automatic bake without a lighting payload accepted")
	}
}

func TestInfluenceEnergyBoundPreservesPrecisionForDistantSources(t *testing.T) {
	lights := make([]world.PointLight, 1000)
	for i := range lights {
		lights[i] = world.PointLight{
			ID:       fmt.Sprintf("source-%d", i),
			Position: world.Vec3{X: float64(i) * 4},
			Radius:   1,
			Color:    world.Vec3{X: 1},
		}
	}
	if bound := influenceEnergyBound(lights); bound < 1 || bound > 1+1e-9 {
		t.Fatalf("1000 distant lamps inflated storage range: %g", bound)
	}
	for i := range lights {
		lights[i].Position = world.Vec3{}
	}
	if bound := influenceEnergyBound(lights); bound < 1000 || bound > 1000+1e-9 {
		t.Fatalf("overlapping source energy underestimated: %g", bound)
	}
	// At any sampled point, all influencing sources must fit under the bound.
	for i := range lights {
		lights[i].Position = world.Vec3{X: float64(i%17) * .1, Y: float64(i%23) * .2, Z: float64(i%11) * .3}
		lights[i].Radius = .05 + float64(i%13)*.1
		lights[i].Color.X = math.Ldexp(1, -(i % 48))
	}
	bound := influenceEnergyBound(lights)
	for sample := range 100 {
		position := world.Vec3{X: float64(sample%17) * .1, Y: float64(sample%23) * .2, Z: float64(sample%11) * .3}
		energy := 0.0
		for _, light := range lights {
			delta := sub(light.Position, position)
			if dot(delta, delta) < light.Radius*light.Radius {
				energy += light.Color.X
			}
		}
		if energy > bound {
			t.Fatalf("sample %d energy %g exceeds conservative bound %g", sample, energy, bound)
		}
	}
}
