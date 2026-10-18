package worldsource

import (
	"fmt"
	"slices"
)

// ExpandCompactRooms copies room slices and creates compact boundaries without
// changing the input or bypassing ordinary validation. Other fields stay shared.
// Compilers call this before prefab expansion; runtimes consume existing sectors.
func ExpandCompactRooms(document *Document) (Document, error) {
	if document == nil {
		return Document{}, ErrReference
	}
	if document.Version < LegacyVersion || document.Version > Version {
		return Document{}, ErrVersion
	}
	if len(document.Rooms) > MaxRooms || len(document.Prefabs) > MaxPrefabs {
		return Document{}, ErrBounds
	}
	result := *document
	var err error
	result.Rooms, err = expandRectangles(document.Version, document.Rooms)
	if err != nil {
		return Document{}, err
	}
	hasCompactPrefabs := slices.ContainsFunc(document.Prefabs, func(prefab Prefab) bool {
		return slices.ContainsFunc(prefab.Rooms, func(room Room) bool { return room.Rect != nil })
	})
	if hasCompactPrefabs {
		result.Prefabs = slices.Clone(document.Prefabs)
		for index := range result.Prefabs {
			result.Prefabs[index].Rooms, err = expandRectangles(document.Version, document.Prefabs[index].Rooms)
			if err != nil {
				return Document{}, fmt.Errorf("prefab %q: %w", result.Prefabs[index].ID, err)
			}
		}
	}
	return result, nil
}

func expandRectangles(version uint16, rooms []Room) ([]Room, error) {
	if len(rooms) > MaxRooms {
		return nil, ErrBounds
	}
	if !slices.ContainsFunc(rooms, func(room Room) bool { return room.Rect != nil }) {
		return rooms, nil
	}
	result := slices.Clone(rooms)
	for index := range result {
		room := &result[index]
		r := room.Rect
		if r == nil {
			continue
		}
		if version < CompactRoomVersion {
			return nil, fmt.Errorf("room %q rect requires source version %d: %w", room.ID, CompactRoomVersion, ErrVersion)
		}
		if room.Boundary != nil || !finite(r.X) || !finite(r.Y) || !finite(r.Width) || !finite(r.Depth) ||
			r.Width < MinEdgeLength || r.Depth < MinEdgeLength || !validIdentifier(r.Material) ||
			!validVec2(Vec2{X: r.X, Y: r.Y}) || !validVec2(Vec2{X: r.X + r.Width, Y: r.Y + r.Depth}) {
			return nil, fmt.Errorf(
				"room %q rect requires positive bounded dimensions, a material and no boundary: %w",
				room.ID,
				ErrGeometry,
			)
		}
		sw, se := Vec2{X: r.X, Y: r.Y}, Vec2{X: r.X + r.Width, Y: r.Y}
		ne, nw := Vec2{X: r.X + r.Width, Y: r.Y + r.Depth}, Vec2{X: r.X, Y: r.Y + r.Depth}
		room.Boundary = []Edge{
			{ID: "south", Start: sw, End: se, Material: r.Material},
			{ID: "east", Start: se, End: ne, Material: r.Material},
			{ID: "north", Start: ne, End: nw, Material: r.Material},
			{ID: "west", Start: nw, End: sw, Material: r.Material},
		}
		room.Rect = nil
	}
	return result, nil
}
