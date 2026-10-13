package worldsource

import (
	"fmt"

	sdkworld "github.com/karty-game/karty-sdk/format/world"
)

// Environment authors world/environment@1 in source v9 and later. A preset
// alone supplies sky and lighting defaults. Angles are explicit when Sun is
// present; omission uses the preset direction. Environment is global and never
// transformed by a prefab instance.
type Environment struct {
	Preset      sdkworld.EnvironmentPreset   `yaml:"preset"`
	Palette     sdkworld.EnvironmentPalette  `yaml:"palette,omitempty"`
	Sun         *sdkworld.EnvironmentSun     `yaml:"sun,omitempty"`
	TimeOfDay   *float64                     `yaml:"time_of_day,omitempty"`
	Backdrop    sdkworld.EnvironmentBackdrop `yaml:"backdrop,omitempty"`
	CloudAmount *float64                     `yaml:"cloud_amount,omitempty"`
	Sky         *Sky                         `yaml:"sky,omitempty"`
}

func validateEnvironment(document *Document) error {
	usesSky := false
	for _, room := range document.Rooms {
		usesSky = usesSky || roomUsesSky(room)
	}
	for _, prefab := range document.Prefabs {
		for _, room := range prefab.Rooms {
			usesSky = usesSky || roomUsesSky(room)
		}
	}
	if document.Environment == nil && !usesSky {
		return nil
	}
	if document.Version < EnvironmentVersion {
		return fmt.Errorf("environment requires source version %d: %w", EnvironmentVersion, ErrVersion)
	}
	if document.Environment == nil {
		return fmt.Errorf("sky ceiling requires environment: %w", ErrEnvironment)
	}
	environment := document.Environment
	if environment.Sky != nil {
		if document.Version < SkyVersion {
			return fmt.Errorf("environment sky requires source version %d: %w", SkyVersion, ErrVersion)
		}
		if err := validateSky(environment.Sky); err != nil {
			return err
		}
	}
	if environment.Backdrop != "" && document.Version < EnvironmentBackdropVersion {
		return fmt.Errorf("environment backdrop requires source version %d: %w", EnvironmentBackdropVersion, ErrVersion)
	}
	if err := sdkworld.ValidateEnvironment(&sdkworld.Environment{
		Version: sdkworld.EnvironmentVersion, Preset: environment.Preset, Palette: environment.Palette, Sun: environment.Sun,
		TimeOfDay: environment.TimeOfDay, CloudAmount: environment.CloudAmount, Backdrop: environment.Backdrop,
	}); err != nil {
		//nolint:errorlint // Preserve the source sentinel while retaining shared validation details.
		return fmt.Errorf("environment: %v: %w", err, ErrEnvironment)
	}
	return nil
}

func roomUsesSky(room Room) bool {
	if room.Sky {
		return true
	}
	for _, edge := range room.Boundary {
		if edge.Sky {
			return true
		}
	}
	return false
}
