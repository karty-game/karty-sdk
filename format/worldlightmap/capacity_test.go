package worldlightmap

import (
	"errors"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
)

func TestPageCapacityDistinguishesPackingFromInvalidInputs(t *testing.T) {
	document := fixture(t)
	for name, sectors := range map[string][]int{"oversized chart": {0, 1}, "full page": {0}} {
		t.Run(name, func(t *testing.T) {
			document := fixture(t)
			if name == "oversized chart" {
				for i := range document.Sectors {
					document.Sectors[i].Floor = world.Plane{}
					document.Sectors[i].Ceiling = world.Plane{C: 2}
					for edge := range document.Sectors[i].Walls {
						document.Sectors[i].Walls[edge].Start.Y *= .5
						document.Sectors[i].Walls[edge].End.Y *= .5
					}
				}
				document.Contents[0].Position.Y *= .5
			}
			options := Options{PageSize: 512, TexelsPerUnit: 64}
			if _, err := CompileReceivers(document, options, sectors, nil); !errors.Is(err, ErrPageCapacity) ||
				!errors.Is(err, ErrLayout) {
				t.Fatalf("packing failure lost capacity/layout identity: %v", err)
			}
			options.PageSize = 1024
			if _, err := CompileReceivers(document, options, sectors, nil); err != nil {
				t.Fatalf("same density did not fit larger page: %v", err)
			}
		})
	}
	for name, options := range map[string]Options{
		"page option": {PageSize: 2048}, "density": {TexelsPerUnit: 65},
		"light recipe": {Lights: []string{"unknown"}}, "texel budget": {MaxTexels: 512*512 - 1},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := CompileReceivers(document, options, []int{0}, nil); !errors.Is(err, ErrLayout) ||
				errors.Is(err, ErrPageCapacity) {
				t.Fatalf("semantic failure reported as capacity: %v", err)
			}
		})
	}
}
