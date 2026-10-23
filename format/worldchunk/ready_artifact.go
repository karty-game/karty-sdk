package worldchunk

import (
	"slices"

	"github.com/karty-game/karty-sdk/format/world"
)

// ReadyAdmission binds compiled renderer streams to one accepted parent chunk.
// The asset pipeline proves geometry and authored attributes before publishing
// that parent's catalog. Callers must verify the complete child's exact catalog
// size and SHA-256 before using this structural admission API. This descriptor
// alone does not establish artifact authenticity or producer correctness.
type ReadyAdmission struct {
	MaterialCount   int
	Sectors, Solids []int
	// Triangles is the catalog's canonical mesh-byte reservation divided by 484.
	Triangles int
	// Lightmap requires packed UV/basis streams on every non-sky batch. Image and
	// layout dimensions/format/decoded-byte limits remain their own boundary.
	Lightmap bool
	// Surfaces optionally requires the mount's already accepted canonical table.
	Surfaces []ReadySurface
}

// DecodeReadyArtifact decodes compiler-authoritative renderer data after whole
// artifact integrity verification. It checks binary/allocation limits, finite
// scalars and complete array/material/owner references. It does not decode a
// semantic mesh, reconstruct geometry, prove surface coverage, regenerate UVs
// or compare normals/bases against authored source. MeshSHA256 is build evidence;
// the verified whole-child catalog hash is the runtime artifact binding.
// Returned arrays are independently owned; inputs are read-only.
func DecodeReadyArtifact(encoded []byte, admission ReadyAdmission) (Ready, error) {
	ready, err := decodeReadyBinary(encoded)
	if err != nil {
		return Ready{}, err
	}
	if err := validateReadyArtifact(&ready, admission); err != nil {
		return Ready{}, err
	}
	return ready, nil
}

func validateReadyArtifact(ready *Ready, admission ReadyAdmission) error {
	if admission.MaterialCount <= 0 || admission.Triangles <= 0 || admission.Triangles > MaxTriangles ||
		!indexes(admission.Sectors, world.MaxSectors, true) || !indexes(admission.Solids, world.MaxStaticSolids, false) {
		return ErrChunk
	}
	if _, err := readySurfaceSlots(ready.Surfaces, admission.MaterialCount); err != nil {
		return err
	}
	if admission.Surfaces != nil && !slices.Equal(admission.Surfaces, ready.Surfaces) {
		return ErrChunk
	}
	batches := make(map[readyBatchKey]struct{}, len(ready.Batches))
	triangles := 0
	for index, batch := range ready.Batches {
		triangles += len(batch.Sources)
		if triangles > admission.Triangles || batch.SurfaceIDs[0] != uint32(batch.Surface) ||
			index > 0 && batch.Sources[0] <= ready.Batches[index-1].Sources[0] {
			return ErrChunk
		}
		key := readyClassification(batch)
		if _, duplicate := batches[key]; duplicate {
			return ErrChunk
		}
		batches[key] = struct{}{}
		for at, source := range batch.Sources {
			if uint64(source) >= uint64(admission.Triangles) || at > 0 && source <= batch.Sources[at-1] {
				return ErrChunk
			}
			first := batch.SurfaceIDs[at*3]
			if batch.SurfaceIDs[at*3+1] != first || batch.SurfaceIDs[at*3+2] != first ||
				batch.Transparent && first != uint32(batch.Surface) {
				return ErrChunk
			}
		}
		if (admission.Lightmap && !batch.Sky) != (len(batch.LightmapUV) != 0) {
			return ErrChunk
		}
		if err := validateReadyArtifactOwners(batch, admission); err != nil {
			return err
		}
	}
	if triangles != admission.Triangles {
		return ErrChunk
	}
	return nil
}

func readyClassification(batch ReadyBatch) readyBatchKey {
	key := readyBatchKey{
		static:      batch.StaticMaterial,
		sky:         batch.Sky,
		outdoor:     batch.Outdoor,
		transparent: batch.Transparent,
		winding:     batch.Winding,
		masked:      batch.Masked,
	}
	if batch.Transparent {
		key.surface = batch.Surface
	}
	return key
}

func validateReadyArtifactOwners(batch ReadyBatch, admission ReadyAdmission) error {
	next := 0
	for _, owner := range batch.Owners {
		if owner.First != next || owner.Count <= 0 || owner.Count%3 != 0 || owner.Count > len(batch.Indices)-next ||
			!validBounds(owner.Bounds) {
			return ErrChunk
		}
		switch {
		case owner.Sector >= 0 && owner.Solid == -1:
			if _, found := slices.BinarySearch(admission.Sectors, owner.Sector); !found {
				return ErrChunk
			}
		case owner.Solid >= 0 && owner.Sector == -1:
			if _, found := slices.BinarySearch(admission.Solids, owner.Solid); !found {
				return ErrChunk
			}
		default:
			return ErrChunk
		}
		next += owner.Count
	}
	if next != len(batch.Indices) {
		return ErrChunk
	}
	return nil
}
