package worldchunk

import (
	"fmt"
	"math"
	"slices"

	"github.com/karty-game/karty-sdk/format/world"
)

func growPlannedChunk(document *world.Document, options Options, seed int, assigned, enclosures []int,
	trial func([]int) (PlannedChunk, error)) (PlannedChunk, error) {
	sectors := []int{seed}
	var chunk PlannedChunk
	var err error
	wholeFits := false
	if enclosures != nil {
		whole := enclosureMembers(document, seed, enclosures, assigned)
		if len(whole) <= options.MaxSectors {
			if chunk, err = trial(whole); err == nil {
				sectors = whole
				wholeFits = true
			}
		}
	}
	if !wholeFits {
		chunk, err = trial(sectors)
	}
	if err != nil {
		return PlannedChunk{}, fmt.Errorf("sector %q cannot fit one chunk: %w", document.Sectors[seed].ID, err)
	}
	for len(sectors) < options.MaxSectors {
		added := false
		for _, candidate := range chunkGrowthCandidates(document, options, sectors, assigned, enclosures) {
			proposal := []int{candidate}
			if enclosures != nil && !hasEnclosure(sectors, enclosures[candidate], enclosures) {
				// Do not fragment another enclosure merely to fill the current bin.
				proposal = enclosureMembers(document, candidate, enclosures, assigned)
			}
			if len(sectors)+len(proposal) > options.MaxSectors {
				continue
			}
			proposal = append(slices.Clone(sectors), proposal...)
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
	return chunk, nil
}

func enclosureMembers(document *world.Document, seed int, enclosures, assigned []int) []int {
	result := []int{seed}
	var seen [world.MaxSectors]bool
	seen[seed] = true
	for next := 0; next < len(result); next++ {
		for edge, wall := range document.Sectors[result[next]].Walls {
			sector := int(wall.Portal)
			if sector >= 0 && !seen[sector] && assigned[sector] < 0 && enclosures[sector] == enclosures[seed] &&
				ordinary(document, result[next], edge) {
				seen[sector] = true
				result = append(result, sector)
			}
		}
	}
	slices.Sort(result)
	return result
}

func hasEnclosure(sectors []int, id int, enclosures []int) bool {
	for _, sector := range sectors {
		if enclosures[sector] == id {
			return true
		}
	}
	return false
}

func chunkGrowthCandidates(document *world.Document, options Options, sectors, assigned, enclosures []int) []int {
	var result []int
	var widths [world.MaxSectors]float64
	for candidate := range document.Sectors {
		if assigned[candidate] >= 0 || slices.Contains(sectors, candidate) ||
			options.SectorGroups != nil && options.SectorGroups[candidate] != options.SectorGroups[sectors[0]] {
			continue
		}
		adjacent := false
		for _, sector := range sectors {
			for edge, wall := range document.Sectors[sector].Walls {
				if int(wall.Portal) == candidate && ordinary(document, sector, edge) {
					adjacent = true
					widths[candidate] += math.Hypot(wall.End.X-wall.Start.X, wall.End.Y-wall.Start.Y)
				}
			}
		}
		if adjacent {
			result = append(result, candidate)
		}
	}
	if enclosures != nil {
		slices.SortStableFunc(result, func(left, right int) int {
			leftInside := hasEnclosure(sectors, enclosures[left], enclosures)
			rightInside := hasEnclosure(sectors, enclosures[right], enclosures)
			if leftInside != rightInside {
				if leftInside {
					return -1
				}
				return 1
			}
			if widths[left] != widths[right] {
				if widths[left] > widths[right] {
					return -1
				}
				return 1
			}
			return left - right
		})
	}
	return result
}
