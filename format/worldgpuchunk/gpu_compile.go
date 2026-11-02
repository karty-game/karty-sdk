package worldgpuchunk

import (
	"encoding/binary"
	"math"
	"slices"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldchunk"
	"github.com/karty-game/karty-sdk/format/worldmaterial"
)

// CompileOptions provides only immutable, offline lighting/ownership inputs.
// Probe values are one-based slots in the shared root reflection resource.
type CompileOptions struct {
	RGBMRange                 float64
	SectorProbes, SolidProbes []uint32
	SolidSectors              [][]uint32
	CellSectors               []uint32
	SkipTerrainSectors        []uint32
}

type compileGroup struct {
	material, owner, kind, surface uint32
}
type compileMesh struct {
	key      compileGroup
	vertices []Vertex
	indices  []uint32
	unique   map[Vertex]uint32
	sectors  []uint32
	bounds   [6]float64
}

// CompileReady converts source-oriented Ready streams into the final cell ABI.
// It runs only in the asset build. Runtime code consumes GPUData unchanged.
func CompileReady(
	ready worldchunk.Ready,
	document *world.Document,
	materials worldmaterial.Layout,
	options CompileOptions,
) (GPUData, error) {
	if document == nil || materials.Validate() != nil || !finite(options.RGBMRange) || options.RGBMRange < 1 ||
		options.RGBMRange > math.MaxFloat32 {
		return GPUData{}, ErrChunk
	}
	if _, err := worldchunk.EncodeReady(ready); err != nil {
		return GPUData{}, err
	}
	meshes := []compileMesh{}
	groups := map[compileGroup]int{}
	for _, batch := range ready.Batches {
		if batch.Sky || batch.Invisible {
			continue
		}
		for _, owner := range batch.Owners {
			if owner.First < 0 || owner.Count <= 0 || owner.Count%3 != 0 || owner.First > len(batch.Indices) ||
				owner.Count > len(batch.Indices)-owner.First {
				return GPUData{}, ErrChunk
			}
			for first := owner.First; first < owner.First+owner.Count; first += 3 {
				if err := compileTriangle(&meshes, groups, batch, owner, first, ready.Surfaces, materials, options, document); err != nil {
					return GPUData{}, err
				}
			}
		}
	}
	return packCompileMeshes(meshes, document, materials, options)
}

func compileTriangle(
	meshes *[]compileMesh,
	groups map[compileGroup]int,
	batch worldchunk.ReadyBatch,
	owner worldchunk.ReadyOwner,
	first int,
	surfaces []worldchunk.ReadySurface,
	materials worldmaterial.Layout,
	options CompileOptions,
	document *world.Document,
) error {
	if first < 0 || first+3 > len(batch.Indices) || owner.Count%3 != 0 {
		return ErrChunk
	}
	if uint64(batch.Indices[first]) >= uint64(len(batch.SurfaceIDs)) ||
		uint64(batch.Indices[first]) >= uint64(len(batch.Vertices)/worldchunk.ReadyStride) {
		return ErrChunk
	}
	vertexID := int(batch.Indices[first])
	surfaceID := batch.SurfaceIDs[vertexID]
	if uint64(surfaceID) >= uint64(len(surfaces)) {
		return ErrChunk
	}
	surface := surfaces[surfaceID]
	if surface.SecondaryStrength > 0 || surface.BandCoverage == 1 {
		return ErrChunk
	}
	material := surface.Primary
	band := surface.BandCoverage == 2
	if band {
		material = surface.Band
	}
	if material < 0 || material >= len(materials.Materials) {
		return ErrChunk
	}
	z := batch.Vertices[vertexID*worldchunk.ReadyStride+5]
	if owner.Solid < 0 && owner.Sector >= 0 && z > 0 && slices.Contains(options.SkipTerrainSectors, uint32(owner.Sector)) {
		return nil
	}
	key, sectors, err := compileOwner(owner, uint32(material), z, options)
	if err != nil {
		return err
	}
	index, exists := groups[key]
	if !exists {
		if len(*meshes) >= MaxRecords {
			return ErrChunk
		}
		index = len(*meshes)
		groups[key] = index
		*meshes = append(
			*meshes,
			compileMesh{
				key:     key,
				sectors: sectors,
				unique:  map[Vertex]uint32{},
				bounds:  [6]float64{math.Inf(1), math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1), math.Inf(-1)},
			},
		)
	}
	mesh := &(*meshes)[index]
	tangent, err := compileTangent(batch, first, band)
	if err != nil {
		return err
	}
	for corner := range 3 {
		vertex, err := compileVertex(batch, int(batch.Indices[first+corner]), band, tangent)
		if err != nil {
			return err
		}
		compileWaterMotion(&vertex, document, materials.Materials[material].MaterialID)
		id, exists := mesh.unique[vertex]
		if !exists {
			if len(mesh.vertices) >= MaxVertices {
				return ErrChunk
			}
			id = uint32(len(mesh.vertices))
			mesh.unique[vertex] = id
			mesh.vertices = append(mesh.vertices, vertex)
			for axis, value := range vertex.Position {
				mesh.bounds[axis] = min(mesh.bounds[axis], float64(value))
				mesh.bounds[axis+3] = max(mesh.bounds[axis+3], float64(value))
			}
			if vertex.Motion[0] == 2 {
				mesh.bounds[2] = min(mesh.bounds[2], float64(vertex.Position[2])-float64(vertex.Motion[1]))
				mesh.bounds[5] = max(mesh.bounds[5], float64(vertex.Position[2])+float64(vertex.Motion[1]))
			}
		}
		if len(mesh.indices) >= MaxIndices {
			return ErrChunk
		}
		mesh.indices = append(mesh.indices, id)
	}
	return nil
}

func compileOwner(owner worldchunk.ReadyOwner, material uint32, normalZ float32, options CompileOptions) (compileGroup, []uint32, error) {
	key := compileGroup{material: material}
	if normalZ > 0 {
		key.surface = 1
	} else if normalZ < 0 {
		key.surface = 2
	}
	if owner.Solid >= 0 {
		key.kind, key.owner = OwnerSolid, uint32(owner.Solid)
		sectors := options.CellSectors
		if owner.Solid < len(options.SolidSectors) {
			sectors = options.SolidSectors[owner.Solid]
		}
		if len(sectors) == 0 {
			return key, nil, ErrChunk
		}
		return key, slices.Clone(sectors), nil
	}
	if owner.Sector < 0 {
		return key, nil, ErrChunk
	}
	key.owner = uint32(owner.Sector)
	return key, []uint32{key.owner}, nil
}

func packCompileMeshes(
	meshes []compileMesh,
	document *world.Document,
	materials worldmaterial.Layout,
	options CompileOptions,
) (GPUData, error) {
	result := GPUData{}
	var draws []compileDrawGroup
	owners := make(map[[3]uint32]int)
	indexCount := 0
	for _, mesh := range meshes {
		if len(result.Vertices)/VertexStride+len(mesh.vertices) > MaxVertices ||
			indexCount+len(mesh.indices) > MaxIndices {
			return GPUData{}, ErrChunk
		}
		index := uint32(len(result.RecordRefs))
		base := uint32(len(result.Vertices) / VertexStride)
		for _, vertex := range mesh.vertices {
			result.Vertices = AppendVertex(result.Vertices, vertex)
			result.Addresses = binary.LittleEndian.AppendUint32(result.Addresses, base)
			result.Addresses = binary.LittleEndian.AppendUint32(result.Addresses, index)
			base++
		}
		addressBase := uint32(len(result.Addresses)/AddressStride - len(mesh.vertices))
		indexCount += len(mesh.indices)
		ref := RecordRef{
			MaterialID:  materials.Materials[mesh.key.material].MaterialID,
			OwnerKind:   mesh.key.kind,
			OwnerID:     mesh.key.owner,
			SurfaceKind: mesh.key.surface,
		}
		record := compileRecord(ref, mesh.key.material, document, materials, options)
		var err error
		result.Records, err = AppendStaticRecord(result.Records, record)
		if err != nil {
			return GPUData{}, err
		}
		result.RecordRefs = append(result.RecordRefs, ref)
		if err := appendCompileDraw(&draws, owners, mesh, index, addressBase, record.AlphaMode); err != nil {
			return GPUData{}, err
		}
	}
	slices.SortStableFunc(draws, func(left, right compileDrawGroup) int { return int(left.draw.AlphaMode) - int(right.draw.AlphaMode) })
	for _, group := range draws {
		group.draw.FirstIndex = uint32(len(result.Indices) / IndexStride)
		group.draw.IndexCount = uint32(len(group.indices))
		for _, address := range group.indices {
			result.Indices = binary.LittleEndian.AppendUint32(result.Indices, address)
		}
		result.Draws = append(result.Draws, group.draw)
	}
	if len(result.Draws) == 0 {
		return GPUData{}, ErrChunk
	}
	ids := make([]uint32, len(materials.Materials))
	for index, material := range materials.Materials {
		ids[index] = material.MaterialID
	}
	solids := 0
	if document.StaticSolids != nil {
		solids = len(document.StaticSolids.Items)
	}
	bounds := result.Draws[0].Bounds
	for _, draw := range result.Draws[1:] {
		for axis := range 3 {
			bounds[axis] = min(bounds[axis], draw.Bounds[axis])
			bounds[axis+3] = max(bounds[axis+3], draw.Bounds[axis+3])
		}
	}
	if err := ValidatePayload(
		Payload{MaterialLayoutSHA256: [32]byte{1}, Bounds: bounds, RGBMRange: options.RGBMRange, GPUData: result},
		ids,
		len(document.Sectors),
		solids,
	); err != nil {
		return GPUData{}, err
	}
	return result, nil
}

type compileDrawGroup struct {
	draw    Draw
	indices []uint32
}

// A chunk shares one portal context. Opaque addresses can select independent
// owner/material/pose records; liquids retain owner boundaries for depth ordering.
func appendCompileDraw(groups *[]compileDrawGroup, owners map[[3]uint32]int, mesh compileMesh, record, addressBase, alpha uint32) error {
	pipeline := uint32(0)
	if alpha == 2 {
		pipeline = 2
	}
	key := [3]uint32{mesh.key.kind, mesh.key.owner, pipeline}
	if pipeline == 0 {
		key = [3]uint32{}
	}
	index, found := owners[key]
	if !found {
		if len(*groups) >= MaxDraws {
			return ErrChunk
		}
		index = len(*groups)
		owners[key] = index
		*groups = append(
			*groups,
			compileDrawGroup{draw: Draw{Record: record, AlphaMode: pipeline, Bounds: mesh.bounds, Sectors: slices.Clone(mesh.sectors)}},
		)
	}
	group := &(*groups)[index]
	for _, vertex := range mesh.indices {
		group.indices = append(group.indices, addressBase+vertex)
	}
	for axis := range 3 {
		group.draw.Bounds[axis] = min(group.draw.Bounds[axis], mesh.bounds[axis])
		group.draw.Bounds[axis+3] = max(group.draw.Bounds[axis+3], mesh.bounds[axis+3])
	}
	group.draw.Sectors = append(group.draw.Sectors, mesh.sectors...)
	slices.Sort(group.draw.Sectors)
	group.draw.Sectors = slices.Compact(group.draw.Sectors)
	return nil
}
