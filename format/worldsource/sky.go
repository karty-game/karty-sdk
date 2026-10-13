package worldsource

import (
	"fmt"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/karty-game/karty-sdk/format/worldsky"
)

// Sky authors the separately packaged world/sky@1 resource in source v12.
// Image paths are confined, slash-separated level-relative PNG references.
type Sky struct {
	CloudSparse string       `yaml:"cloud_sparse,omitempty"`
	CloudDense  string       `yaml:"cloud_dense,omitempty"`
	Background  string       `yaml:"background,omitempty"`
	Horizon     string       `yaml:"horizon,omitempty"`
	Settings    *SkySettings `yaml:"settings,omitempty"`
}

// SkySettings applies optional presentation overrides independently. Explicit
// zero is retained, while omission inherits worldsky.DefaultSettings().
type SkySettings struct {
	CloudUVScale          *[2]float64 `yaml:"cloud_uv_scale,omitempty"`
	CloudDenseScrollScale *float64    `yaml:"cloud_dense_scroll_scale,omitempty"`
	CloudDenseOffset      *[2]float64 `yaml:"cloud_dense_offset,omitempty"`
	CloudParallax         *[2]float64 `yaml:"cloud_parallax,omitempty"`
	HorizonUVScale        *float64    `yaml:"horizon_uv_scale,omitempty"`
	HorizonParallax       *float64    `yaml:"horizon_parallax,omitempty"`
	HorizonHaze           *float64    `yaml:"horizon_haze,omitempty"`
	HorizonEmission       *[3]float64 `yaml:"horizon_emission,omitempty"`
}

// ResolvedSettings merges individual authored fields onto artwork-free defaults.
func (sky Sky) ResolvedSettings() worldsky.Settings {
	settings := worldsky.DefaultSettings()
	if sky.Settings == nil {
		return settings
	}
	overrides := sky.Settings
	if overrides.CloudUVScale != nil {
		settings.CloudUVScale = *overrides.CloudUVScale
	}
	if overrides.CloudDenseScrollScale != nil {
		settings.CloudDenseScrollScale = *overrides.CloudDenseScrollScale
	}
	if overrides.CloudDenseOffset != nil {
		settings.CloudDenseOffset = *overrides.CloudDenseOffset
	}
	if overrides.CloudParallax != nil {
		settings.CloudParallax = *overrides.CloudParallax
	}
	if overrides.HorizonUVScale != nil {
		settings.HorizonUVScale = *overrides.HorizonUVScale
	}
	if overrides.HorizonParallax != nil {
		settings.HorizonParallax = *overrides.HorizonParallax
	}
	if overrides.HorizonHaze != nil {
		settings.HorizonHaze = *overrides.HorizonHaze
	}
	if overrides.HorizonEmission != nil {
		settings.HorizonEmission = *overrides.HorizonEmission
	}
	return settings
}

func validateSky(sky *Sky) error {
	for _, reference := range [4]string{sky.CloudSparse, sky.CloudDense, sky.Background, sky.Horizon} {
		if reference == "" {
			continue
		}
		if len(reference) > 1024 || !utf8.ValidString(reference) || strings.ContainsAny(reference, "\\\x00:") ||
			strings.HasPrefix(reference, "/") || path.Clean(reference) != reference || reference == ".." ||
			strings.HasPrefix(reference, "../") || !strings.EqualFold(path.Ext(reference), ".png") {
			return fmt.Errorf("sky image path: %w", ErrEnvironment)
		}
	}
	if err := worldsky.ValidateSettings(sky.ResolvedSettings()); err != nil {
		return fmt.Errorf("sky settings: %w: %w", ErrEnvironment, err)
	}
	return nil
}
