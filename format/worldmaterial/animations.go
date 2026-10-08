package worldmaterial

import (
	"fmt"

	"github.com/karty-game/karty-sdk/format/world"
)

// ValidateAnimationCoverage verifies that material effects only select opaque
// slots. Masked artwork participates in fixed coverage and cannot be animated in
// world/animations@1. Explicit actor flipbooks use ordinary textures separately.
func ValidateAnimationCoverage(document *world.Document, layout Layout) error {
	if document == nil {
		return world.ErrSyntax
	}
	if document.Animations == nil {
		return nil
	}
	if err := world.ValidateAnimations(document); err != nil {
		return err
	}
	if err := layout.Validate(); err != nil {
		return err
	}
	opaque := make(map[uint32]bool, len(layout.Materials))
	for _, rect := range layout.Materials {
		opaque[rect.MaterialID] = layout.Schema == Schema && rect.Coverage == "" ||
			layout.Schema == SchemaV2 && rect.Coverage == CoverageOpaque
	}
	for _, binding := range document.Animations.Materials {
		if !opaque[binding.Material] {
			return fmt.Errorf("animated material %d requires opaque atlas coverage: %w", binding.Material, ErrAtlas)
		}
		for _, preset := range document.Animations.Presets {
			if preset.Name != binding.Preset || preset.Kind != "flipbook" {
				continue
			}
			for _, frame := range preset.Frames {
				if !opaque[frame] {
					return fmt.Errorf("animation %q frame %d requires opaque atlas coverage: %w", preset.Name, frame, ErrAtlas)
				}
			}
		}
	}
	return nil
}
