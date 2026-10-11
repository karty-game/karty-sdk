package worldchunk

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

func TestChunkPagesKeepDensityRecipeAndGlobalIdentity(t *testing.T) {
	document := fixture(t)
	for i := range document.Sectors {
		document.Sectors[i].Floor = world.Plane{}
		document.Sectors[i].Ceiling = world.Plane{C: 2}
		for edge := range document.Sectors[i].Walls {
			document.Sectors[i].Walls[edge].Start.Y *= .5
			document.Sectors[i].Walls[edge].End.Y *= .5
		}
	}
	document.Contents[0].Position.Y *= .5
	document.Lighting = &world.Lighting{Version: 1, Lights: []world.PointLight{
		{ID: "neighbor", Position: world.Vec3{X: 6, Y: 1, Z: 1}, Color: world.Vec3{X: 1}, Radius: 10},
	}}
	for _, density := range []float64{16, 64} {
		options := Options{Lightmap: worldlightmap.Options{
			PageSize: 1024, TexelsPerUnit: density, Padding: 6, Lights: []string{"neighbor"}, ShadowSize: 128,
		}}
		plan, err := Plan(document, options)
		if err != nil || len(plan.Chunks) != 1 {
			t.Fatalf("density %g: %v", density, err)
		}
		chunk := plan.Chunks[0]
		page := 512
		if density == 64 {
			page = 1024
		}
		if chunk.Layout.Pages[0] != (worldlightmap.Page{Width: page, Height: page}) ||
			chunk.Layout.TexelsPerUnit != density || chunk.Layout.Padding != 6 ||
			chunk.LightmapBytes != uint64(page*page*12) ||
			!slices.Equal(chunk.Layout.RuntimeBake.LightIDs, []string{"neighbor"}) ||
			chunk.Layout.RuntimeBake.ShadowSize != 128 {
			t.Fatalf("density/recipe/reservation changed: %+v", chunk.Layout)
		}
		fixedOptions := options.Lightmap
		fixedOptions.PageSize = page
		fixed, err := worldlightmap.CompileReceivers(document, fixedOptions, chunk.Sectors, chunk.Solids)
		if err != nil || !reflect.DeepEqual(fixed, chunk.Layout) {
			t.Fatalf("selected layout changed global bindings or recipe: %v", err)
		}
		if err := ValidateMeshBinding(&chunk.Mesh, &chunk.Layout); err != nil {
			t.Fatal(err)
		}
		again, err := Plan(document, options)
		if err != nil {
			t.Fatal(err)
		}
		before, _ := json.Marshal(plan)
		after, _ := json.Marshal(again)
		if !bytes.Equal(before, after) {
			t.Fatal("adaptive page identity is not deterministic")
		}
		legacy, err := worldlightmap.Compile(document, options.Lightmap)
		if err != nil || legacy.Pages[0].Width != 1024 {
			t.Fatalf("legacy fixed page changed: %v", err)
		}
	}
}

func TestChunkPageMaximumAndOtherBudgetsRemainHard(t *testing.T) {
	document := fixture(t)
	if _, err := Plan(
		document,
		Options{Lightmap: worldlightmap.Options{PageSize: 512, TexelsPerUnit: 64}},
	); !errors.Is(
		err,
		worldlightmap.ErrPageCapacity,
	) {
		t.Fatalf("explicit 512 maximum was exceeded: %v", err)
	}
	for name, options := range map[string]Options{
		"recipe":  {Lightmap: worldlightmap.Options{Lights: []string{"missing"}}},
		"density": {Lightmap: worldlightmap.Options{TexelsPerUnit: 65}},
		"page":    {Lightmap: worldlightmap.Options{PageSize: 2048}},
		"texels":  {Lightmap: worldlightmap.Options{MaxTexels: 512*512 - 1}},
		"mesh":    {MaxMeshBytes: 1}, "encoded": {MaxEncodedBytes: 1},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Plan(document, options); err == nil || errors.Is(err, worldlightmap.ErrPageCapacity) {
				t.Fatalf("nonpacking failure swallowed or reclassified: %v", err)
			}
		})
	}
	plan, err := Plan(document, Options{MaxSectors: 1})
	if err != nil || len(plan.Chunks) != 2 {
		t.Fatalf("adaptive pages bypassed sector guard: %v", err)
	}
}
