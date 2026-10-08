package worldmaterial_test

import (
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldmaterial"
)

func TestAnimationCoverageRejectsMaskedTargetAndFinalFrame(t *testing.T) {
	document := world.Document{
		Version: world.Version,
		Sectors: []world.Sector{{FloorMaterial: 1}},
		Animations: &world.Animations{
			Version:   1,
			Presets:   []world.AnimationPreset{{Name: "glitch", Kind: "flipbook", Frames: []uint32{1, 2, 3}, FPS: 30}},
			Materials: []world.MaterialAnimation{{Material: 1, Preset: "glitch"}},
		},
	}
	for _, newLayout := range []func([]uint32) (worldmaterial.Layout, error){worldmaterial.NewLayout, worldmaterial.NewLayoutV2} {
		layout, err := newLayout([]uint32{1, 2, 3})
		if err != nil {
			t.Fatal(err)
		}
		if err := worldmaterial.ValidateAnimationCoverage(&document, layout); err != nil {
			t.Fatal(err)
		}
	}
	for _, slot := range []int{0, 2} {
		layout, err := worldmaterial.NewLayoutV2([]uint32{1, 2, 3})
		if err != nil {
			t.Fatal(err)
		}
		layout.Materials[slot].Coverage = worldmaterial.CoverageMasked
		if err := worldmaterial.ValidateAnimationCoverage(&document, layout); err == nil {
			t.Fatal("masked animated material or final frame accepted")
		}
	}
	layout, err := worldmaterial.NewLayout([]uint32{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	if err := worldmaterial.ValidateAnimationCoverage(&document, layout); err == nil {
		t.Fatal("missing final frame accepted")
	}
}
