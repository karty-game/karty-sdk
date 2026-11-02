package worldgpuchunk

import "math"

// Validate checks canonical identities, spatial ownership and allocation bounds.
func (catalog Catalog) Validate() error {
	if catalog.GeometrySHA256 == [32]byte{} || catalog.MaterialLayoutSHA256 == [32]byte{} ||
		!canonicalIDs(
			catalog.BakedLights,
			MaxCells,
		) || len(catalog.Cells) == 0 || len(catalog.Cells) > MaxCells || !finite(catalog.CellSize) || catalog.CellSize <= 0 ||
		!finite(catalog.Origin[0]) || !finite(catalog.Origin[1]) {
		return ErrChunk
	}
	spaces := make(map[uint32]uint32)
	for index, cell := range catalog.Cells {
		if cell.ID != uint32(index) || cell.SpaceID >= MaxCells || cell.Group > 1 ||
			cell.GridSpan == 0 || cell.GridSpan > MaxCells || cell.GridSpan&(cell.GridSpan-1) != 0 ||
			cell.ContentSHA256 == [32]byte{} || cell.Size == 0 || cell.Size > 32<<20 ||
			cell.GPUBytes == 0 || cell.GPUBytes > MaxEncodedSize ||
			cell.RNMBytes < 12 || cell.RNMBytes%12 != 0 || cell.RNMBytes > MaxDimension*MaxDimension*12 ||
			!validBounds(cell.Bounds) || !canonicalIDs(cell.Sectors, MaxCells) ||
			!canonicalIDs(cell.PhysicalIDs, MaxCells) || len(cell.Sectors) == 0 {
			return ErrChunk
		}
		if index > 0 && !cellBefore(catalog.Cells[index-1], cell) {
			return ErrChunk
		}
		for axis := range 2 {
			minimum := catalog.Origin[axis] + float64(cell.Grid[axis])*catalog.CellSize
			maximum := minimum + catalog.CellSize*float64(cell.GridSpan)
			tolerance := math.Max(1, math.Max(math.Abs(minimum), math.Abs(maximum))) * 1e-6
			if !finite(minimum) || !finite(maximum) || cell.Bounds[axis] < minimum-tolerance || cell.Bounds[axis+3] > maximum+tolerance {
				return ErrChunk
			}
		}
		for _, sector := range cell.Sectors {
			if previous, ok := spaces[sector]; ok && previous != cell.SpaceID {
				return ErrChunk
			}
			spaces[sector] = cell.SpaceID
		}
	}
	return nil
}

// ValidateReferences checks catalog ownership against admitted root metadata.
// sectorSpaces must come from the root's coincident identity-portal components.
func (catalog Catalog) ValidateReferences(sectorSpaces []uint32, physicalCount int) error {
	if catalog.Validate() != nil || physicalCount < 0 || physicalCount > MaxCells {
		return ErrChunk
	}
	for _, cell := range catalog.Cells {
		for _, sector := range cell.Sectors {
			if uint64(sector) >= uint64(len(sectorSpaces)) || sectorSpaces[sector] != cell.SpaceID {
				return ErrChunk
			}
		}
		for _, id := range cell.PhysicalIDs {
			if uint64(id) >= uint64(physicalCount) {
				return ErrChunk
			}
		}
	}
	return nil
}

func cellBefore(a, b Cell) bool {
	if a.SpaceID != b.SpaceID {
		return a.SpaceID < b.SpaceID
	}
	if a.Group != b.Group {
		return a.Group < b.Group
	}
	if a.Grid[1] != b.Grid[1] {
		return a.Grid[1] < b.Grid[1]
	}
	if a.Grid[0] != b.Grid[0] {
		return a.Grid[0] < b.Grid[0]
	}
	if a.GridSpan != b.GridSpan {
		return a.GridSpan < b.GridSpan
	}
	if len(a.Sectors) == 0 || len(b.Sectors) == 0 {
		return false
	}
	return a.Sectors[0] < b.Sectors[0]
}
func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }
func validBounds(bounds [6]float64) bool {
	for axis := range 3 {
		if !finite(bounds[axis]) || !finite(bounds[axis+3]) || bounds[axis] > bounds[axis+3] {
			return false
		}
	}
	return true
}
func containsBounds(outer, inner [6]float64) bool {
	for axis := range 3 {
		if inner[axis] < outer[axis] || inner[axis+3] > outer[axis+3] {
			return false
		}
	}
	return true
}
func canonicalIDs(ids []uint32, maximum int) bool {
	if len(ids) > maximum {
		return false
	}
	for i, id := range ids {
		if uint64(id) >= uint64(maximum) {
			return false
		}
		if i > 0 && id <= ids[i-1] {
			return false
		}
	}
	return true
}

// ValidateLights binds the sorted baked-source union to the root authored light
// table. It preserves selective prebaking without runtime double contribution.
func (catalog Catalog) ValidateLights(lightCount int) error {
	if lightCount < 0 || lightCount > MaxCells || !canonicalIDs(catalog.BakedLights, MaxCells) {
		return ErrChunk
	}
	for _, index := range catalog.BakedLights {
		if uint64(index) >= uint64(lightCount) {
			return ErrChunk
		}
	}
	return nil
}
