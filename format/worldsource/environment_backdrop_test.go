package worldsource_test

import (
	"errors"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldsource"
)

func TestEnvironmentBackdropRequiresSourceEleven(t *testing.T) {
	t.Parallel()
	if worldsource.EnvironmentBackdropVersion != 11 {
		t.Fatal("backdrop source gate drifted")
	}
	for _, selector := range []world.EnvironmentBackdrop{world.EnvironmentBackdropNone, world.EnvironmentBackdropMountains, world.EnvironmentBackdropCity} {
		document := validSource()
		document.Environment = &worldsource.Environment{Preset: world.EnvironmentDaylight, Backdrop: selector}
		for version := uint16(9); version <= 11; version++ {
			document.Version = version
			err := worldsource.Validate(&document)
			if version < 11 && !errors.Is(err, worldsource.ErrVersion) {
				t.Fatal("released source accepted backdrop", version, err)
			}
			if version == 11 && err != nil {
				t.Fatal("current source rejected backdrop", err)
			}
		}
	}
	document := validSource()
	document.Environment = &worldsource.Environment{Preset: world.EnvironmentDaylight, Backdrop: "ocean"}
	if err := worldsource.Validate(&document); !errors.Is(err, worldsource.ErrEnvironment) {
		t.Fatal("unknown source backdrop", err)
	}
}
