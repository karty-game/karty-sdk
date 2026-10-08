package world_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
)

func animationDocument() world.Document {
	return world.Document{
		Version: world.Version,
		Sectors: []world.Sector{{FloorMaterial: 3, CeilingMaterial: 7}},
		Animations: &world.Animations{Version: 1, Presets: []world.AnimationPreset{
			{Name: "glitch", Kind: "flipbook", Frames: []uint32{3, 4, 5}, FPS: 30, IntervalSeconds: 3},
			{Name: "acid", Kind: "liquid", Flow: world.Vec2{X: .02}, Amplitude: .025, Frequency: 2, Speed: .4},
			{Name: "fan", Kind: "spin", Axis: world.Vec3{Y: 1}, Speed: 2},
			{Name: "piston", Kind: "oscillate", Offset: world.Vec3{Z: .25}, PeriodSeconds: 2},
		}, Materials: []world.MaterialAnimation{{Material: 3, Preset: "glitch"}, {Material: 7, Preset: "acid"}}},
		Contents: []world.Content{
			{
				ID:    "fan",
				Actor: &world.Actor{Sprite: &world.Sprite{Facing: world.SpriteFixed}, Animation: &world.AnimationBinding{Preset: "fan"}},
			},
		},
	}
}

func TestAnimationValidationCompleteBindingsAndBounds(t *testing.T) {
	d := animationDocument()
	if err := world.ValidateAnimations(&d); err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*world.Document){
		"version":               func(d *world.Document) { d.Version = 2 },
		"payload version":       func(d *world.Document) { d.Animations.Version = 2 },
		"duplicate preset":      func(d *world.Document) { d.Animations.Presets[3].Name = "fan" },
		"unknown final binding": func(d *world.Document) { d.Animations.Materials[1].Preset = "missing" },
		"unused material":       func(d *world.Document) { d.Animations.Materials[1].Material = 99 },
		"duplicate material":    func(d *world.Document) { d.Animations.Materials[1].Material = 3 },
		"recursive frame":       func(d *world.Document) { d.Animations.Presets[0].Frames[2] = 7 },
		"short interval":        func(d *world.Document) { d.Animations.Presets[0].IntervalSeconds = .01 },
		"unknown kind":          func(d *world.Document) { d.Animations.Presets[3].Kind = "rotate" },
		"cross kind field":      func(d *world.Document) { d.Animations.Presets[3].FPS = 1 },
		"nonunit axis":          func(d *world.Document) { d.Animations.Presets[2].Axis.Y = 2 },
		"nonfinite":             func(d *world.Document) { d.Animations.Presets[1].Speed = math.NaN() },
		"missing actor preset":  func(d *world.Document) { d.Contents[0].Actor.Animation.Preset = "missing" },
		"nonfixed motion":       func(d *world.Document) { d.Contents[0].Actor.Sprite.Facing = world.SpriteUpright },
		"missing payload":       func(d *world.Document) { d.Animations = nil },
		"negative phase":        func(d *world.Document) { d.Contents[0].Actor.Animation.PhaseSeconds = -1 },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			d := animationDocument()
			mutate(&d)
			if err := world.ValidateAnimations(&d); err == nil {
				t.Fatal("accepted invalid complete animation payload")
			}
		})
	}
}

func TestAnimationMaterialIDsAndLegacyEncoding(t *testing.T) {
	d := animationDocument()
	d.Sectors = []world.Sector{{FloorMaterial: 3, CeilingMaterial: 7}}
	ids := world.MaterialIDs(&d)
	if len(ids) != 4 || ids[0] != 3 || ids[1] != 7 || ids[2] != 4 || ids[3] != 5 {
		t.Fatalf("IDs = %v", ids)
	}
	d.Animations.Presets = append(
		d.Animations.Presets,
		world.AnimationPreset{Name: "actor-only", Kind: "flipbook", Frames: []uint32{90, 91}, FPS: 10},
	)
	if len(world.MaterialIDs(&d)) != 4 {
		t.Fatal("actor-only frames entered world material atlas")
	}
	data, err := json.Marshal(world.Document{Version: 1})
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{\"version\":1,\"sectors\":null,\"contents\":null}" {
		t.Fatalf("legacy encoding changed: %s", data)
	}
}

func TestLiquidSurfaceBoundsAndTarget(t *testing.T) {
	for name, mutate := range map[string]func(*world.AnimationPreset){
		"negative displacement":  func(p *world.AnimationPreset) { p.SurfaceAmplitude = -.01 },
		"excess displacement":    func(p *world.AnimationPreset) { p.SurfaceAmplitude = .251 },
		"nonfinite displacement": func(p *world.AnimationPreset) { p.SurfaceAmplitude = math.Inf(1) },
		"negative opacity":       func(p *world.AnimationPreset) { p.Opacity = new(-.1) },
		"excess opacity":         func(p *world.AnimationPreset) { p.Opacity = new(1.1) },
		"nonfinite opacity":      func(p *world.AnimationPreset) { p.Opacity = new(math.NaN()) },
		"excess pixels":          func(p *world.AnimationPreset) { p.PixelSize = 33 },
	} {
		t.Run(name, func(t *testing.T) {
			p := world.AnimationPreset{Name: "acid", Kind: "liquid", Frequency: 1}
			mutate(&p)
			if world.ValidateAnimationPreset(&p) == nil {
				t.Fatal("invalid liquid parameters accepted")
			}
		})
	}
	for _, kind := range []string{"flipbook", "spin", "oscillate"} {
		for _, field := range []string{"displacement", "opacity", "pixels"} {
			t.Run(kind+"/"+field, func(t *testing.T) {
				d := animationDocument()
				p := d.Animations.Presets[0]
				for _, candidate := range d.Animations.Presets {
					if candidate.Kind == kind {
						p = candidate
					}
				}
				switch field {
				case "displacement":
					p.SurfaceAmplitude = .02
				case "opacity":
					p.Opacity = new(1.0)
				case "pixels":
					p.PixelSize = 4
				}
				if world.ValidateAnimationPreset(&p) == nil {
					t.Fatal("liquid field accepted on other kind")
				}
			})
		}
	}
	d := animationDocument()
	d.Animations.Presets[1].SurfaceAmplitude = .045
	if world.ValidateAnimations(&d) == nil {
		t.Fatal("ceiling-only material accepted for waves")
	}
	d.Sectors[0].FloorMaterial = 7
	d.Sectors[0].CeilingMaterial = 3
	if err := world.ValidateAnimations(&d); err != nil {
		t.Fatal(err)
	}
	d.Sectors[0].Floor.A = .1
	if world.ValidateAnimations(&d) == nil {
		t.Fatal("sloped-only material accepted for waves")
	}
	d.StaticSolids = &world.StaticSolids{Items: []world.Solid{{TopMaterial: 7}}}
	if err := world.ValidateAnimations(&d); err != nil {
		t.Fatal(err)
	}
}

func TestLiquidOpacityCanonicalEncoding(t *testing.T) {
	d := lightingWorld()
	d.Animations = &world.Animations{Version: 1, Presets: []world.AnimationPreset{{Name: "acid", Kind: "liquid", Frequency: 1}}}
	encoded, err := world.Encode(d)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte(`"opacity"`)) {
		t.Fatal("omitted opacity encoded")
	}
	null := bytes.Replace(encoded, []byte(`"name":"acid"`), []byte(`"name":"acid","opacity":null`), 1)
	if _, err := world.Decode(null); !errors.Is(err, world.ErrCanonical) {
		t.Fatalf("null opacity accepted: %v", err)
	}
	d.Animations.Presets[0].Opacity = new(0.0)
	encoded, err = world.Encode(d)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := world.Decode(encoded)
	if err != nil || decoded.Animations.Presets[0].Opacity == nil || *decoded.Animations.Presets[0].Opacity != 0 {
		t.Fatalf("explicit transparent opacity lost: %v", err)
	}
}
