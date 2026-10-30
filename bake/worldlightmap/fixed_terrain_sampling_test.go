package worldlightmapbake

import (
	"bytes"
	"math"
	"reflect"
	"slices"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
	"github.com/karty-game/karty-sdk/format/worldterrain"
)

func terrainSamplingFixture(t *testing.T) (world.Document, worldlightmap.Layout, []Material, worldterrain.Document) {
	t.Helper()
	document, _, materials := room(t, 2.5)
	document.Sectors[0].Sky = true
	document.Environment = &world.Environment{Version: 1, Preset: world.EnvironmentDaylight}
	document.Lighting = nil
	terrain := fixedTerrainFixture()
	terrain.CellSize, terrain.Columns, terrain.Rows = 1, 5, 3
	terrain.Heights = slices.Repeat([]float32{1}, 15)
	terrain.Normals = slices.Repeat([][3]float32{{0, 0, 1}}, 15)
	terrain.Holes = make([]byte, 8)
	terrain.Patches = []worldterrain.Patch{{Columns: 2, Rows: 2, Weights: terrain.Patches[0].Weights},
		{X: 2, Columns: 2, Rows: 2, Weights: terrain.Patches[0].Weights}}
	layout, err := worldlightmap.Compile(document, worldlightmap.Options{PageSize: 512, TexelsPerUnit: 1})
	if err != nil {
		t.Fatal(err)
	}
	return document, layout, materials, terrain
}

type terrainSampleKey struct{ position, normal [3]float32 }

func terrainSamplingCoefficients(t *testing.T, receivers *worldterrain.Document,
	lighting []worldlightmap.TerrainPatchLighting,
) map[terrainSampleKey][9]float32 {
	t.Helper()
	meshes, err := worldterrain.CompilePatches(receivers)
	if err != nil {
		t.Fatal(err)
	}
	result := make(map[terrainSampleKey][9]float32)
	for patch, mesh := range meshes {
		for index, vertex := range mesh.Vertices {
			key := terrainSampleKey{vertex.Position, vertex.Normal}
			var coefficients [9]float32
			copy(coefficients[:], lighting[patch].Coefficients[index*9:(index+1)*9])
			if previous, duplicate := result[key]; duplicate && previous != coefficients {
				t.Fatalf("shared physical terrain vertex %+v has patch-dependent RNM: %v / %v", key, previous, coefficients)
			}
			result[key] = coefficients
		}
	}
	return result
}

func TestFixedTerrainSamplingIsStableAcrossPatchesAndCrops(t *testing.T) {
	document, layout, materials, terrain := terrainSamplingFixture(t)
	options := Options{Samples: 8, Bounces: 1, Workers: 1, Seed: 17, Terrain: &terrain, TerrainReceivers: &terrain}
	baseline, err := BakeFixed(t.Context(), document, layout, materials, options)
	if err != nil {
		t.Fatal(err)
	}
	want := terrainSamplingCoefficients(t, &terrain, baseline.Lighting.Terrain)
	whole := terrain
	whole.Patches = []worldterrain.Patch{{Columns: 4, Rows: 2, Weights: terrain.Patches[0].Weights}}
	rows := terrain
	rows.Patches = []worldterrain.Patch{{Columns: 4, Rows: 1, Weights: terrain.Patches[0].Weights},
		{Y: 1, Columns: 4, Rows: 1, Weights: terrain.Patches[0].Weights}}
	crop := terrain
	crop.Origin, crop.Columns = [2]float32{2, 0}, 3
	crop.Heights, crop.Normals, crop.Holes = make([]float32, 9), make([][3]float32, 9), make([]byte, 4)
	for row := range 3 {
		copy(crop.Heights[row*3:(row+1)*3], terrain.Heights[row*5+2:row*5+5])
		copy(crop.Normals[row*3:(row+1)*3], terrain.Normals[row*5+2:row*5+5])
	}
	crop.Patches = []worldterrain.Patch{{Columns: 2, Rows: 2, Weights: terrain.Patches[0].Weights}}
	for name, receivers := range map[string]*worldterrain.Document{"whole": &whole, "row partitions": &rows, "cropped chunk": &crop} {
		t.Run(name, func(t *testing.T) {
			options.TerrainReceivers = receivers
			result, err := BakeFixed(t.Context(), document, layout, materials, options)
			if err != nil {
				t.Fatal(err)
			}
			for key, coefficients := range terrainSamplingCoefficients(t, receivers, result.Lighting.Terrain) {
				if coefficients != want[key] {
					t.Fatalf("receiver partition changed physical sample %+v: %v / %v", key, want[key], coefficients)
				}
			}
			if !bytes.Equal(result.Image.Pix, baseline.Image.Pix) || !reflect.DeepEqual(result.Lighting.Probes, baseline.Lighting.Probes) {
				t.Fatal("terrain receiver indexing changed ordinary atlas or actor-probe samples")
			}
		})
	}
}

func TestFixedTerrainPhysicalSampleIdentityAndAllocation(t *testing.T) {
	position, normal := vec{X: 2, Z: 1}, vec{Z: 1}
	want := terrainSampleIndex(position, normal)
	if want != 74645182 {
		t.Fatalf("physical sampling identity changed: %d", want)
	}
	negativeZero := math.Copysign(0, -1)
	if terrainSampleIndex(vec{X: 2, Y: negativeZero, Z: 1}, vec{X: negativeZero, Y: negativeZero, Z: 1}) != want {
		t.Fatal("signed zero changed physical terrain sample identity")
	}
	if terrainSampleIndex(vec{X: 3, Z: 1}, normal) == want || terrainSampleIndex(position, vec{Y: 1}) == want {
		t.Fatal("physical position/normal missing from terrain sample identity")
	}
	actual := 0
	if allocations := testing.AllocsPerRun(
		1000,
		func() { actual = terrainSampleIndex(position, normal) },
	); allocations != 0 ||
		actual != want {
		t.Fatalf("sample identity allocated or changed: %g allocations, %d", allocations, actual)
	}
}

func TestFixedTerrainPhysicalSamplingRetainsSeedAndSourceResponse(t *testing.T) {
	document, layout, materials, terrain := terrainSamplingFixture(t)
	options := Options{Samples: 8, Bounces: 1, Workers: 1, Seed: 17, Terrain: &terrain, TerrainReceivers: &terrain}
	first, err := BakeFixed(t.Context(), document, layout, materials, options)
	if err != nil {
		t.Fatal(err)
	}
	options.Seed++
	otherSeed, err := BakeFixed(t.Context(), document, layout, materials, options)
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(first.Lighting.Terrain, otherSeed.Lighting.Terrain) ||
		first.Lighting.InputSHA256 == otherSeed.Lighting.InputSHA256 {
		t.Fatal("user seed no longer changes finite terrain samples/provenance")
	}
	document.Environment.CloudAmount = new(1.0)
	otherSource, err := BakeFixed(t.Context(), document, layout, materials, options)
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(otherSeed.Lighting.Terrain, otherSource.Lighting.Terrain) ||
		otherSeed.Lighting.InputSHA256 == otherSource.Lighting.InputSHA256 {
		t.Fatal("fixed environment source edit no longer changes terrain/provenance")
	}
	document.Environment = &world.Environment{Version: world.ExplicitEnvironmentVersion, Appearance: &world.EnvironmentAppearance{}}
	dark, err := BakeFixed(t.Context(), document, layout, materials, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(dark.Lighting.Terrain) != len(terrain.Patches) {
		t.Fatal("unlit terrain lost its explicit receiver records")
	}
	for _, patch := range dark.Lighting.Terrain {
		for _, value := range patch.Coefficients {
			if value != 0 {
				t.Fatal("unlit terrain ceased to be covered black", value)
			}
		}
	}
}
