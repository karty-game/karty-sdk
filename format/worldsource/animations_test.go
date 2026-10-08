package worldsource_test

import (
	"errors"
	"testing"

	"github.com/karty-game/karty-sdk/format/worldsource"
)

func TestAuthoredAnimationFrameResolution(t *testing.T) {
	p := worldsource.AnimationPreset{Name: "glitch", Kind: "flipbook", Frames: []string{"screen", "glitch"}, FPS: 30, IntervalSeconds: 3}
	compiled, err := worldsource.CompileAnimationPreset(p, map[string]uint32{"screen": 1, "glitch": 2})
	if err != nil {
		t.Fatal(err)
	}
	if compiled.Frames[0] != 1 || compiled.Frames[1] != 2 {
		t.Fatal(compiled)
	}
	if _, err := worldsource.CompileAnimationPreset(p, map[string]uint32{"screen": 1}); err == nil {
		t.Fatal("undeclared frame accepted")
	}
}

func TestAnimationCompilerBoundsBeforeAllocatingFrames(t *testing.T) {
	preset := worldsource.AnimationPreset{Name: "glitch", Kind: "flipbook", Frames: make([]string, 65), FPS: 30}
	if _, err := worldsource.CompileAnimationPreset(preset, nil); !errors.Is(err, worldsource.ErrBounds) {
		t.Fatalf("unbounded frame list: %v", err)
	}
	preset = worldsource.AnimationPreset{Name: "fan", Kind: "spin", Speed: 1}
	if _, err := worldsource.CompileAnimationPreset(preset, nil); !errors.Is(err, worldsource.ErrBounds) {
		t.Fatalf("source bounds identity: %v", err)
	}
}

func TestLiquidSurfaceCompilerPreservesAndOwnsOpacity(t *testing.T) {
	opacity := .78
	p := worldsource.AnimationPreset{Name: "acid", Kind: "liquid", Frequency: 1.2, SurfaceAmplitude: .045, Opacity: &opacity, PixelSize: 4}
	compiled, err := worldsource.CompileAnimationPreset(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if compiled.SurfaceAmplitude != .045 || compiled.Opacity == nil || *compiled.Opacity != .78 || compiled.PixelSize != 4 {
		t.Fatal(compiled)
	}
	opacity = .2
	if *compiled.Opacity != .78 {
		t.Fatal("compiled preset aliases authored opacity")
	}
	p.PixelSize = 33
	if _, err := worldsource.CompileAnimationPreset(p, nil); !errors.Is(err, worldsource.ErrBounds) {
		t.Fatalf("bad source bounds: %v", err)
	}
	p.PixelSize = 0
	p.Opacity = nil
	p.SurfaceAmplitude = 0
	compiled, err = worldsource.CompileAnimationPreset(p, nil)
	if err != nil || compiled.Opacity != nil {
		t.Fatalf("omitted opacity changed: %+v %v", compiled, err)
	}
}
