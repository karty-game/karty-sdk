// Package performance owns the versioned cartridge contract for adaptive
// presentation profiles. It intentionally stays independent of renderer state.
package performance

import (
	"cmp"
	"errors"
	"math"
	"slices"
)

const (
	SectionName = "karty.performance.v1"
	MaxConfig   = 64 * 1024
)

var ErrConfig = errors.New("game performance profile configuration is invalid")

// Config uses the binary cartridge contract in SectionName. Profiles are ordered from
// highest to lowest quality; width bounds are inclusive window pixels.
type Config struct {
	TargetFPS      float64   `toml:"target_fps"`
	DefaultProfile string    `toml:"default_profile"`
	Profiles       []Profile `toml:"profiles"`
}

type Profile struct {
	Name             string  `toml:"name"`
	RenderScale      float64 `toml:"render_scale"`
	MinWindowWidth   int     `toml:"min_window_width,omitempty"`
	MaxWindowWidth   int     `toml:"max_window_width,omitempty"`
	FloorReflections *bool   `toml:"floor_reflections,omitempty"`
}

var tierNames = []string{"high", "medium", "low", "potato"}

func Default() Config {
	reflectionsOn := true
	noReflections := false
	return Config{
		TargetFPS:      60,
		DefaultProfile: "medium",
		Profiles: []Profile{
			{Name: "high", RenderScale: 1.5, MinWindowWidth: 1401, FloorReflections: &reflectionsOn},
			{Name: "medium", RenderScale: 1, FloorReflections: &reflectionsOn},
			{Name: "low", RenderScale: .75, FloorReflections: &noReflections},
			{Name: "potato", RenderScale: .5, MaxWindowWidth: 799, FloorReflections: &noReflections},
		},
	}
}

func Validate(config Config) error {
	if math.IsNaN(config.TargetFPS) || math.IsInf(config.TargetFPS, 0) || config.TargetFPS < 1 || config.TargetFPS > 1000 ||
		!slices.Contains(tierNames, config.DefaultProfile) || len(config.Profiles) != len(tierNames) {
		return ErrConfig
	}

	previousScale := math.Inf(1)
	for index, profile := range config.Profiles {
		if profile.Name != tierNames[index] || math.IsNaN(profile.RenderScale) || math.IsInf(profile.RenderScale, 0) ||
			profile.RenderScale < .25 || profile.RenderScale > 1.5 || profile.RenderScale >= previousScale ||
			profile.MinWindowWidth < 0 || profile.MaxWindowWidth < 0 ||
			profile.MinWindowWidth > 0 && profile.MaxWindowWidth > 0 && profile.MinWindowWidth > profile.MaxWindowWidth {
			return ErrConfig
		}
		if profile.Name == "high" && profile.MinWindowWidth < 1401 ||
			profile.Name == "potato" && (profile.MaxWindowWidth < 1 || profile.MaxWindowWidth > 799) {
			return ErrConfig
		}
		previousScale = profile.RenderScale
	}

	return validateWidthCoverage(config.Profiles)
}

// validateWidthCoverage checks the union without changing quality order or
// allocating a second ladder. Zero bounds mean unbounded; widths are positive.
func validateWidthCoverage(profiles []Profile) error {
	var ordered [4]Profile
	copy(ordered[:], profiles)
	slices.SortFunc(ordered[:], func(left, right Profile) int {
		return cmp.Compare(left.MinWindowWidth, right.MinWindowWidth)
	})
	nextWidth := uint64(1)
	for _, profile := range ordered {
		if uint64(profile.MinWindowWidth) > nextWidth {
			return ErrConfig
		}
		if profile.MaxWindowWidth == 0 {
			return nil
		}
		nextWidth = max(nextWidth, uint64(profile.MaxWindowWidth)+1)
	}
	return ErrConfig
}

// Normalize overlays explicitly authored profile values on the built-in ladder.
func Normalize(authored Config) (Config, error) {
	defaults := Default()
	if len(authored.Profiles) > len(tierNames) {
		return Config{}, ErrConfig
	}
	if authored.TargetFPS != 0 {
		defaults.TargetFPS = authored.TargetFPS
	}
	if authored.DefaultProfile != "" {
		defaults.DefaultProfile = authored.DefaultProfile
	}
	if len(authored.Profiles) != 0 {
		seen := make(map[string]bool, len(authored.Profiles))
		for _, profile := range authored.Profiles {
			if seen[profile.Name] {
				return Config{}, ErrConfig
			}
			seen[profile.Name] = true
			index := slices.IndexFunc(defaults.Profiles, func(candidate Profile) bool { return candidate.Name == profile.Name })
			if index < 0 {
				return Config{}, ErrConfig
			}
			resolved := &defaults.Profiles[index]
			if profile.RenderScale != 0 {
				resolved.RenderScale = profile.RenderScale
			}
			if profile.MinWindowWidth != 0 {
				resolved.MinWindowWidth = profile.MinWindowWidth
			}
			if profile.MaxWindowWidth != 0 {
				resolved.MaxWindowWidth = profile.MaxWindowWidth
			}
			if profile.FloorReflections != nil {
				value := *profile.FloorReflections
				resolved.FloorReflections = &value
			}
		}
	}
	if err := Validate(defaults); err != nil {
		return Config{}, err
	}

	return defaults, nil
}
