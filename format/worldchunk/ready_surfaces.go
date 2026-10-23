package worldchunk

import (
	"cmp"
	"slices"

	"github.com/karty-game/karty-sdk/format/world"
)

// ReadySurfaces derives a deterministic level-wide material surface table.
// Passing this table to every CompileReady call keeps streamed chunk vertex
// surface IDs stable without rewriting their scalar streams at runtime.
func ReadySurfaces(meshes []Mesh, document *world.Document) ([]ReadySurface, error) {
	if document == nil || len(meshes) == 0 {
		return nil, ErrChunk
	}
	slots := make(map[uint32]int)
	for index, id := range world.MaterialIDs(document) {
		slots[id] = index
	}
	unique := make(map[ReadySurface]struct{})
	for index := range meshes {
		mesh := &meshes[index]
		if err := ValidateMesh(mesh, document); err != nil {
			return nil, err
		}
		for _, triangle := range mesh.Triangles {
			surface, valid := readySurface(triangle, slots)
			if !valid {
				return nil, ErrChunk
			}
			unique[surface] = struct{}{}
		}
	}
	if len(unique) == 0 || len(unique) > MaxTriangles {
		return nil, ErrChunk
	}
	result := make([]ReadySurface, 0, len(unique))
	for surface := range unique {
		result = append(result, surface)
	}
	slices.SortFunc(result, compareReadySurfaces)
	return result, nil
}

func compareReadySurfaces(left, right ReadySurface) int {
	for _, comparison := range [...]int{
		cmp.Compare(left.Primary, right.Primary),
		cmp.Compare(left.Secondary, right.Secondary),
		cmp.Compare(left.Band, right.Band),
		cmp.Compare(left.SecondaryStrength, right.SecondaryStrength),
		cmp.Compare(left.BandCoverage, right.BandCoverage),
	} {
		if comparison != 0 {
			return comparison
		}
	}
	if left.BandRepeatU != right.BandRepeatU {
		if left.BandRepeatU {
			return 1
		}
		return -1
	}
	if left.BandRepeatV != right.BandRepeatV {
		if left.BandRepeatV {
			return 1
		}
		return -1
	}
	return 0
}

func readySurfaceSlots(surfaces []ReadySurface, materials int) (map[ReadySurface]int, error) {
	if len(surfaces) == 0 || len(surfaces) > MaxTriangles {
		return nil, ErrChunk
	}
	slots := make(map[ReadySurface]int, len(surfaces))
	for index, surface := range surfaces {
		if surface.Primary < 0 || surface.Primary >= materials || surface.Secondary < 0 || surface.Secondary >= materials ||
			surface.Band < 0 || surface.Band >= materials || !finite(float64(surface.SecondaryStrength)) || surface.SecondaryStrength < 0 || surface.SecondaryStrength > 1 || surface.BandCoverage > 2 {
			return nil, ErrChunk
		}
		if _, found := slots[surface]; found {
			return nil, ErrChunk
		}
		slots[surface] = index
	}
	return slots, nil
}
