package worldlightmapbake

import (
	"context"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

func receiverRooms(t testing.TB, barrier bool) (world.Document, worldlightmap.Layout, []Material) {
	t.Helper()
	d, _, materials := room(t, 0)
	d.Sectors[0].ID = "a"
	second := d.Sectors[0]
	second.ID = "b"
	second.Walls = append([]world.Wall(nil), second.Walls...)
	for i := range second.Walls {
		second.Walls[i].Start.X += 4
		second.Walls[i].End.X += 4
	}
	d.Sectors[0].Walls[1].Portal = 1
	d.Sectors[0].Walls[1].PortalWall = 4
	second.Walls[3].Portal = 0
	second.Walls[3].PortalWall = 2
	d.Sectors = append(d.Sectors, second)
	d.Lighting.Lights[0].Position = vec{X: 6, Y: 2, Z: 1}
	if barrier {
		d.StaticSolids = &world.StaticSolids{
			Version: 1,
			Items: []world.Solid{
				{
					ID:             "outside-chunk",
					Footprint:      []world.Vec2{{X: 4.9, Y: 0}, {X: 5.1, Y: 0}, {X: 5.1, Y: 4}, {X: 4.9, Y: 4}},
					Bottom:         world.Plane{},
					Top:            world.Plane{C: 3},
					SideMaterial:   1,
					TopMaterial:    1,
					BottomMaterial: 1,
				},
			},
		}
	}
	l, err := worldlightmap.CompileReceivers(
		d,
		worldlightmap.Options{TexelsPerUnit: 1, PageSize: 512, Padding: 2, Lights: []string{"white"}},
		[]int{0},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	return d, l, materials
}

func TestReceiverBakeIncludesNeighborOcclusion(t *testing.T) {
	open, openLayout, materials := receiverRooms(t, false)
	blocked, blockedLayout, _ := receiverRooms(t, true)
	lit, err := Bake(context.Background(), open, openLayout, materials, Options{Samples: 1, Workers: 1})
	if err != nil {
		t.Fatal(err)
	}
	shadow, err := Bake(context.Background(), blocked, blockedLayout, materials, Options{Samples: 1, Workers: 1})
	if err != nil {
		t.Fatal(err)
	}
	x, y := floorAt(openLayout, vec{X: 2, Y: 2})
	if maximum(decoded(lit, x, y)) < .05 {
		t.Fatal("light from neighboring chunk missing")
	}
	x, y = floorAt(blockedLayout, vec{X: 2, Y: 2})
	if maximum(decoded(shadow, x, y)) > 1e-8 {
		t.Fatal("unselected neighboring solid did not cast shadow")
	}
	if shadow.Stats.Triangles <= lit.Stats.Triangles {
		t.Fatal("neighbor geometry absent from transport scene")
	}
	for _, binding := range blockedLayout.Bindings {
		if binding.Kind == "solid-top" && binding.Chart != -1 {
			t.Fatal("neighbor allocated receiver chart")
		}
	}
}

func TestReceiverBakeStopsAtTransformedLightingBoundary(t *testing.T) {
	d, _, materials := receiverRooms(t, false)
	for i := range d.Sectors[1].Walls {
		d.Sectors[1].Walls[i].Start.X += 100
		d.Sectors[1].Walls[i].End.X += 100
	}
	d.Lighting.Lights[0].Position.X += 100
	l, err := worldlightmap.CompileReceivers(
		d,
		worldlightmap.Options{TexelsPerUnit: 1, PageSize: 512, Padding: 2, Lights: []string{"white"}},
		[]int{0},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Bake(context.Background(), d, l, materials, Options{Samples: 1, Workers: 1})
	if err != nil {
		t.Fatal(err)
	}
	x, y := floorAt(l, vec{X: 2, Y: 2})
	if maximum(decoded(result, x, y)) > 1e-8 || result.Image.NRGBAAt(x, y).A == 0 {
		t.Fatal("dark physical space is not covered black")
	}
	if result.Stats.Triangles >= 24 {
		t.Fatal("transformed destination geometry entered transport scene")
	}
}
