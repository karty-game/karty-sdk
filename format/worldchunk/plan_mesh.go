package worldchunk

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

// Mesh-only geometry is independent of receiver selection. Keep bounded
// fragments in global surface order rather than constructing a whole-world
// mesh, which may exceed the bounds of an individual child.
type meshOnlyPreparation struct {
	fragments      []Mesh
	owners         []int
	components     []int
	liquidCounts   []int
	nonLiquid      []int
	expandedCounts []int
	sectorCount    int
	geometryDigest string
	surfaceDigest  string
}

func prepareMeshOnly(document *world.Document, owners []int) (*meshOnlyPreparation, error) {
	geometryDigest, err := GeometryDigest(document)
	if err != nil {
		return nil, err
	}
	surfaceDigest, err := meshSurfaceDigest(document)
	if err != nil {
		return nil, err
	}
	surfaces, err := worldlightmap.Surfaces(document)
	if err != nil {
		return nil, err
	}
	count := len(document.Sectors) + len(owners)
	p := &meshOnlyPreparation{
		fragments: make([]Mesh, count), owners: owners, components: make([]int, count),
		nonLiquid: make([]int, count), expandedCounts: make([]int, count), sectorCount: len(document.Sectors),
		geometryDigest: geometryDigest, surfaceDigest: surfaceDigest,
	}
	for index := range p.components {
		p.components[index] = -1
		p.fragments[index].Bounds = emptyMeshBounds()
		if index < p.sectorCount {
			p.fragments[index].Sectors = []int{index}
		} else {
			p.fragments[index].Solids = []int{index - p.sectorCount}
		}
	}
	var junctions []pointIndex
	for seed := range document.Sectors {
		if p.components[seed] >= 0 {
			continue
		}
		component, solids, scopeErr := worldlightmap.ReceiverScope(document, &worldlightmap.ReceiverSelection{Sectors: []int{seed}})
		if scopeErr != nil {
			return nil, scopeErr
		}
		id := len(junctions)
		junctions = append(junctions, componentJunctions(document, surfaces, component, solids))
		for sector, included := range component {
			if included {
				p.components[sector] = id
			}
		}
		for solid, included := range solids {
			if included {
				p.components[p.sectorCount+solid] = id
			}
		}
	}
	for _, surface := range surfaces {
		index := surface.Binding.Index
		if !strings.HasPrefix(surface.Binding.Kind, "sector-") {
			index += p.sectorCount
		}
		component := p.components[index]
		if component < 0 {
			return nil, ErrChunk
		}
		if err := emitSurface(&p.fragments[index], document, worldlightmap.Layout{}, junctions[component], surface, -1); err != nil {
			return nil, p.sectorFailure(document, index, err)
		}
	}
	liquids := displacedLiquidMaterials(document)
	p.liquidCounts = make([]int, len(junctions))
	// Only liquid triangles join patches. Omitting unrelated opaque triangles
	// retains exactly the same factors, edge union order and patch budgets.
	for component := range junctions {
		if err := p.refineComponent(document, component, liquids); err != nil {
			return nil, err
		}
	}
	return p, nil
}

func (p *meshOnlyPreparation) refineComponent(document *world.Document, component int, liquids map[uint32]liquidMaterial) error {
	type reference struct{ fragment, triangle int }
	var refs []reference
	patch := Mesh{}
	factors := make([][]int, len(p.fragments))
	for index := range p.fragments {
		if p.components[index] != component {
			continue
		}
		fragment := &p.fragments[index]
		factors[index] = make([]int, len(fragment.Triangles))
		for triangle, item := range fragment.Triangles {
			factors[index][triangle] = 1
			if horizontalLiquid(unpacked(item.Normal), item.Material, liquids) {
				if len(refs) >= MaxTriangles {
					return p.sectorFailure(document, index, fmt.Errorf("offline liquid triangle budget: %w", ErrChunk))
				}
				refs = append(refs, reference{index, triangle})
				patch.Triangles = append(patch.Triangles, item)
				patch.Vertices = append(patch.Vertices, fragment.Vertices[triangle*3:triangle*3+3]...)
			} else {
				p.nonLiquid[index]++
			}
		}
	}
	p.liquidCounts[component] = len(refs)
	if len(refs) > 0 {
		for index, factor := range liquidFactors(&patch, liquids) {
			ref := refs[index]
			factors[ref.fragment][ref.triangle] = factor
		}
	}
	for index, fragment := range p.fragments {
		if p.components[index] != component {
			continue
		}
		for _, factor := range factors[index] {
			p.expandedCounts[index] += factor * factor
		}
		if len(liquids) > 0 {
			refined, err := tessellateLiquidFactors(fragment, factors[index])
			if err != nil {
				return p.sectorFailure(document, index, err)
			}
			p.fragments[index] = refined
		}
	}
	return nil
}

func (p *meshOnlyPreparation) sectorFailure(document *world.Document, fragment int, err error) error {
	sector := fragment
	if fragment >= p.sectorCount {
		sector = p.owners[fragment-p.sectorCount]
	}
	return fmt.Errorf("sector %q cannot fit one chunk: %w", document.Sectors[sector].ID, err)
}

func (p *meshOnlyPreparation) trial(sectors []int, options Options) (PlannedChunk, error) {
	solids := make([]int, 0)
	for solid, owner := range p.owners {
		if _, included := slices.BinarySearch(sectors, owner); included {
			solids = append(solids, solid)
		}
	}
	chunk := PlannedChunk{Sectors: slices.Clone(sectors), Solids: solids}
	raw := p.liquidCounts[p.components[sectors[0]]]
	expanded := 0
	for _, index := range sectors {
		chunk.MeshBytes += DecodedMeshBytes(p.fragments[index])
		raw += p.nonLiquid[index]
		expanded += p.expandedCounts[index]
	}
	for _, solid := range solids {
		index := p.sectorCount + solid
		chunk.MeshBytes += DecodedMeshBytes(p.fragments[index])
		raw += p.nonLiquid[index]
		expanded += p.expandedCounts[index]
	}
	if raw > MaxTriangles || expanded > MaxTriangles {
		return PlannedChunk{}, fmt.Errorf("offline triangle budget: %w", ErrChunk)
	}
	if chunk.MeshBytes > options.MaxMeshBytes {
		return PlannedChunk{}, fmt.Errorf("mesh needs %d decoded bytes (budget %d): %w", chunk.MeshBytes, options.MaxMeshBytes, ErrChunk)
	}
	encoded := uint64(meshHeaderBytes+4*(len(sectors)+len(solids))) + chunk.MeshBytes
	if encoded > MaxEncodedSize || encoded > options.MaxEncodedBytes {
		return PlannedChunk{}, fmt.Errorf("mesh needs %d encoded bytes (budget %d): %w", encoded, options.MaxEncodedBytes, ErrChunk)
	}
	chunk.Bounds = emptyMeshBounds()
	for _, sector := range sectors {
		mergeMeshBounds(&chunk.Bounds, p.fragments[sector].Bounds)
	}
	for _, solid := range solids {
		mergeMeshBounds(&chunk.Bounds, p.fragments[p.sectorCount+solid].Bounds)
	}
	return chunk, nil
}

func mergeMeshBounds(bounds *[6]float64, fragment [6]float64) {
	for axis := range 3 {
		bounds[axis] = min(bounds[axis], fragment[axis])
		bounds[axis+3] = max(bounds[axis+3], fragment[axis+3])
	}
}

func (p *meshOnlyPreparation) finish(chunk PlannedChunk, document *world.Document) (PlannedChunk, error) {
	count := int(chunk.MeshBytes / 484)
	mesh := Mesh{
		Schema: MeshSchema, GeometrySHA256: p.geometryDigest, SurfaceSHA256: p.surfaceDigest,
		Sectors: slices.Clone(chunk.Sectors), Solids: slices.Clone(chunk.Solids), Bounds: emptyMeshBounds(),
		Vertices: make([]Vertex, 0, count*3), Indices: make([]uint32, 0, count*3), Triangles: make([]Triangle, 0, count),
	}
	appendFragment := func(fragment Mesh) {
		for range fragment.Vertices {
			mesh.Indices = append(mesh.Indices, uint32(len(mesh.Indices)))
		}
		mesh.Vertices = append(mesh.Vertices, fragment.Vertices...)
		mesh.Triangles = append(mesh.Triangles, fragment.Triangles...)
		for axis := range 3 {
			mesh.Bounds[axis] = min(mesh.Bounds[axis], fragment.Bounds[axis])
			mesh.Bounds[axis+3] = max(mesh.Bounds[axis+3], fragment.Bounds[axis+3])
		}
	}
	for _, sector := range chunk.Sectors {
		appendFragment(p.fragments[sector])
	}
	for _, solid := range chunk.Solids {
		appendFragment(p.fragments[p.sectorCount+solid])
	}
	// Public input validation and complete semantic coverage remain mandatory;
	// growth trials skip only repeated work on immutable prepared geometry.
	if _, err := EncodeMesh(mesh, document); err != nil {
		return PlannedChunk{}, err
	}
	chunk.Mesh, chunk.Bounds = mesh, mesh.Bounds
	return chunk, nil
}

func emptyMeshBounds() [6]float64 {
	return [6]float64{math.Inf(1), math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1), math.Inf(-1)}
}
