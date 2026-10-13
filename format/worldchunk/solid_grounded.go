package worldchunk

import (
	"errors"
	"math"
	"slices"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

// This build-only pass retains every other balanced owner. Exact page trials
// and the caller's final complete owner validation preserve all hard bounds.
// If concentrating the selected solids cannot fit, retain the original plan.
func groundedSolidOwners(document world.Document, options worldlightmap.Options, owners []int) ([]int, error) {
	selected := make([]bool, len(owners))
	assigned := make([][]int, len(document.Sectors))
	found := false
	for solid, owner := range owners {
		selected[solid] = narrowGroundedSolid(document.StaticSolids.Items[solid], document.Sectors[owner])
		found = found || selected[solid]
		if !selected[solid] {
			assigned[owner] = append(assigned[owner], solid)
		}
	}
	if !found {
		return owners, nil
	}
	proposal := slices.Clone(owners)
	for solid, included := range selected {
		if !included {
			continue
		}
		placed := false
		for sector, room := range document.Sectors {
			if !overlaps(document.StaticSolids.Items[solid], room) {
				continue
			}
			candidate := append(slices.Clone(assigned[sector]), solid)
			slices.Sort(candidate)
			_, err := worldlightmap.CompileReceivers(document, options, []int{sector}, candidate)
			if errors.Is(err, worldlightmap.ErrPageCapacity) {
				continue
			}
			if err != nil {
				return nil, err
			}
			assigned[sector], proposal[solid], placed = candidate, sector, true
			break
		}
		if !placed {
			return owners, nil
		}
	}
	return proposal, nil
}

func narrowGroundedSolid(solid world.Solid, sector world.Sector) bool {
	const (
		minimumLength      = 2.0
		minimumAspect      = 4.0
		maximumWidth       = 0.6
		maximumHeight      = 1.25
		maximumFloorHeight = 1.0
	)
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	topRelative, bottomRelative, height := math.Inf(-1), math.Inf(1), 0.0
	for _, point := range solid.Footprint {
		minX, minY = min(minX, point.X), min(minY, point.Y)
		maxX, maxY = max(maxX, point.X), max(maxY, point.Y)
		floor := at(point, sector.Floor).Z
		top, bottom := at(point, solid.Top).Z, at(point, solid.Bottom).Z
		topRelative, bottomRelative = max(topRelative, top-floor), min(bottomRelative, bottom-floor)
		height = max(height, top-bottom)
	}
	long, short := max(maxX-minX, maxY-minY), min(maxX-minX, maxY-minY)
	return long >= minimumLength && long >= minimumAspect*short && short <= maximumWidth &&
		height <= maximumHeight && topRelative <= maximumFloorHeight && bottomRelative <= 0
}
