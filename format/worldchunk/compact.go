package worldchunk

import (
	"slices"

	"github.com/karty-game/karty-sdk/format/world"
)

func compactLightmapChunks(document world.Document, options Options, chunks []PlannedChunk,
	trial func([]int) (PlannedChunk, error)) ([]PlannedChunk, error) {
	components := make([]int, len(document.Sectors))
	for i := range components {
		components[i] = -1
	}
	for seed := range document.Sectors {
		if components[seed] >= 0 {
			continue
		}
		queue := []int{seed}
		components[seed] = seed
		for next := 0; next < len(queue); next++ {
			for edge, wall := range document.Sectors[queue[next]].Walls {
				if wall.Portal >= 0 && components[wall.Portal] < 0 && ordinary(&document, queue[next], edge) {
					components[wall.Portal] = seed
					queue = append(queue, int(wall.Portal))
				}
			}
		}
	}
	ordered := slices.Clone(chunks)
	slices.SortStableFunc(ordered, func(a, b PlannedChunk) int { return occupiedCharts(b.Layout) - occupiedCharts(a.Layout) })
	var bins []PlannedChunk
	for _, chunk := range ordered {
		best := -1
		var merged PlannedChunk
		indices := make([]int, len(bins))
		for i := range indices {
			indices[i] = i
		}
		slices.SortStableFunc(indices, func(a, b int) int { return occupiedCharts(bins[b].Layout) - occupiedCharts(bins[a].Layout) })
		for _, index := range indices {
			bin := bins[index]
			if components[bin.Sectors[0]] != components[chunk.Sectors[0]] || len(bin.Sectors)+len(chunk.Sectors) > options.MaxSectors {
				continue
			}
			if options.SectorGroups != nil && options.SectorGroups[bin.Sectors[0]] != options.SectorGroups[chunk.Sectors[0]] {
				continue
			}
			sectors := append(slices.Clone(bin.Sectors), chunk.Sectors...)
			slices.Sort(sectors)
			candidate, err := trial(sectors)
			if err != nil || candidate.LightmapBytes > bin.LightmapBytes+chunk.LightmapBytes {
				continue
			}
			best, merged = index, candidate
			break
		}
		if best < 0 {
			bins = append(bins, chunk)
		} else {
			bins[best] = merged
		}
	}
	// Earlier best-fit choices can leave another exact pair merge available.
	for {
		changed := false
		for i := 0; i < len(bins) && !changed; i++ {
			for j := i + 1; j < len(bins); j++ {
				if components[bins[i].Sectors[0]] != components[bins[j].Sectors[0]] ||
					len(bins[i].Sectors)+len(bins[j].Sectors) > options.MaxSectors {
					continue
				}
				if options.SectorGroups != nil && options.SectorGroups[bins[i].Sectors[0]] != options.SectorGroups[bins[j].Sectors[0]] {
					continue
				}
				sectors := append(slices.Clone(bins[i].Sectors), bins[j].Sectors...)
				slices.Sort(sectors)
				candidate, err := trial(sectors)
				if err != nil || candidate.LightmapBytes > bins[i].LightmapBytes+bins[j].LightmapBytes {
					continue
				}
				bins[i] = candidate
				bins = slices.Delete(bins, j, j+1)
				changed = true
				break
			}
		}
		if !changed {
			break
		}
	}
	slices.SortFunc(bins, func(a, b PlannedChunk) int { return a.Sectors[0] - b.Sectors[0] })
	return bins, nil
}
