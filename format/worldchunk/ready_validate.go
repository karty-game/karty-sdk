package worldchunk

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"slices"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

// DecodeReady proves renderer streams against an already validated semantic
// mesh and its accepted lightmap binding. Builders and offline cache acceptance
// use this complete source proof before publishing the whole-child catalog hash.
// Runtime compiled artifacts use DecodeReadyArtifact after integrity checking.
func DecodeReady(
	encoded []byte,
	mesh *Mesh,
	encodedMesh []byte,
	document *world.Document,
	layout *worldlightmap.Layout,
	options ReadyOptions,
) (Ready, error) {
	ready, err := decodeReadyBinary(encoded)
	if err != nil {
		return Ready{}, err
	}
	if err := ValidateReady(&ready, mesh, encodedMesh, document, layout, options); err != nil {
		return Ready{}, err
	}
	return ready, nil
}

// ValidateReady checks complete source coverage and exact packed values without
// constructing a second renderer representation. Inputs are read-only.
func ValidateReady(
	ready *Ready,
	mesh *Mesh,
	encodedMesh []byte,
	document *world.Document,
	layout *worldlightmap.Layout,
	options ReadyOptions,
) error {
	if err := readyShape(ready); err != nil {
		return err
	}
	if mesh == nil || document == nil || len(mesh.Triangles) == 0 || len(mesh.Triangles) > MaxTriangles ||
		len(mesh.Vertices) != len(mesh.Triangles)*3 || len(mesh.Indices) != len(mesh.Vertices) {
		return ErrChunk
	}
	for index, value := range mesh.Indices {
		if value != uint32(index) {
			return ErrChunk
		}
	}
	digest := sha256.Sum256(encodedMesh)
	if ready.MeshSHA256 != hex.EncodeToString(digest[:]) {
		return ErrChunk
	}
	materials, slots, err := readyMaterials(document, options)
	if err != nil {
		return err
	}
	if options.CleanupStatic {
		if err := ValidateMesh(mesh, document); err != nil {
			return err
		}
		if err := ValidateMeshBinding(mesh, layout); err != nil {
			return err
		}
		derived, err := cleanupReadyMesh(mesh, document, materials)
		if err != nil {
			return err
		}
		mesh = &derived
	}
	if _, err := readySurfaceSlots(ready.Surfaces, len(slots)); err != nil {
		return err
	}
	if options.Surfaces != nil && !slices.Equal(options.Surfaces, ready.Surfaces) {
		return ErrChunk
	}

	seen := make([]bool, len(mesh.Triangles))
	batches := make(map[readyBatchKey]struct{}, len(ready.Batches))
	for index, batch := range ready.Batches {
		if index > 0 && batch.Sources[0] <= ready.Batches[index-1].Sources[0] {
			return ErrChunk
		}
		key := readyClassification(batch)
		if _, found := batches[key]; found {
			return ErrChunk
		}
		batches[key] = struct{}{}
		if err := validateReadyBatch(batch, ready.Surfaces, mesh, layout, materials, slots, seen); err != nil {
			return err
		}
	}
	for _, found := range seen {
		if !found {
			return ErrChunk
		}
	}
	return nil
}

func validateReadyBatch(batch ReadyBatch, surfaces []ReadySurface, mesh *Mesh, layout *worldlightmap.Layout,
	materials map[uint32]readyMaterial, slots map[uint32]int, seen []bool,
) error {
	for index, source := range batch.Sources {
		if uint64(source) >= uint64(len(mesh.Triangles)) || seen[source] || index > 0 && source <= batch.Sources[index-1] {
			return ErrChunk
		}
		seen[source] = true
		triangle := mesh.Triangles[source]
		surface, valid := readySurface(triangle, slots)
		basis, primary, secondary, static := readyMaterialBasis(triangle, materials)
		material := materials[triangle.Material]
		winding, masked := int8(0), false
		if static && !material.transparent {
			winding, masked = readyWinding(mesh, int(source)), material.masked
		}
		if !valid || batch.StaticMaterial != static || batch.Sky != triangle.Sky || batch.Outdoor != triangle.Outdoor ||
			batch.Transparent != material.transparent || batch.Invisible != material.invisible || batch.Winding != winding || batch.Masked != masked ||
			(index == 0 || material.transparent) && surfaces[batch.Surface] != surface {
			return ErrChunk
		}
		for corner := range 3 {
			at := index*3 + corner
			vertex := mesh.Vertices[int(source)*3+corner]
			packed := readyVertex(vertex, triangle, primary, secondary, static)
			if surfaces[batch.SurfaceIDs[at]] != surface || !slices.Equal(batch.Vertices[at*ReadyStride:(at+1)*ReadyStride], packed[:]) ||
				!slices.Equal(batch.WaveUV[at*2:at*2+2], vertex.WaveUV[:]) ||
				static && !slices.Equal(batch.MaterialBasis[at*8:at*8+8], basis[:]) {
				return ErrChunk
			}
			if err := validateReadyLightmap(batch, at, vertex, triangle, layout); err != nil {
				return err
			}
		}
	}
	return validateReadyOwners(batch, mesh, materials)
}

func validateReadyLightmap(batch ReadyBatch, at int, vertex Vertex, triangle Triangle, layout *worldlightmap.Layout) error {
	baked := layout != nil && !triangle.Sky
	if !baked {
		if len(batch.LightmapUV) != 0 || len(batch.LightmapBasis) != 0 {
			return ErrChunk
		}
		return nil
	}
	if triangle.Chart < 0 || triangle.Chart >= len(layout.Charts) || len(batch.LightmapUV) == 0 ||
		!slices.Equal(batch.LightmapUV[at*2:at*2+2], vertex.LightmapUV[:]) {
		return ErrChunk
	}
	chart := layout.Charts[triangle.Chart]
	basis := [6]float32{float32(chart.Tangent.X), float32(chart.Tangent.Y), float32(chart.Tangent.Z),
		float32(chart.Bitangent.X), float32(chart.Bitangent.Y), float32(chart.Bitangent.Z)}
	if !slices.Equal(batch.LightmapBasis[at*6:at*6+6], basis[:]) {
		return ErrChunk
	}
	return nil
}

func validateReadyOwners(batch ReadyBatch, mesh *Mesh, materials map[uint32]readyMaterial) error {
	next := 0
	for _, owner := range batch.Owners {
		if owner.First != next || owner.Count <= 0 || owner.Count%3 != 0 || owner.Count > len(batch.Indices)-next ||
			!validBounds(owner.Bounds) {
			return ErrChunk
		}
		bounds := [6]float64{math.Inf(1), math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1), math.Inf(-1)}
		for at := next / 3; at < (next+owner.Count)/3; at++ {
			index := batch.Sources[at]
			source := mesh.Triangles[index]
			if source.Sector != owner.Sector || source.Solid != owner.Solid {
				return ErrChunk
			}
			for corner := range 3 {
				for axis, value := range mesh.Vertices[int(index)*3+corner].Position {
					padding := 0.0
					if axis == 2 {
						padding = math.Abs(materials[source.Material].amplitude)
					}
					bounds[axis] = min(bounds[axis], float64(value)-padding)
					bounds[axis+3] = max(bounds[axis+3], float64(value)+padding)
				}
			}
		}
		if owner.Bounds != bounds {
			return ErrChunk
		}
		next += owner.Count
	}
	if next != len(batch.Indices) {
		return ErrChunk
	}
	return nil
}
