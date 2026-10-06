package worldlightmap

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"reflect"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
)

func fixture(t testing.TB) world.Document {
	t.Helper()
	encoded, err := os.ReadFile("../world/testdata/two-room.world.json")
	if err != nil {
		t.Fatal(err)
	}
	document, err := world.Decode(bytes.TrimSpace(encoded))
	if err != nil {
		t.Fatal(err)
	}
	return document
}

func TestCompileCanonicalRoundTripAndSlopedDensity(t *testing.T) {
	document := fixture(t)
	layout, err := Compile(document, Options{})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := Encode(layout, &document)
	if err != nil {
		t.Fatal(err)
	}
	again, err := Compile(document, Options{})
	if err != nil || !reflect.DeepEqual(layout, again) {
		t.Fatalf("nondeterministic layout: %v", err)
	}
	decoded, err := Decode(encoded, &document)
	if err != nil || !reflect.DeepEqual(layout, decoded) {
		t.Fatalf("round trip: %v", err)
	}
	chart := layout.Charts[0]
	factor := math.Sqrt(
		chart.UPlane[0]*chart.UPlane[0]+chart.UPlane[1]*chart.UPlane[1]+chart.UPlane[2]*chart.UPlane[2],
	) * float64(
		layout.Pages[0].Width,
	)
	if math.Abs(factor-16) > 1e-8 {
		t.Fatalf("slope density %g", factor)
	}
	if _, err := Decode(append(encoded, ' '), &document); err == nil {
		t.Fatal("noncanonical encoding accepted")
	}
	if _, err := Decode(bytes.Replace(encoded, []byte(`"algorithm":1`), []byte(`"algorithm":1,"algorithm":1`), 1), &document); err == nil {
		t.Fatal("duplicate field accepted")
	}
}

func TestConnectedCapsAndOpenWallBindings(t *testing.T) {
	document := fixture(t)
	document.Sectors[1].Floor = document.Sectors[0].Floor
	document.Sectors[1].Ceiling = document.Sectors[0].Ceiling
	layout, err := Compile(document, Options{})
	if err != nil {
		t.Fatal(err)
	}
	bindings := map[string][]int{}
	for _, b := range layout.Bindings {
		bindings[b.Kind] = append(bindings[b.Kind], b.Chart)
	}
	if bindings["sector-floor"][0] != bindings["sector-floor"][1] || bindings["sector-ceiling"][0] != bindings["sector-ceiling"][1] {
		t.Fatal("ordinary coplanar caps were not merged")
	}
	if bindings["sector-wall"][1] != -1 || bindings["sector-wall"][7] != -1 {
		t.Fatal("fully open portal got receiver chart")
	}
}

func TestGeometryDigestExcludesLightingMaterialAndContent(t *testing.T) {
	document := fixture(t)
	before, err := GeometryDigest(&document)
	if err != nil {
		t.Fatal(err)
	}
	document.Sectors[0].FloorMaterial = 99
	document.Contents[0].Position.X += .1
	after, err := GeometryDigest(&document)
	if err != nil || before != after {
		t.Fatalf("nongeometry invalidation: %v", err)
	}
	document.Sectors[0].Floor.C += .01
	after, err = GeometryDigest(&document)
	if err != nil || before == after {
		t.Fatalf("missed geometry invalidation: %v", err)
	}
}

func TestMalformedCompleteLayoutRejected(t *testing.T) {
	document := fixture(t)
	layout, err := Compile(document, Options{})
	if err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*Layout){
		"digest":            func(l *Layout) { l.GeometrySHA256 = "bad" },
		"unknown algorithm": func(l *Layout) { l.Algorithm = 2 },
		"last binding":      func(l *Layout) { l.Bindings[len(l.Bindings)-1].Index = 999 },
		"missing binding":   func(l *Layout) { l.Bindings = l.Bindings[:len(l.Bindings)-1] },
		"last chart overlap": func(l *Layout) {
			l.Charts[len(l.Charts)-1].Rect = l.Charts[0].Rect
			l.Charts[len(l.Charts)-1].ReceiverRect = l.Charts[0].ReceiverRect
		},
		"bad plane":       func(l *Layout) { l.Charts[len(l.Charts)-1].UPlane[3] = math.NaN() },
		"receiver escape": func(l *Layout) { l.Charts[0].UPlane[3] += 1 },
		"bad basis":       func(l *Layout) { l.Charts[0].Normal.X = 2 },
		"oversized page":  func(l *Layout) { l.Pages[0] = Page{2048, 2048} },
		"missing light": func(l *Layout) {
			l.RuntimeBake = &RuntimeBake{Encoding: "point-visibility@1", LightID: "missing", ShadowSize: 512}
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			encoded, _ := json.Marshal(layout)
			var mutated Layout
			_ = json.Unmarshal(encoded, &mutated)
			mutate(&mutated)
			if err := Validate(&mutated, &document); err == nil {
				t.Fatal("accepted malformed complete layout")
			}
		})
	}
}

func TestExplicitBakeRecipeAndBounds(t *testing.T) {
	document := fixture(t)
	document.Version = world.Version
	document.Sectors[0].Walls[1].PortalWall = 4
	document.Sectors[1].Walls[3].PortalWall = 2
	document.Lighting = &world.Lighting{
		Version: 1,
		Lights: []world.PointLight{
			{ID: "sunroom", Position: world.Vec3{X: 2, Y: 2, Z: 3}, Color: world.Vec3{X: 1, Y: .6, Z: .3}, Radius: 8},
		},
	}
	layout, err := Compile(document, Options{Light: "sunroom", ShadowSize: 256})
	if err != nil || layout.RuntimeBake == nil || layout.RuntimeBake.LightID != "sunroom" {
		t.Fatalf("recipe: %v", err)
	}
	for _, opts := range []Options{{Light: "other"}, {PageSize: 2048}, {MaxPages: 2}, {MaxTexels: 1024}, {TexelsPerUnit: math.Inf(1)}, {ShadowSize: 513}, {Padding: -1}} {
		if _, err := Compile(document, opts); err == nil {
			t.Fatalf("accepted options %+v", opts)
		}
	}
}

func TestSolidPerEdgeBindingsAndThinReceiverSpan(t *testing.T) {
	document := fixture(t)
	document.Version = world.Version
	document.Sectors[0].Walls[1].PortalWall = 4
	document.Sectors[1].Walls[3].PortalWall = 2
	document.StaticSolids = &world.StaticSolids{
		Version: 1,
		Items: []world.Solid{
			{
				ID:             "step",
				Footprint:      []world.Vec2{{X: 1, Y: 1}, {X: 2, Y: 1}, {X: 2, Y: 2}, {X: 1, Y: 2}},
				Bottom:         world.Plane{C: 0},
				Top:            world.Plane{C: .001},
				SideMaterial:   3,
				TopMaterial:    3,
				BottomMaterial: 3,
			},
		},
	}
	layout, err := Compile(document, Options{})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int]bool{}
	for _, b := range layout.Bindings {
		if b.Kind != "solid-side" {
			continue
		}
		if seen[b.Chart] {
			t.Fatal("solid sides share chart")
		}
		seen[b.Chart] = true
		chart := layout.Charts[b.Chart]
		if chart.ReceiverRect[3]-chart.ReceiverRect[1] < 3 {
			t.Fatal("thin receiver has no interior raster sample")
		}
	}
	if len(seen) != 4 {
		t.Fatal("incomplete solid sides")
	}
}

func TestPartialPortalCrossingsPreserveDoorway(t *testing.T) {
	document := fixture(t)
	document.Version = world.Version
	document.Sectors[0].Walls[1].PortalWall = 4
	document.Sectors[1].Walls[3].PortalWall = 2
	document.Sectors[0].Floor = world.Plane{}
	document.Sectors[0].Ceiling = world.Plane{C: 4}
	document.Sectors[1].Floor = world.Plane{B: .5, C: -1}
	document.Sectors[1].Ceiling = world.Plane{B: -.5, C: 5}
	surfaces, err := Surfaces(&document)
	if err != nil {
		t.Fatal(err)
	}
	var wall Surface
	for _, surface := range surfaces {
		if surface.Binding.Kind == "sector-wall" && surface.Binding.Index == 0 && surface.Binding.Edge == 1 {
			wall = surface
		}
	}
	if len(wall.Polygons) != 2 {
		t.Fatalf("crossing portal opaque bands: %+v", wall.Polygons)
	}
	for _, polygon := range wall.Polygons {
		for _, point := range polygon {
			if point.Y < 2 || point.Y > 4 || (point.Z > 1 && point.Z < 3) {
				t.Fatalf("opaque band filled portal opening: %+v", point)
			}
		}
	}
	if _, err := Compile(document, Options{}); err != nil {
		t.Fatal(err)
	}
}

func TestDisconnectedCoincidentCapsRemainIndependent(t *testing.T) {
	document := fixture(t)
	document.Sectors[0].Walls[1].Portal = -1
	document.Sectors[1].Walls[3].Portal = -1
	document.Sectors[1].Floor = document.Sectors[0].Floor
	document.Sectors[1].Ceiling = document.Sectors[0].Ceiling
	layout, err := Compile(document, Options{})
	if err != nil {
		t.Fatal(err)
	}
	floors := []int{}
	for _, binding := range layout.Bindings {
		if binding.Kind == "sector-floor" {
			floors = append(floors, binding.Chart)
		}
	}
	if floors[0] == floors[1] {
		t.Fatal("disconnected caps merged")
	}
	for index := range layout.Bindings {
		binding := &layout.Bindings[index]
		if binding.Kind == "sector-floor" && binding.Index == 1 {
			binding.Chart = floors[0]
		}
	}
	if err := Validate(&layout, &document); err == nil {
		t.Fatal("disconnected chart alias accepted")
	}
}

func TestRuntimeBakeRejectsUnsupportedPhysicalScope(t *testing.T) {
	for name, change := range map[string]func(*world.Document){
		"disconnected": func(d *world.Document) {
			d.Sectors[0].Walls[1].Portal, d.Sectors[0].Walls[1].PortalWall = -1, 0
			d.Sectors[1].Walls[3].Portal, d.Sectors[1].Walls[3].PortalWall = -1, 0
		},
		"one way": func(d *world.Document) { d.Sectors[1].Walls[3].Portal, d.Sectors[1].Walls[3].PortalWall = -1, 0 },
		"transformed": func(d *world.Document) {
			for i := range d.Sectors[1].Walls {
				d.Sectors[1].Walls[i].Start.Y += 10
				d.Sectors[1].Walls[i].End.Y += 10
			}
			d.Sectors[1].Floor.C -= 10 * d.Sectors[1].Floor.B
			d.Sectors[1].Ceiling.C -= 10 * d.Sectors[1].Ceiling.B
			d.Contents[0].Position.Y += 10
		},
		"outside emitter": func(d *world.Document) { d.Lighting.Lights[0].Position.Z = 100 },
		"orphan solid": func(d *world.Document) {
			d.StaticSolids = &world.StaticSolids{Version: 1, Items: []world.Solid{{ID: "orphan", Footprint: []world.Vec2{{X: 100, Y: 100}, {X: 101, Y: 100}, {X: 101, Y: 101}, {X: 100, Y: 101}}, Bottom: world.Plane{}, Top: world.Plane{C: 1}, SideMaterial: 3, TopMaterial: 3, BottomMaterial: 3}}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			document := fixture(t)
			document.Version = world.Version
			document.Sectors[0].Walls[1].PortalWall = 4
			document.Sectors[1].Walls[3].PortalWall = 2
			document.Lighting = &world.Lighting{
				Version: 1,
				Lights:  []world.PointLight{{ID: "light", Position: world.Vec3{X: 2, Y: 2, Z: 2}, Color: world.Vec3{X: 1}, Radius: 10}},
			}
			change(&document)
			if _, err := Compile(document, Options{}); err != nil {
				t.Fatalf("layout-only rejected valid geometry: %v", err)
			}
			if _, err := Compile(document, Options{Light: "light"}); err == nil {
				t.Fatal("accepted unsupported runtime bake")
			}
			if _, err := Compile(document, Options{Lights: []string{"light"}}); err == nil {
				t.Fatal("accepted unsupported directional runtime bake")
			}
		})
	}
}
