package worldchunk

import (
	"errors"
	"fmt"
	"slices"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

// BalancedSolidOwners is an opt-in build policy for decorative structures that
// overlap several receiver sectors. Largest chart sets are placed first, then
// assigned to the least occupied eligible sector whose exact receiver layout
// fits. It never changes density, page limits, geometry or physical components.
// The returned complete list can be supplied as Options.SolidOwners. The
// default Plan ownership remains unchanged.
func BalancedSolidOwners(document world.Document, options worldlightmap.Options) ([]int, error) {
	if err := world.Validate(&document); err != nil {
		return nil, err
	}
	initial, err := solidOwners(&document)
	if err != nil {
		return nil, err
	}
	if len(initial) == 0 {
		return initial, nil
	}
	if options.PageSize == 0 {
		options.PageSize = 1024
	}
	loads := make([]int, len(document.Sectors))
	assigned := make([][]int, len(document.Sectors))
	for i := range document.Sectors {
		layout, err := worldlightmap.CompileReceivers(document, options, []int{i}, nil)
		if err != nil {
			return nil, err
		}
		loads[i] = occupiedCharts(layout)
	}
	type placement struct{ solid, cost int }
	placements := make([]placement, len(initial))
	for solid := range initial {
		found := false
		for owner, room := range document.Sectors {
			if !overlaps(document.StaticSolids.Items[solid], room) {
				continue
			}
			layout, err := worldlightmap.CompileReceivers(document, options, []int{owner}, []int{solid})
			if errors.Is(err, worldlightmap.ErrPageCapacity) {
				continue
			}
			if err != nil {
				return nil, err
			}
			placements[solid] = placement{solid, occupiedCharts(layout) - loads[owner]}
			found = true
			break
		}
		if !found {
			return nil, fmt.Errorf(
				"solid %q cannot fit any receiver page: %w",
				document.StaticSolids.Items[solid].ID,
				worldlightmap.ErrPageCapacity,
			)
		}
	}
	slices.SortStableFunc(placements, func(a, b placement) int { return b.cost - a.cost })
	owners := make([]int, len(initial))
	for _, placement := range placements {
		var eligible []int
		for sector, room := range document.Sectors {
			if overlaps(document.StaticSolids.Items[placement.solid], room) {
				eligible = append(eligible, sector)
			}
		}
		slices.SortStableFunc(eligible, func(a, b int) int { return loads[a] - loads[b] })
		placed := false
		for _, sector := range eligible {
			proposal := slices.Clone(assigned[sector])
			proposal = append(proposal, placement.solid)
			slices.Sort(proposal)
			layout, err := worldlightmap.CompileReceivers(document, options, []int{sector}, proposal)
			if errors.Is(err, worldlightmap.ErrPageCapacity) {
				continue
			}
			if err != nil {
				return nil, err
			}
			assigned[sector], owners[placement.solid], loads[sector] = proposal, sector, occupiedCharts(layout)
			placed = true
			break
		}
		if !placed {
			return nil, fmt.Errorf(
				"solid %q has no receiver page at density %g: %w",
				document.StaticSolids.Items[placement.solid].ID,
				options.TexelsPerUnit,
				worldlightmap.ErrPageCapacity,
			)
		}
	}
	if _, err := solidOwners(&document, owners); err != nil {
		return nil, err
	}
	return owners, nil
}

func occupiedCharts(layout worldlightmap.Layout) int {
	total := 0
	for _, chart := range layout.Charts {
		total += (chart.Rect[2] - chart.Rect[0]) * (chart.Rect[3] - chart.Rect[1])
	}
	return total
}
