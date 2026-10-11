package worldchunk

import (
	"fmt"
	"slices"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

func validateCatalogBakedLights(catalog *Catalog, document *world.Document) error {
	count := 0
	if document.Lighting != nil {
		count = len(document.Lighting.Lights)
	}
	if !indexes(catalog.BakedLights, count, false) || len(catalog.BakedLights) > world.MaxLights {
		return fmt.Errorf("catalog baked source indexes: %w", ErrChunk)
	}
	if len(catalog.BakedLights) == 0 {
		return nil
	}
	static, err := worldlightmap.StaticLightIDs(document)
	if err != nil {
		return err
	}
	eligible := make(map[string]bool, len(static))
	for _, id := range static {
		eligible[id] = true
	}
	components := physicalComponents(document)
	complete := make([]bool, len(document.Sectors))
	for i := range complete {
		complete[i] = true
	}
	for _, chunk := range catalog.Chunks {
		if !indexes(chunk.Sectors, len(document.Sectors), true) {
			return ErrChunk
		}
		if chunk.LightmapBytes == 0 {
			complete[components[chunk.Sectors[0]]] = false
		}
	}
	for _, index := range catalog.BakedLights {
		light := document.Lighting.Lights[index]
		if !eligible[light.ID] {
			return fmt.Errorf("catalog source %q is dynamic: %w", light.ID, ErrChunk)
		}
		found := false
		for sector, geometry := range document.Sectors {
			if emitterInSector(&geometry, light.Position) {
				found = true
				if !complete[components[sector]] {
					return fmt.Errorf("catalog source %q has an unbaked receiver component: %w", light.ID, ErrChunk)
				}
			}
		}
		if !found {
			return fmt.Errorf("catalog source %q is outside its receiver component: %w", light.ID, ErrChunk)
		}
	}
	return nil
}

// ValidateBakedSelection binds a child's source membership to its parent
// catalog. The recipe must contain exactly the parent union filtered to this
// ordinary physical component; ordering remains part of the prebake identity.
// A nil layout is valid only for a descriptor with no lightmap.
func ValidateBakedSelection(catalog *Catalog, chunkID int, document *world.Document, layout *worldlightmap.Layout) error {
	if err := ValidateCatalog(catalog, document); err != nil {
		return err
	}
	if chunkID < 0 || chunkID >= len(catalog.Chunks) {
		return ErrChunk
	}
	chunk := catalog.Chunks[chunkID]
	if chunk.LightmapBytes == 0 {
		if layout != nil {
			return ErrChunk
		}
		return nil
	}
	if layout == nil || layout.Receivers == nil || !slices.Equal(layout.Receivers.Sectors, chunk.Sectors) ||
		!slices.Equal(layout.Receivers.Solids, chunk.Solids) {
		return ErrChunk
	}
	if err := worldlightmap.Validate(layout, document); err != nil {
		return err
	}
	if layout.RuntimeBake == nil || layout.RuntimeBake.Encoding != worldlightmap.DirectRNMEncoding {
		return ErrChunk
	}
	ids := make([]string, len(catalog.BakedLights))
	for i, index := range catalog.BakedLights {
		ids[i] = document.Lighting.Lights[index].ID
	}
	expected, err := worldlightmap.LightIDsInReceiverScope(document, layout.Receivers, ids)
	if err != nil {
		return err
	}
	if len(expected) != len(layout.RuntimeBake.LightIDs) {
		return fmt.Errorf("child bake source membership differs from catalog: %w", ErrChunk)
	}
	selected := make(map[string]bool, len(expected))
	for _, id := range expected {
		selected[id] = true
	}
	for _, id := range layout.RuntimeBake.LightIDs {
		if !selected[id] {
			return fmt.Errorf("child bake source membership differs from catalog: %w", ErrChunk)
		}
	}
	return nil
}

func physicalComponents(document *world.Document) []int {
	components := make([]int, len(document.Sectors))
	for i := range components {
		components[i] = -1
	}
	queue := make([]int, 0, len(components))
	for start := range components {
		if components[start] >= 0 {
			continue
		}
		queue = append(queue[:0], start)
		components[start] = start
		for next := 0; next < len(queue); next++ {
			sector := queue[next]
			for edge, wall := range document.Sectors[sector].Walls {
				if wall.Portal >= 0 && components[wall.Portal] < 0 && ordinary(document, sector, edge) {
					components[wall.Portal] = start
					queue = append(queue, int(wall.Portal))
				}
			}
		}
	}
	return components
}

func emitterInSector(sector *world.Sector, position world.Vec3) bool {
	if position.Z < sector.Floor.A*position.X+sector.Floor.B*position.Y+sector.Floor.C ||
		position.Z > sector.Ceiling.A*position.X+sector.Ceiling.B*position.Y+sector.Ceiling.C {
		return false
	}
	for _, wall := range sector.Walls {
		if (wall.End.X-wall.Start.X)*(position.Y-wall.Start.Y)-(wall.End.Y-wall.Start.Y)*(position.X-wall.Start.X) < -1e-8 {
			return false
		}
	}
	return true
}
