package worldlightmapbake

import (
	"bytes"
	"image"
	"image/color"
	"testing"

	"github.com/karty-game/karty-sdk/format/worldvegetation"
)

func testBakeVegetation(alpha byte) *Vegetation {
	return &Vegetation{Document: worldvegetation.Document{
		Version: worldvegetation.Version,
		Presets: []worldvegetation.Preset{
			{
				Name:         "grass",
				Texture:      17,
				Width:        worldvegetation.Range{Min: .8, Max: .8},
				Height:       worldvegetation.Range{Min: 1, Max: 1},
				DrawDistance: 32,
			},
		},
		Layers: []worldvegetation.Layer{{Sector: 0, Preset: "grass", Density: 1, Seed: 42, Surface: worldvegetation.SurfaceFloor}},
	},
		Materials: []Material{{ID: 17, Albedo: solid(color.NRGBA{G: 220, A: alpha})}}}
}

func TestVegetationCutoutsRejectTransparentRaysAndRetainDiffuseColor(t *testing.T) {
	t.Parallel()
	albedo := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	albedo.SetNRGBA(0, 0, color.NRGBA{G: 220, A: 0})
	albedo.SetNRGBA(1, 0, color.NRGBA{G: 220, A: 255})
	p := preparedTransport{}
	p.accel.vegetation = make([]vegetationReflectance, 0, 1)
	points := [3]vec{{X: 0, Y: 0, Z: 1}, {X: 2, Y: 0, Z: 1}, {X: 0, Y: 2, Z: 1}}
	uv := [3][2]float32{{0, 0}, {1, 0}, {0, 1}}
	p.appendVegetationTriangle(points, uv, albedo, true)
	p.accel.prepare()
	direction := vec{Z: 1}
	if p.accel.blocked(vec{X: .25, Y: .25}, direction, 2) || !p.accel.blocked(vec{X: 1.25, Y: .25}, direction, 2) {
		t.Fatal("cutout did not distinguish clear and opaque texels")
	}
	if !p.accel.blocked(vec{X: 1.25, Y: .25, Z: 2}, vec{Z: -1}, 2) {
		t.Fatal("foliage stopped casting from its back face")
	}
	reflectance := p.materials[0].sample(vec{X: 1.25, Y: .25, Z: 1})
	if reflectance.Y <= 0 || reflectance.X != 0 || reflectance.Z != 0 {
		t.Fatal("foliage albedo did not reach ordinary diffuse transport", reflectance)
	}
	p.accel.vegetation[0].leaf = false
	if !p.accel.blocked(vec{X: .25, Y: .25}, direction, 2) {
		t.Fatal("negative control failed to detect opaque cutout replacement")
	}
}

func TestVegetationStaticBakeUsesSameMapsAndSharedPreparation(t *testing.T) {
	t.Parallel()
	document, layout, materials := room(t, 0)
	options := Options{Samples: 4, Bounces: 1, Workers: 1, Seed: 7}
	baker := NewBaker(document, materials)
	baseline, err := baker.BakeFixed(t.Context(), layout, options)
	if err != nil {
		t.Fatal(err)
	}
	options.Vegetation = testBakeVegetation(0)
	transparent, err := baker.BakeFixed(t.Context(), layout, options)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(transparent.Image.Pix, baseline.Image.Pix) || transparent.Stats.ReceiverTexels != baseline.Stats.ReceiverTexels ||
		len(transparent.Lighting.Probes) != len(baseline.Lighting.Probes) {
		t.Fatal("clear foliage changed existing receiver atlas or probe layout")
	}
	options.Vegetation = testBakeVegetation(255)
	opaque, err := baker.BakeFixed(t.Context(), layout, options)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(opaque.Image.Pix, baseline.Image.Pix) || opaque.Stats.ReceiverTexels != baseline.Stats.ReceiverTexels ||
		opaque.Lighting.InputSHA256 == transparent.Lighting.InputSHA256 {
		t.Fatal("static foliage did not shadow existing atlas or invalidate clear-alpha input")
	}
	previous := baker.prepared
	options.Workers = 2
	again, err := baker.BakeFixed(t.Context(), layout, options)
	if err != nil {
		t.Fatal(err)
	}
	if previous != baker.prepared || !bytes.Equal(again.Image.Pix, opaque.Image.Pix) {
		t.Fatal("unchanged foliage rebuilt transport or worker count changed pixels")
	}
	options.Vegetation.Document.Layers[0].Seed++
	changed, err := baker.BakeFixed(t.Context(), layout, options)
	if err != nil {
		t.Fatal(err)
	}
	if baker.prepared == previous || changed.Lighting.InputSHA256 == opaque.Lighting.InputSHA256 {
		t.Fatal("seed edit reused stale foliage transport or cached pixels")
	}
}

func TestVegetationCandidateBudgetRejectsBeforeExpansion(t *testing.T) {
	t.Parallel()
	document, _, _ := room(t, 0)
	input := testBakeVegetation(255)
	input.Document.Presets[0].Geometry = worldvegetation.GeometryTree
	input.Document.Version = worldvegetation.TreeVersion
	for index := range document.Sectors[0].Walls {
		document.Sectors[0].Walls[index].Start.X *= 1000
		document.Sectors[0].Walls[index].End.X *= 1000
	}
	if _, _, err := prepareVegetationLayers(&document, input, nil); err == nil {
		t.Fatal("unbounded static foliage expansion admitted")
	}
}

func TestVegetationPlacementDigestIncludesSeedSizeAndAlpha(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"seed", "width", "alpha"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			input := testBakeVegetation(255)
			before, err := vegetationInputDigest(input)
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "seed":
				input.Document.Layers[0].Seed++
			case "width":
				input.Document.Presets[0].Width.Max += .1
			case "alpha":
				input.Materials[0].Albedo = solid(color.NRGBA{G: 220, A: 0})
			}
			after, err := vegetationInputDigest(input)
			if err != nil || before == after {
				t.Fatal("vegetation input edit failed to invalidate bake", kind, err)
			}
		})
	}
}

func TestVegetationCombinedBudgetCountsExplicitDefaultAlbedo(t *testing.T) {
	t.Parallel()
	document, _, _ := room(t, 0)
	images := map[uint32]image.Image{0: &image.NRGBA{Rect: image.Rect(0, 0, 4096, 4096)}}
	options := Options{Vegetation: testBakeVegetation(255)}
	if err := validateVegetationForBake(&document, options, images); err == nil {
		t.Fatal("large explicit material zero bypassed combined foliage pixel budget")
	}
}
