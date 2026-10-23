package worldchunk

import (
	"crypto/sha256"
	"encoding/hex"
	"math"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

type readyMaterial struct {
	liquid, transparent, invisible, masked bool
	amplitude                              float64
}

type readyBatchKey struct {
	static, sky, outdoor, transparent, masked bool
	winding                                   int8
	surface                                   int
}

// CompileReady groups and packs already tessellated source triangles at build
// time. Optional static cleanup cuts covered geometry while interpolating the
// source attributes; canonical source geometry and lighting remain unchanged.
func CompileReady(
	mesh *Mesh,
	encodedMesh []byte,
	document *world.Document,
	layout *worldlightmap.Layout,
	options ReadyOptions,
) (Ready, error) {
	if err := ValidateMesh(mesh, document); err != nil {
		return Ready{}, err
	}
	if err := ValidateMeshBinding(mesh, layout); err != nil {
		return Ready{}, err
	}
	materials, slots, err := readyMaterials(document, options)
	if err != nil {
		return Ready{}, err
	}
	digest := sha256.Sum256(encodedMesh)
	ready := Ready{Schema: ReadySchema, MeshSHA256: hex.EncodeToString(digest[:])}
	table := options.Surfaces
	if table == nil {
		table, err = ReadySurfaces([]Mesh{*mesh}, document)
		if err != nil {
			return Ready{}, err
		}
	}
	surfaces, err := readySurfaceSlots(table, len(slots))
	if err != nil {
		return Ready{}, err
	}
	ready.Surfaces = append([]ReadySurface(nil), table...)
	if options.CleanupStatic {
		derived, err := cleanupReadyMesh(mesh, document, materials)
		if err != nil {
			return Ready{}, err
		}
		mesh = &derived
	}
	groups := make(map[readyBatchKey]int)
	for index, triangle := range mesh.Triangles {
		surface, found := readySurface(triangle, slots)
		if !found {
			return Ready{}, ErrChunk
		}
		surfaceID, found := surfaces[surface]
		if !found {
			return Ready{}, ErrChunk
		}
		basis, primary, secondary, static := readyMaterialBasis(triangle, materials)
		material := materials[triangle.Material]
		key := readyBatchKey{static: static, sky: triangle.Sky, outdoor: triangle.Outdoor}
		if static {
			key.winding, key.masked = readyWinding(mesh, index), material.masked
		}
		if material.transparent {
			key = readyBatchKey{static: static, sky: triangle.Sky, outdoor: triangle.Outdoor, transparent: true, surface: surfaceID}
		}
		batchID, found := groups[key]
		if !found {
			batchID = len(ready.Batches)
			groups[key] = batchID
			ready.Batches = append(ready.Batches, ReadyBatch{
				Surface:        surfaceID,
				Sky:            triangle.Sky,
				Outdoor:        triangle.Outdoor,
				StaticMaterial: static,
				Transparent:    material.transparent,
				Invisible:      material.invisible,
				Masked:         key.masked,
				Winding:        key.winding,
			})
		}
		batch := &ready.Batches[batchID]
		first := len(batch.Indices)
		batch.Sources = append(batch.Sources, uint32(index))
		for corner := range 3 {
			vertex := mesh.Vertices[mesh.Indices[index*3+corner]]
			packed := readyVertex(vertex, triangle, primary, secondary, static)
			batch.Indices = append(batch.Indices, uint32(len(batch.SurfaceIDs)))
			batch.Vertices = append(batch.Vertices, packed[:]...)
			batch.SurfaceIDs = append(batch.SurfaceIDs, uint32(surfaceID))
			batch.WaveUV = append(batch.WaveUV, vertex.WaveUV[:]...)
			if layout != nil && !triangle.Sky {
				chart := layout.Charts[triangle.Chart]
				batch.LightmapUV = append(batch.LightmapUV, vertex.LightmapUV[:]...)
				batch.LightmapBasis = append(
					batch.LightmapBasis,
					float32(chart.Tangent.X),
					float32(chart.Tangent.Y),
					float32(chart.Tangent.Z),
					float32(chart.Bitangent.X),
					float32(chart.Bitangent.Y),
					float32(chart.Bitangent.Z),
				)
			}
			if static {
				batch.MaterialBasis = append(batch.MaterialBasis, basis[:]...)
			}
		}
		readyAppendOwner(batch, mesh, index, first, material.amplitude)
	}
	if ReadyBytes(&ready) > MaxReadyBytes {
		return Ready{}, ErrReadyCapacity
	}
	return ready, nil
}

func readyMaterials(document *world.Document, options ReadyOptions) (map[uint32]readyMaterial, map[uint32]int, error) {
	ids := world.MaterialIDs(document)
	if len(options.Masked) != 0 && len(options.Masked) != len(ids) {
		return nil, nil, ErrChunk
	}
	materials, slots := make(map[uint32]readyMaterial, len(ids)), make(map[uint32]int, len(ids))
	for slot, id := range ids {
		slots[id] = slot
		materials[id] = readyMaterial{masked: len(options.Masked) != 0 && options.Masked[slot]}
	}
	if document.Animations != nil {
		for _, binding := range document.Animations.Materials {
			for _, preset := range document.Animations.Presets {
				if preset.Name != binding.Preset || preset.Kind != "liquid" {
					continue
				}
				material := materials[binding.Material]
				material.liquid, material.amplitude = true, preset.SurfaceAmplitude
				if preset.Opacity != nil {
					opacity := math.Round(*preset.Opacity * 255)
					material.transparent, material.invisible = opacity < 255, opacity == 0
				}
				materials[binding.Material] = material
			}
		}
	}
	return materials, slots, nil
}

func readySurface(triangle Triangle, slots map[uint32]int) (ReadySurface, bool) {
	primary, valid := slots[triangle.Material]
	secondary, secondaryValid := slots[triangle.Secondary.Material]
	band, bandValid := slots[triangle.Band.Material]
	strength := triangle.Secondary.Strength
	if strength == 0 {
		strength = 0
	} // Canonical positive zero for material-key identity.
	return ReadySurface{Primary: primary, Secondary: secondary, Band: band, SecondaryStrength: strength,
			BandCoverage: triangle.Band.Coverage, BandRepeatU: triangle.Band.RepeatU, BandRepeatV: triangle.Band.RepeatV},
		valid && (triangle.Secondary.Strength == 0 || secondaryValid) && (triangle.Band.Coverage == 0 || bandValid)
}

func readyAppendOwner(batch *ReadyBatch, mesh *Mesh, index, first int, amplitude float64) {
	triangle := mesh.Triangles[index]
	owner := ReadyOwner{Sector: triangle.Sector, Solid: triangle.Solid, First: first, Count: 3,
		Bounds: [6]float64{math.Inf(1), math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1), math.Inf(-1)}}
	for corner := range 3 {
		vertex := mesh.Vertices[mesh.Indices[index*3+corner]]
		for axis, value := range vertex.Position {
			padding := 0.0
			if axis == 2 {
				padding = math.Abs(amplitude)
			}
			owner.Bounds[axis] = min(owner.Bounds[axis], float64(value)-padding)
			owner.Bounds[axis+3] = max(owner.Bounds[axis+3], float64(value)+padding)
		}
	}
	if count := len(batch.Owners); count > 0 && batch.Owners[count-1].Sector == owner.Sector && batch.Owners[count-1].Solid == owner.Solid {
		previous := &batch.Owners[count-1]
		previous.Count += 3
		for axis := range 3 {
			previous.Bounds[axis] = min(previous.Bounds[axis], owner.Bounds[axis])
			previous.Bounds[axis+3] = max(previous.Bounds[axis+3], owner.Bounds[axis+3])
		}
	} else {
		batch.Owners = append(batch.Owners, owner)
	}
}
