package worldsource

import (
	"errors"
	"math"
	"testing"
)

func TestCompactRoomValidatesWithoutMutatingSource(t *testing.T) {
	t.Parallel()
	doc := Document{
		Version: CompactRoomVersion,
		Rooms: []Room{
			{
				ID:              "room",
				Rect:            &RoomRectangle{X: -2, Y: 3, Width: 8, Depth: 6, Material: "stone"},
				Ceiling:         Plane{C: 3},
				FloorMaterial:   "stone",
				CeilingMaterial: "stone",
			},
		},
	}
	if err := Validate(&doc); err != nil {
		t.Fatal(err)
	}
	expanded, err := ExpandCompactRooms(&doc)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Rooms[0].Rect == nil || doc.Rooms[0].Boundary != nil || expanded.Rooms[0].Rect != nil || len(expanded.Rooms[0].Boundary) != 4 {
		t.Fatal("source ownership was lost")
	}
	for i, name := range []string{"south", "east", "north", "west"} {
		if expanded.Rooms[0].Boundary[i].ID != name {
			t.Fatal("unstable edge names")
		}
	}
	if expanded.Rooms[0].Boundary[2].Start != (Vec2{X: 6, Y: 9}) {
		t.Fatal("rectangle coordinates changed")
	}
}

func TestCompactRoomRejectsAmbiguityAndInvalidGeometry(t *testing.T) {
	t.Parallel()
	for _, r := range []RoomRectangle{{Width: 0, Depth: 2, Material: "stone"}, {Width: -1, Depth: 2, Material: "stone"}, {Width: math.Inf(1), Depth: 2, Material: "stone"}, {X: MaxCoordinate, Width: 2, Depth: 2, Material: "stone"}, {Width: 2, Depth: 2}} {
		doc := Document{Version: CompactRoomVersion, Rooms: []Room{{ID: "room", Rect: &r}}}
		if _, err := ExpandCompactRooms(&doc); !errors.Is(err, ErrGeometry) {
			t.Fatalf("%+v: %v", r, err)
		}
	}
	doc := Document{
		Version: CompactRoomVersion,
		Rooms:   []Room{{ID: "room", Rect: &RoomRectangle{Width: 2, Depth: 2, Material: "stone"}, Boundary: []Edge{}}},
	}
	if _, err := ExpandCompactRooms(&doc); !errors.Is(err, ErrGeometry) {
		t.Fatal(err)
	}
	doc.Rooms[0].Boundary = nil
	doc.Version = CompactRoomVersion - 1
	if _, err := ExpandCompactRooms(&doc); !errors.Is(err, ErrVersion) {
		t.Fatal(err)
	}
}

func TestCompactPrefabExpansionKeepsSourceOwnership(t *testing.T) {
	t.Parallel()
	doc := Document{
		Version: CompactRoomVersion,
		Prefabs: []Prefab{{ID: "cell", Rooms: []Room{{ID: "room", Rect: &RoomRectangle{Width: 2, Depth: 3, Material: "stone"}}}}},
	}
	expanded, err := ExpandCompactRooms(&doc)
	if err != nil {
		t.Fatal(err)
	}
	expanded.Prefabs[0].Rooms[0].ID = "changed"
	if doc.Prefabs[0].Rooms[0].ID != "room" || doc.Prefabs[0].Rooms[0].Rect == nil || doc.Prefabs[0].Rooms[0].Boundary != nil {
		t.Fatal("prefab expansion mutated authored source")
	}
}
