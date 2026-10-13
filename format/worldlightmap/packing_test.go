package worldlightmap

import (
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
)

func TestCompactPackingRotatesChartsWithoutChangingDensityOrGeometry(t *testing.T) {
	document := fixture(t)
	document.Contents = nil
	document.Version = world.Version
	document.Environment = &world.Environment{Version: 1, Preset: world.EnvironmentDaylight}
	for i, extent := range [][2]float64{{12, 24}, {24, 12}} {
		sector := &document.Sectors[i]
		sector.Floor, sector.Ceiling, sector.Sky = world.Plane{}, world.Plane{C: 30}, true
		x := float64(i) * 100
		points := []world.Vec2{{X: x}, {X: x + extent[0]}, {X: x + extent[0], Y: extent[1]}, {X: x, Y: extent[1]}}
		sector.Walls = nil
		for edge, start := range points {
			sector.Walls = append(sector.Walls, world.Wall{Start: start, End: points[(edge+1)%4], Portal: -1, Sky: true})
		}
	}
	options := Options{TexelsPerUnit: 16, PageSize: 512, Padding: 4}
	if _, err := Compile(document, options); !errors.Is(err, ErrPageCapacity) {
		t.Fatalf("legacy shelves should not fit these complementary charts: %v", err)
	}
	options.CompactPacking = true
	layout, err := Compile(document, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(layout.Charts) != 2 || layout.Pages[0] != (Page{512, 512}) || layout.TexelsPerUnit != 16 {
		t.Fatal("compact packing changed chart coverage, density or page limits")
	}
	if layout.Charts[0].Tangent == layout.Charts[1].Tangent {
		t.Fatal("complementary chart did not rotate")
	}
	for _, chart := range layout.Charts {
		basisError := sub(cross(chart.Tangent, chart.Bitangent), chart.Normal)
		if dot(basisError, basisError) > 1e-20 {
			t.Fatal("rotation changed handedness or surface normal")
		}
		for _, plane := range [][4]float64{chart.UPlane, chart.VPlane} {
			density := math.Sqrt(plane[0]*plane[0]+plane[1]*plane[1]+plane[2]*plane[2]) * 512
			if math.Abs(density-16) > 1e-10 {
				t.Fatal("rotation changed affine texel density")
			}
		}
	}
	encoded, err := Encode(layout, &document)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(encoded, &document)
	if err != nil || !reflect.DeepEqual(layout, decoded) {
		t.Fatalf("rotated layout failed complete geometry/UV validation: %v", err)
	}
	again, err := Compile(document, options)
	if err != nil || !reflect.DeepEqual(layout, again) {
		t.Fatalf("rotating packing was nondeterministic: %v", err)
	}
}

func TestCapsWithinSourceRoomPreservesPrefabInstanceBoundaries(t *testing.T) {
	document := fixture(t)
	document.Sectors[1].Floor = document.Sectors[0].Floor
	document.Sectors[1].Ceiling = document.Sectors[0].Ceiling
	options := Options{CapsWithinSourceRoom: true}
	for _, sameInstance := range []bool{false, true} {
		document.Sectors[0].SourceRoom, document.Sectors[1].SourceRoom = "shared", "shared"
		document.Sectors[0].Instance, document.Sectors[1].Instance = "first", "second"
		if sameInstance {
			document.Sectors[1].Instance = "first"
		}
		layout, err := Compile(document, options)
		if err != nil {
			t.Fatal(err)
		}
		var caps []int
		for _, binding := range layout.Bindings {
			if binding.Kind == "sector-floor" {
				caps = append(caps, binding.Chart)
			}
		}
		if (caps[0] == caps[1]) != sameInstance {
			t.Fatal("cap policy did not preserve authored room/instance boundaries")
		}
		if _, err := Encode(layout, &document); err != nil {
			t.Fatal(err)
		}
	}
}
