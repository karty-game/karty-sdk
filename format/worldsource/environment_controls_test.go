package worldsource_test

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldsource"
)

func TestEnvironmentSourceClockAndClouds(t *testing.T) {
	t.Parallel()
	clock, clouds := 0.0, 0.0
	document := validSource()
	document.Environment = &worldsource.Environment{Preset: world.EnvironmentDaylight, TimeOfDay: &clock, CloudAmount: &clouds}
	if err := worldsource.Validate(&document); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	var decoded worldsource.Document
	if err := json.Unmarshal(encoded, &decoded); err != nil || !reflect.DeepEqual(document, decoded) {
		t.Fatal("zero source controls lost", err)
	}
	document.Environment.Sun = &world.EnvironmentSun{}
	if err := worldsource.Validate(&document); !errors.Is(err, worldsource.ErrEnvironment) {
		t.Fatal("ambiguous clock and sun", err)
	}
	document.Environment.Sun = nil
	for _, value := range []float64{-1, 24, math.NaN(), math.Inf(1)} {
		document.Environment.TimeOfDay = &value
		if err := worldsource.Validate(&document); !errors.Is(err, worldsource.ErrEnvironment) {
			t.Fatal("bad source clock", value, err)
		}
	}
	document.Environment.TimeOfDay = &clock
	for _, value := range []float64{-.001, 1.001, math.NaN(), math.Inf(1)} {
		document.Environment.CloudAmount = &value
		if err := worldsource.Validate(&document); !errors.Is(err, worldsource.ErrEnvironment) {
			t.Fatal("bad source clouds", value, err)
		}
	}
}
