package worldgpuchunk

import (
	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldmaterial"
)

func compileRecord(
	ref RecordRef,
	layer uint32,
	document *world.Document,
	materials worldmaterial.Layout,
	options CompileOptions,
) StaticRecord {
	record := StaticRecord{Layer: layer, RGBMRange: float32(options.RGBMRange)}
	if materials.Materials[layer].Coverage == worldmaterial.CoverageMasked {
		record.AlphaMode = 1
	}
	probes := options.SectorProbes
	if ref.OwnerKind == OwnerSolid {
		probes = options.SolidProbes
	}
	if uint64(ref.OwnerID) < uint64(len(probes)) {
		record.Probe = probes[ref.OwnerID]
	}
	if document.Emission != nil {
		for _, emission := range document.Emission.Materials {
			if emission.Material == ref.MaterialID {
				record.Emission = float32(float64(emission.Intensity) * world.MaxEmissionIntensity / 255)
				break
			}
		}
	}
	if animation, phase, water := compileLiquid(document, ref.MaterialID); water {
		record.Water = true
		record.AlphaMode = 2
		record.Opacity = .65
		record.Exponent = 64
		record.Specular = [3]float32{.55, .6, .65}
		record.UVTransform = [4]float32{0, 0, .018, .011}
		if animation != nil {
			record.UVTransform = [4]float32{
				float32(phase * animation.Flow.X),
				float32(phase * animation.Flow.Y),
				float32(animation.Flow.X),
				float32(animation.Flow.Y),
			}
			if animation.Opacity != nil {
				record.Opacity = float32(*animation.Opacity)
			}
		}
	}
	return record
}

func compileLiquid(document *world.Document, material uint32) (*world.AnimationPreset, float64, bool) {
	if document.Animations != nil {
		for _, binding := range document.Animations.Materials {
			if binding.Material != material {
				continue
			}
			for index := range document.Animations.Presets {
				preset := &document.Animations.Presets[index]
				if preset.Name == binding.Preset && preset.Kind == "liquid" {
					return preset, binding.PhaseSeconds, true
				}
			}
		}
	}
	return nil, 0, false
}

func compileWaterMotion(vertex *Vertex, document *world.Document, material uint32) {
	animation, phase, water := compileLiquid(document, material)
	if !water || animation == nil || animation.SurfaceAmplitude <= 0 || vertex.Normal[2] < .9 {
		return
	}
	x, y := float64(vertex.Position[0]), float64(vertex.Position[1])
	wavePhase := x*animation.Frequency + y*animation.Frequency*.73 - x*.11 - y*.17 + phase*animation.Speed
	vertex.Motion = [4]float32{2, float32(animation.SurfaceAmplitude), float32(wavePhase), float32(animation.Speed)}
}
