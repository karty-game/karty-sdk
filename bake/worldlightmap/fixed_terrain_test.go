package worldlightmapbake

import (
	"context"
	"testing"

	"github.com/karty-game/karty-sdk/codec/qoi"
	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
	"github.com/karty-game/karty-sdk/format/worldterrain"
)

func fixedTerrainFixture() worldterrain.Document {
	image := func(size int, rgba [4]byte) worldterrain.Image {
		pixels := make([]byte, size*size*4)
		for at := 0; at < len(pixels); at += 4 {
			copy(pixels[at:at+4], rgba[:])
		}
		return worldterrain.Image{Width: uint16(size), Height: uint16(size), Pixels: pixels}
	}
	return worldterrain.Document{
		Version:         1,
		CellSize:        4,
		Columns:         2,
		Rows:            2,
		Heights:         []float32{1, 1, 1, 1},
		Normals:         [][3]float32{{0, 0, 1}, {0, 0, 1}, {0, 0, 1}, {0, 0, 1}},
		ExteriorSectors: []uint16{0},
		Holes:           []byte{0},
		Materials: []worldterrain.Material{
			{Albedo: image(256, [4]byte{180, 180, 180, 255}), Normal: image(256, [4]byte{128, 128, 255, 255}), Scale: 1},
		},
		Patches: []worldterrain.Patch{
			{Columns: 1, Rows: 1, Weights: image(2, [4]byte{255, 0, 0, 0})},
		},
		FarColour: image(2, [4]byte{180, 180, 180, 255}),
	}
}

func TestFixedTerrainReceivesCombinedCoefficientsAndBindsGeometry(t *testing.T) {
	document, _, materials := room(t, 0)
	document.Sectors[0].Sky = true
	document.Environment = &world.Environment{Version: 1, Preset: world.EnvironmentDaylight}
	document.Lighting.Lights[0].Position.Z = 2
	terrain := fixedTerrainFixture()
	layout, err := worldlightmap.Compile(document, worldlightmap.Options{PageSize: 512, TexelsPerUnit: 1, AllStatic: true})
	if err != nil {
		t.Fatal(err)
	}
	options := Options{Samples: 2, Workers: 1, Terrain: &terrain, TerrainReceivers: &terrain}
	result, err := BakeFixed(context.Background(), document, layout, materials, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Lighting.Terrain) != 1 || len(result.Lighting.Terrain[0].Coefficients) != 36 {
		t.Fatal("missing exact terrain vertex coefficients")
	}
	for _, probe := range result.Lighting.Probes {
		if probe.Sector != 0 || probe.Position.Z <= 1 {
			t.Fatal("probe under terrain or wrong sector", probe)
		}
	}
	if err := worldlightmap.ValidateFixedTerrainBinding(result.Lighting, &terrain); err != nil {
		t.Fatal(err)
	}
	encodedImage, _, err := qoi.Encode(result.Image, qoi.Options{Channels: qoi.ChannelsRGBA, Colorspace: qoi.ColorspaceLinear})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := worldlightmap.NewFixedLighting(layout, &document, encodedImage, result.Lighting)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := worldlightmap.EncodeFixedLighting(manifest)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := worldlightmap.DecodeFixedLightingArtifact(wire, encodedImage, layout)
	if err != nil {
		t.Fatal(err)
	}
	if err := worldlightmap.ValidateFixedTerrainBinding(decoded, &terrain); err != nil {
		t.Fatal(err)
	}
	sum := func(values []float32) float64 {
		total := 0.0
		for _, value := range values {
			total += float64(value)
		}
		return total
	}
	if sum(result.Lighting.Terrain[0].Coefficients) <= 0 {
		t.Fatal("terrain did not receive sunlight and lamp")
	}
	oldDigest := result.Lighting.InputSHA256
	document.Lighting.Lights[0].Color = world.Vec3{}
	withoutLamp, err := BakeFixed(context.Background(), document, layout, materials, options)
	if err != nil {
		t.Fatal(err)
	}
	if sum(withoutLamp.Lighting.Terrain[0].Coefficients) >= sum(result.Lighting.Terrain[0].Coefficients) {
		t.Fatal("terrain did not accumulate static lamp")
	}
	if oldDigest == withoutLamp.Lighting.InputSHA256 {
		t.Fatal("lamp did not change complete fixed identity")
	}
	terrain.Heights[0] = 1.1
	if err := worldlightmap.ValidateFixedTerrainBinding(decoded, &terrain); err == nil {
		t.Fatal("changed terrain silently accepted")
	}
}
