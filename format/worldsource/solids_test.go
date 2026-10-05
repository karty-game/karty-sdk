package worldsource_test

import (
	"errors"
	"fmt"
	"github.com/karty-game/karty-sdk/format/worldsource"
	"testing"
)

func sourceSolid() worldsource.Solid {
	return worldsource.Solid{ID: "pillar", Footprint: []worldsource.Vec2{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}}, Top: worldsource.Plane{C: 2}, SideMaterial: "marble", TopMaterial: "marble", BottomMaterial: "marble"}
}

func sourceSolidBatch(count int) []worldsource.Solid {
	items := make([]worldsource.Solid, count)
	for index := range items {
		items[index] = sourceSolid()
		items[index].ID = fmt.Sprintf("pillar-%04d", index)
	}
	return items
}

func TestSourceThousandPillarsAndAggregateCountBoundary(t *testing.T) {
	for _, count := range []int{1000, 1024} {
		document := validSource()
		document.Version = 6
		document.Solids = sourceSolidBatch(count)
		if err := worldsource.Validate(&document); err != nil {
			t.Fatalf("source v6 rejected %d pillars: %v", count, err)
		}
		document.Solids[count-1].SideMaterial = ""
		if err := worldsource.Validate(&document); !errors.Is(err, worldsource.ErrReference) {
			t.Fatalf("invalid final material accepted: %v", err)
		}
	}
	document := validSource()
	document.Version = 6
	document.Solids = sourceSolidBatch(1025)
	if err := worldsource.Validate(&document); !errors.Is(err, worldsource.ErrBounds) {
		t.Fatalf("source accepted 1,025 root solids: %v", err)
	}
	document.Solids = sourceSolidBatch(512)
	document.Prefabs = append(document.Prefabs, worldsource.Prefab{ID: "pillars", Solids: sourceSolidBatch(512)})
	if err := worldsource.Validate(&document); err != nil {
		t.Fatalf("root/prefab aggregate rejected 1,024 solids: %v", err)
	}
	document.Prefabs[len(document.Prefabs)-1].Solids = sourceSolidBatch(513)
	if err := worldsource.Validate(&document); !errors.Is(err, worldsource.ErrBounds) {
		t.Fatalf("source accepted root/prefab aggregate of 1,025 solids: %v", err)
	}
}

func TestSourceDetailOnlyPrefabsVersioned(t *testing.T) {
	for _, kind := range []string{"solid", "sprite"} {
		t.Run(kind, func(t *testing.T) {
			document := validSource()
			document.Version = 6
			prefab := worldsource.Prefab{ID: "detail"}
			if kind == "solid" {
				prefab.Solids = []worldsource.Solid{sourceSolid()}
			} else {
				prefab.Contents = []worldsource.Content{{ID: "sprite", Kind: "decoration", Position: worldsource.Vec3{}, Actor: &worldsource.Actor{Sprite: &worldsource.Sprite{Texture: "art", Facing: "upright", Alpha: "cutout", Width: 1, Height: 2}}}}
			}
			document.Prefabs = append(document.Prefabs, prefab)
			if err := worldsource.Validate(&document); err != nil {
				t.Fatal(err)
			}
			for version := uint16(1); version < 6; version++ {
				document.Version = version
				if err := worldsource.Validate(&document); !errors.Is(err, worldsource.ErrVersion) {
					t.Fatalf("v%d accepted detail-only prefab: %v", version, err)
				}
			}
		})
	}
}

func TestSourceSolidsSideMappingAndCompleteValidation(t *testing.T) {
	document := validSource()
	document.Version = 6
	document.Solids = []worldsource.Solid{sourceSolid(), sourceSolid()}
	document.Solids[1].ID = "last"
	document.Solids[1].SideUV = &worldsource.UVSettings{Mode: worldsource.UVTriplanar}
	if err := worldsource.Validate(&document); !errors.Is(err, worldsource.ErrUVMapping) {
		t.Fatalf("per-edge-inexpressible mapping accepted: %v", err)
	}
	document.Solids[1].SideUV = nil
	document.Solids[1].Top = worldsource.Plane{}
	if err := worldsource.Validate(&document); err == nil {
		t.Fatal("invalid last solid accepted")
	}
}
