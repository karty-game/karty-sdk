package worldlightmap

import (
	"fmt"
	"slices"
	"strings"

	"github.com/karty-game/karty-sdk/format/world"
)

func receiverSelected(binding Binding, selection *ReceiverSelection) bool {
	if strings.HasPrefix(binding.Kind, "sector-") {
		_, found := slices.BinarySearch(selection.Sectors, binding.Index)
		return found
	}
	_, found := slices.BinarySearch(selection.Solids, binding.Index)
	return found
}

// LightIDsInReceiverScope preserves the given source order while filtering to
// the receivers' ordinary physical component. Unknown and duplicate IDs fail
// even when their source would be outside that component.
func LightIDsInReceiverScope(document *world.Document, selection *ReceiverSelection, ids []string) ([]string, error) {
	if len(ids) > MaxBakeLights {
		return nil, ErrLayout
	}
	scope, _, err := ReceiverScope(document, selection)
	if err != nil {
		return nil, err
	}
	if document.Lighting == nil {
		return nil, ErrLayout
	}
	component := *document
	component.Sectors = make([]world.Sector, 0)
	for index, included := range scope {
		if included {
			component.Sectors = append(component.Sectors, document.Sectors[index])
		}
	}
	seen := make(map[string]bool, len(ids))
	sources := make(map[string]world.PointLight, len(document.Lighting.Lights))
	for _, light := range document.Lighting.Lights {
		sources[light.ID] = light
	}
	filtered := make([]string, 0, len(ids))
	for _, id := range ids {
		if seen[id] {
			return nil, fmt.Errorf("duplicate bake light: %w", ErrLayout)
		}
		seen[id] = true
		light, found := sources[id]
		if !found {
			return nil, fmt.Errorf("unknown bake light: %w", ErrLayout)
		}
		if runtimeEmitterInside(&component, light.Position) {
			filtered = append(filtered, id)
		}
	}
	return filtered, nil
}

// ReceiverScope returns transport geometry in the selected receivers' ordinary
// physical component. Transformed and one-way portals are lighting boundaries.
// The caller must validate the complete document before using these masks.
// Solids touching multiple physical spaces are rejected: coordinate overlap
// cannot unambiguously assign an independently authored volume to either space.
func ReceiverScope(document *world.Document, selection *ReceiverSelection) ([]bool, []bool, error) {
	if err := world.Validate(document); err != nil {
		return nil, nil, err
	}
	if document == nil || selection == nil || len(selection.Sectors) == 0 || len(selection.Sectors) > len(document.Sectors) {
		return nil, nil, ErrLayout
	}
	if !orderedIndexes(selection.Sectors, len(document.Sectors)) {
		return nil, nil, ErrLayout
	}
	solidCount := 0
	if document.StaticSolids != nil {
		solidCount = len(document.StaticSolids.Items)
	}
	if !orderedIndexes(selection.Solids, solidCount) {
		return nil, nil, ErrLayout
	}
	visited := make([]bool, len(document.Sectors))
	queue := []int{selection.Sectors[0]}
	visited[queue[0]] = true
	for next := 0; next < len(queue); next++ {
		sector := queue[next]
		for edge, wall := range document.Sectors[sector].Walls {
			if ordinaryPortal(document, sector, edge, wall) && !visited[wall.Portal] {
				visited[wall.Portal] = true
				queue = append(queue, int(wall.Portal))
			}
		}
	}
	for _, sector := range selection.Sectors {
		if !visited[sector] {
			return nil, nil, fmt.Errorf("receivers span physical spaces: %w", ErrLayout)
		}
	}
	solids := make([]bool, solidCount)
	rooms := make([][4]float64, len(document.Sectors))
	for i, sector := range document.Sectors {
		points := make([]world.Vec2, len(sector.Walls))
		for j, wall := range sector.Walls {
			points[j] = wall.Start
		}
		rooms[i] = xyBounds(points)
	}
	if document.StaticSolids != nil {
		for i, solid := range document.StaticSolids.Items {
			bounds := xyBounds(solid.Footprint)
			inside, outside := false, false
			for j, room := range rooms {
				if bounds[0] <= room[2]+1e-9 && bounds[2] >= room[0]-1e-9 && bounds[1] <= room[3]+1e-9 && bounds[3] >= room[1]-1e-9 {
					if visited[j] {
						inside = true
					} else {
						outside = true
					}
				}
			}
			if inside && outside {
				return nil, nil, fmt.Errorf("solid %q touches multiple physical spaces: %w", solid.ID, ErrLayout)
			}
			solids[i] = inside
		}
	}
	for _, solid := range selection.Solids {
		if !solids[solid] {
			return nil, nil, fmt.Errorf("solid receiver outside physical space: %w", ErrLayout)
		}
	}
	return visited, solids, nil
}

func orderedIndexes(indexes []int, count int) bool {
	for i, index := range indexes {
		if index < 0 || index >= count || (i > 0 && index <= indexes[i-1]) {
			return false
		}
	}
	return true
}
