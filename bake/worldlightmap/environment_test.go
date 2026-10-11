package worldlightmapbake

import (
	"bytes"
	"context"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

func TestSkyOpeningsAreNotBakeBlockersOrReflectanceReceivers(t *testing.T) {
	t.Parallel()
	document, _, materials := room(t, 0)
	document.Environment = &world.Environment{Version: 1, Preset: world.EnvironmentDaylight}
	document.Sectors[0].Sky = true
	for i := range document.Sectors[0].Walls {
		document.Sectors[0].Walls[i].Sky = true
	}
	layout, err := worldlightmap.Compile(document, worldlightmap.Options{
		TexelsPerUnit: 1, PageSize: 512, Padding: 2, Lights: []string{"white"}, ShadowSize: 32,
	})
	if err != nil {
		t.Fatal(err)
	}
	// The ceiling's nonzero material is deliberately missing: only actual
	// polygons participate in reflectance, not finite sky masks.
	materials = materials[:1]
	first, err := Bake(context.Background(), document, layout, materials, Options{Samples: 4, Bounces: 1, Workers: 1})
	if err != nil {
		t.Fatal(err)
	}
	if first.Stats.Triangles != 2 {
		t.Fatalf("sky masks became blockers: triangles=%d", first.Stats.Triangles)
	}
	document.Environment.Preset = world.EnvironmentSpace
	document.Environment.Sun = &world.EnvironmentSun{Azimuth: 270, Elevation: 0}
	second, err := Bake(context.Background(), document, layout, materials, Options{Samples: 4, Bounces: 1, Workers: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Image.Pix, second.Image.Pix) {
		t.Fatal("point-light-only producer unexpectedly baked sun or sky fill")
	}
}
