package worldchunk

import (
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

// Plan grows connected chunks deterministically in authored sector order. A
// budget failure never lowers density. An oversized single sector is an error.
// MaxEncodedBytes limits the mesh entry here; builders must also check the final
// child module including compressed lightmaps against that exact byte budget.
// Lightmap.PageSize is a maximum here: a 1024 maximum first tries a 512 page
// at identical density and padding, then retries 1024 only on page capacity.
func Plan(document world.Document, options Options) (PlanResult, error) {
	if err := world.Validate(&document); err != nil {
		return PlanResult{}, err
	}
	if options.SectorGroups != nil {
		if len(options.SectorGroups) != len(document.Sectors) {
			return PlanResult{}, fmt.Errorf("sector groups must name every global sector: %w", ErrChunk)
		}
		for index, group := range options.SectorGroups {
			if group < 0 || group >= world.MaxSectors {
				return PlanResult{}, fmt.Errorf("sector %d group is outside 0..%d: %w", index, world.MaxSectors-1, ErrChunk)
			}
		}
	}
	if options.MaxMeshBytes == 0 {
		options.MaxMeshBytes = 4 * 1024 * 1024
	}
	if options.MaxEncodedBytes == 0 {
		options.MaxEncodedBytes = 8 * 1024 * 1024
	}
	if options.MaxSectors == 0 {
		options.MaxSectors = world.MaxSectors
	}
	if options.MaxMeshBytes < 1 || options.MaxMeshBytes > 64*1024*1024 || options.MaxEncodedBytes < 1 ||
		options.MaxEncodedBytes > MaxEncodedBytes ||
		options.MaxSectors < 1 ||
		options.MaxSectors > world.MaxSectors {
		return PlanResult{}, ErrChunk
	}
	owners, err := solidOwners(&document)
	if err != nil {
		return PlanResult{}, err
	}
	var prepared *meshOnlyPreparation
	if options.NoLightmap {
		prepared, err = prepareMeshOnly(&document, owners)
		if err != nil {
			return PlanResult{}, err
		}
	}
	result := PlanResult{SectorChunks: make([]int, len(document.Sectors))}
	for i := range result.SectorChunks {
		result.SectorChunks[i] = -1
	}
	trial := func(sectors []int) (PlannedChunk, error) {
		if prepared != nil {
			return prepared.trial(sectors, options)
		}
		solids := make([]int, 0)
		for solid, owner := range owners {
			if _, included := slices.BinarySearch(sectors, owner); included {
				solids = append(solids, solid)
			}
		}
		lightmapOptions := options.Lightmap
		adaptive := lightmapOptions.PageSize == 0 || lightmapOptions.PageSize == 1024
		if adaptive {
			lightmapOptions.PageSize = 512
		}
		layout, compileErr := worldlightmap.CompileReceivers(document, lightmapOptions, sectors, solids)
		if adaptive && errors.Is(compileErr, worldlightmap.ErrPageCapacity) {
			lightmapOptions.PageSize = 1024
			layout, compileErr = worldlightmap.CompileReceivers(document, lightmapOptions, sectors, solids)
		}
		if compileErr != nil {
			return PlannedChunk{}, compileErr
		}
		mesh, compileErr := CompileMesh(document, sectors, solids, layout)
		if compileErr != nil {
			return PlannedChunk{}, compileErr
		}
		decoded := DecodedMeshBytes(mesh)
		if decoded > options.MaxMeshBytes {
			return PlannedChunk{}, fmt.Errorf("mesh needs %d decoded bytes (budget %d): %w", decoded, options.MaxMeshBytes, ErrChunk)
		}
		encoded, compileErr := EncodeMesh(mesh, &document)
		if compileErr != nil {
			return PlannedChunk{}, compileErr
		}
		if uint64(len(encoded)) > options.MaxEncodedBytes {
			return PlannedChunk{}, fmt.Errorf(
				"mesh needs %d encoded bytes (budget %d): %w",
				len(encoded),
				options.MaxEncodedBytes,
				ErrChunk,
			)
		}
		lightmapBytes := uint64(0)
		if len(layout.Pages) > 0 {
			lightmapBytes = uint64(layout.Pages[0].Width) * uint64(layout.Pages[0].Height) * 12
		}
		return PlannedChunk{
			Sectors:       slices.Clone(sectors),
			Solids:        solids,
			Mesh:          mesh,
			Layout:        layout,
			Bounds:        mesh.Bounds,
			MeshBytes:     decoded,
			LightmapBytes: lightmapBytes,
		}, nil
	}
	for seed := range document.Sectors {
		if result.SectorChunks[seed] >= 0 {
			continue
		}
		sectors := []int{seed}
		chunk, trialErr := trial(sectors)
		if trialErr != nil {
			return PlanResult{}, fmt.Errorf("sector %q cannot fit one chunk: %w", document.Sectors[seed].ID, trialErr)
		}
		for len(sectors) < options.MaxSectors {
			added := false
			for candidate := range document.Sectors {
				if options.SectorGroups != nil && options.SectorGroups[candidate] != options.SectorGroups[seed] {
					continue
				}
				if result.SectorChunks[candidate] >= 0 {
					continue
				}
				if _, included := slices.BinarySearch(sectors, candidate); included {
					continue
				}
				adjacent := false
				for _, sector := range sectors {
					for edge, wall := range document.Sectors[sector].Walls {
						adjacent = adjacent || int(wall.Portal) == candidate && ordinary(&document, sector, edge)
					}
				}
				if !adjacent {
					continue
				}
				proposal := slices.Clone(sectors)
				proposal = append(proposal, candidate)
				slices.Sort(proposal)
				grown, growErr := trial(proposal)
				if growErr != nil {
					continue
				}
				sectors, chunk, added = proposal, grown, true
				break
			}
			if !added {
				break
			}
		}
		if prepared != nil {
			chunk, trialErr = prepared.finish(chunk, &document)
			if trialErr != nil {
				return PlanResult{}, trialErr
			}
		}
		chunk.ID = len(result.Chunks)
		for _, sector := range sectors {
			result.SectorChunks[sector] = chunk.ID
		}
		result.Chunks = append(result.Chunks, chunk)
	}
	result.Candidates = reachableCandidates(&document, result.SectorChunks, len(result.Chunks))
	return result, nil
}

func ordinary(document *world.Document, sector, edge int) bool {
	wall := document.Sectors[sector].Walls[edge]
	if wall.Portal < 0 {
		return false
	}
	target := world.Wall{}
	found := false
	if wall.PortalWall > 0 {
		target = document.Sectors[wall.Portal].Walls[wall.PortalWall-1]
		found = true
	} else {
		for _, candidate := range document.Sectors[wall.Portal].Walls {
			if candidate.Portal == int32(sector) && coincident(wall, candidate) {
				target = candidate
				found = true
				break
			}
		}
	}
	return found && target.Portal == int32(sector) && coincident(wall, target) &&
		(document.Version < world.Version || int(target.PortalWall) == edge+1)
}
func coincident(a, b world.Wall) bool {
	return math.Hypot(a.Start.X-b.End.X, a.Start.Y-b.End.Y) <= 1e-9 && math.Hypot(a.End.X-b.Start.X, a.End.Y-b.Start.Y) <= 1e-9
}

func reachableCandidates(document *world.Document, sectorChunks []int, count int) [][]int {
	result := make([][]int, len(document.Sectors))
	for start := range document.Sectors {
		visited := make([]bool, len(document.Sectors))
		visited[start] = true
		chunks := make([]bool, count)
		queue := []int{start}
		for next := 0; next < len(queue); next++ {
			sector := queue[next]
			chunks[sectorChunks[sector]] = true
			for _, wall := range document.Sectors[sector].Walls {
				if wall.Portal >= 0 && !visited[wall.Portal] {
					visited[wall.Portal] = true
					queue = append(queue, int(wall.Portal))
				}
			}
		}
		for chunk, included := range chunks {
			if included {
				result[start] = append(result[start], chunk)
			}
		}
	}
	return result
}

func solidOwners(document *world.Document) ([]int, error) {
	if document.StaticSolids == nil {
		return nil, nil
	}
	owners := make([]int, len(document.StaticSolids.Items))
	for i := range owners {
		owners[i] = -1
	}
	scoped := make([]bool, len(document.Sectors))
	for solid, item := range document.StaticSolids.Items {
		for sector, room := range document.Sectors {
			if overlaps(item, room) {
				owners[solid] = sector
				break
			}
		}
		if owners[solid] < 0 {
			return nil, fmt.Errorf("solid %q has no receiver space: %w", item.ID, ErrChunk)
		}
		if scoped[owners[solid]] {
			continue
		}
		component, _, err := worldlightmap.ReceiverScope(
			document,
			&worldlightmap.ReceiverSelection{Sectors: []int{owners[solid]}, Solids: []int{solid}},
		)
		if err != nil {
			return nil, err
		}
		// ReceiverScope checks every solid against the complete component. All
		// later owners in this immutable component share that exact result.
		for sector, included := range component {
			scoped[sector] = scoped[sector] || included
		}
	}
	return owners, nil
}
func overlaps(solid world.Solid, sector world.Sector) bool {
	a, b := [4]float64{
		math.Inf(1),
		math.Inf(1),
		math.Inf(-1),
		math.Inf(-1),
	}, [4]float64{
		math.Inf(1),
		math.Inf(1),
		math.Inf(-1),
		math.Inf(-1),
	}
	for _, p := range solid.Footprint {
		a[0] = math.Min(a[0], p.X)
		a[1] = math.Min(a[1], p.Y)
		a[2] = math.Max(a[2], p.X)
		a[3] = math.Max(a[3], p.Y)
	}
	for _, wall := range sector.Walls {
		p := wall.Start
		b[0] = math.Min(b[0], p.X)
		b[1] = math.Min(b[1], p.Y)
		b[2] = math.Max(b[2], p.X)
		b[3] = math.Max(b[3], p.Y)
	}
	return a[0] <= b[2]+1e-9 && a[2] >= b[0]-1e-9 && a[1] <= b[3]+1e-9 && a[3] >= b[1]-1e-9
}
