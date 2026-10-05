package worldmaterial

import "math"

// Strengths scales material effects independently. An omitted Strengths pointer
// means one for every channel; explicit zero disables that effect.
type Strengths struct {
	Normal float64 `json:"normal"`
	Height float64 `json:"height"`
	AO     float64 `json:"ao"`
	Rim    float64 `json:"rim"`
}

func (s Strengths) Validate() error {
	for _, value := range [4]float64{s.Normal, s.Height, s.AO, s.Rim} {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 4 {
			return ErrAtlas
		}
	}
	return nil
}

// MipLevel records one canonical shelf band. Cells contain an albedo tile then
// a raw data tile for each material in the L0 first-use order. Coordinates and
// dimensions are derived rather than repeating hundreds of per-material rects.
type MipLevel struct {
	Level   int `json:"level"`
	Size    int `json:"size"`
	Gutter  int `json:"gutter"`
	Y       int `json:"y"`
	Columns int `json:"columns"`
}

func canonicalMips(count int) (records [MipLevels]MipLevel, width, height int) {
	for index := range MipLevels {
		level := index + 1
		size, gutter := TileSize>>level, max(1, Gutter>>level)
		cell := size + 2*gutter
		columns := min(2*count, 4096/cell)
		records[index] = MipLevel{level, size, gutter, height, columns}
		width = max(width, columns*cell)
		height += ((2*count + columns - 1) / columns) * cell
	}
	return records, width, height
}

func (l Layout) validateMips() error {
	if len(l.Mips) == 0 {
		return nil
	}
	if len(l.Mips) != MipLevels {
		return ErrAtlas
	}
	want, width, height := canonicalMips(len(l.Materials))
	if width > 4096 || height > 4096 {
		return ErrAtlas
	}
	for index, record := range l.Mips {
		if record != want[index] {
			return ErrAtlas
		}
	}
	return nil
}

// NewMipLayout adds the complete L1-L8 chain, preserving the exact L0 geometry.
// It clones placements and strength values, retaining caller slice ownership.
func NewMipLayout(l Layout) (Layout, error) {
	if l.Validate() != nil {
		return Layout{}, ErrAtlas
	}
	l.Materials = append([]Rect(nil), l.Materials...)
	for index := range l.Materials {
		if s := l.Materials[index].Strengths; s != nil {
			copy := *s
			l.Materials[index].Strengths = &copy
		}
	}
	records, _, _ := canonicalMips(len(l.Materials))
	l.Mips = append([]MipLevel(nil), records[:]...)
	return l, nil
}

// MipDimensions returns the padded allocation dimensions, or zero when absent
// or malformed. It does not use untrusted record arithmetic.
func (l Layout) MipDimensions() (width, height int) {
	if len(l.Materials) < 1 || len(l.Materials) > MaxMaterials || len(l.Mips) != MipLevels || l.validateMips() != nil {
		return 0, 0
	}
	_, width, height = canonicalMips(len(l.Materials))
	return width, height
}

// MipRect returns an interior region in the combined tail. Level is 1-8 and
// slot is the zero-based index in Materials; data selects the raw RGBA region.
func (l Layout) MipRect(level, slot int, data bool) (Rect, error) {
	if level < 1 || level > MipLevels || slot < 0 || slot >= len(l.Materials) ||
		len(l.Materials) > MaxMaterials || len(l.Mips) != MipLevels || l.validateMips() != nil {
		return Rect{}, ErrAtlas
	}
	m := l.Mips[level-1]
	cell, index := m.Size+2*m.Gutter, 2*slot
	if data {
		index++
	}
	return Rect{MaterialID: l.Materials[slot].MaterialID, X: (index%m.Columns)*cell + m.Gutter,
		Y: m.Y + (index/m.Columns)*cell + m.Gutter, Width: m.Size, Height: m.Size, Gutter: m.Gutter}, nil
}
