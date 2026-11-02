package worldgpuchunk

import (
	"math"
	"slices"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldchunk"
	"github.com/karty-game/karty-sdk/format/worldmaterial"
)

func TestCompileReadyKeepsLiquidFieldsAndSeparateBlendedTail(t *testing.T) {
	t.Parallel()
	ready, document, _ := compilerFixture(t)
	materials, err := worldmaterial.NewLayoutV2([]uint32{7, 9})
	if err != nil {
		t.Fatal(err)
	}
	opacity := .7
	document.Animations = &world.Animations{
		Version: world.AnimationVersion,
		Presets: []world.AnimationPreset{
			{
				Name:             "water",
				Kind:             "liquid",
				Flow:             world.Vec2{X: .03, Y: -.02},
				Amplitude:        .1,
				SurfaceAmplitude: .05,
				Opacity:          &opacity,
				Frequency:        1.2,
				Speed:            .6,
			},
		},
		Materials: []world.MaterialAnimation{{Material: 9, Preset: "water", PhaseSeconds: 2}},
	}
	ready.Surfaces = append(ready.Surfaces, worldchunk.ReadySurface{Primary: 1, Secondary: 1, Band: 1})
	opaque := ready.Batches[0]
	water := opaque
	water.Vertices = slices.Clone(opaque.Vertices)
	water.SurfaceIDs = []uint32{1, 1, 1}
	water.Surface = 1
	water.Transparent = true
	water.StaticMaterial = false
	water.MaterialBasis = nil
	for vertex := range 3 {
		water.Vertices[vertex*worldchunk.ReadyStride] += 2
	}
	water.Owners = slices.Clone(opaque.Owners)
	water.Owners[0].Bounds = [6]float64{2, 0, 0, 3, 1, 0}
	ready.Batches = []worldchunk.ReadyBatch{water, opaque}
	data, err := CompileReady(ready, document, materials, CompileOptions{RGBMRange: 8})
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Draws) != 2 || data.Draws[0].AlphaMode != 0 || data.Draws[0].Record != 1 || data.Draws[1].AlphaMode != 2 ||
		data.Draws[1].Record != 0 {
		t.Fatal("water merged into opaque owner draw", data.Draws)
	}
	record := data.Records[:RecordStride]
	for offset, want := range map[int]float32{124: .7, 256: .55, 260: .6, 264: .65, 268: 64, 332: 1, 352: .06, 356: -.04, 360: .03, 364: -.02} {
		if readF32(record, offset) != want {
			t.Fatal("water record field changed", offset, readF32(record, offset), want)
		}
	}
	if readU32(record, 296) != 2 || readU32(record, 292)&0x40000000 != 0 || readF32(record, 236) != 8 || readU32(record, 276) != 4 {
		t.Fatal("water lost blend/double-sided/prepared-RNM profile")
	}
	for index := range 3 {
		vertex := data.Vertices[index*VertexStride : (index+1)*VertexStride]
		if readF32(vertex, 80) != 2 || readF32(vertex, 84) != .05 || readF32(vertex, 92) != .6 {
			t.Fatal("missing prepared wave motion")
		}
		x, y := float64(readF32(vertex, 0)), float64(readF32(vertex, 4))
		actual := float64(readF32(vertex, 88)) + .11*x + .17*y
		want := (x+.73*y)*1.2 + 2*.6
		if math.Abs(actual-want) > 1e-6 {
			t.Fatal("wave phase changed at a cell boundary", actual, want)
		}
	}
	if data.Draws[1].Bounds[2] != -float64(float32(.05)) || data.Draws[1].Bounds[5] != float64(float32(.05)) {
		t.Fatal("wave displacement not included in immutable draw bounds", data.Draws[1].Bounds)
	}
}
