package worldmaterialssbump

import (
	"image"

	"github.com/karty-game/karty-sdk/codec/qoi"
	"github.com/karty-game/karty-sdk/format/asset"
	"github.com/karty-game/karty-sdk/format/worldmaterial"
)

// Pair retains caller ownership of encoded slices. Both images are linear RGBA
// QOI; RGB is authored RNM coefficient energy and alpha is reserved 255. Missing
// selected materials and all unused cells contain flat (85,85,85,255) pixels.
type Pair struct {
	Descriptor Descriptor
	Atlas      []byte
	MipTail    []byte
}

// DecodedPair owns its pixels; no partial result escapes on any error.
type DecodedPair struct{ Atlas, MipTail *image.NRGBA }

func (p Pair) Validate(layout worldmaterial.Layout) error {
	_, err := p.Decode(layout)
	return err
}

// ValidateTogether preflights the aggregate decoded texture budget before any
// pixel allocation, then validates every base and companion stream completely.
func (p Pair) ValidateTogether(base worldmaterial.Pair) error {
	images := [][]byte{base.Albedo, base.Data, base.MipTail, base.Reflectance, base.ReflectanceMipTail, p.Atlas, p.MipTail}
	if decodedBudget(images) != nil || base.Validate() != nil {
		return ErrCompanion
	}
	return p.Validate(base.Layout)
}

func decodedBudget(images [][]byte) error {
	var total uint64
	for _, encoded := range images {
		if len(encoded) == 0 {
			continue
		}
		m, err := qoi.Validate(encoded)
		if err != nil || m.DecodedBytes > asset.MaxDecodedTextures-total {
			return ErrCompanion
		}
		total += m.DecodedBytes
	}
	return nil
}

// Decode validates the layout binding, full streams and companion budget before
// allocating pixels. Whole-level consumers also call ValidateTogether.
func (p Pair) Decode(layout worldmaterial.Layout) (DecodedPair, error) {
	if p.Descriptor.Validate(layout) != nil || (len(layout.Mips) != 0) != (len(p.MipTail) != 0) {
		return DecodedPair{}, ErrCompanion
	}
	images := [2][]byte{p.Atlas, p.MipTail}
	if decodedBudget(images[:]) != nil {
		return DecodedPair{}, ErrCompanion
	}
	width, height := layout.MipDimensions()
	for i, encoded := range images {
		if i == 1 && len(encoded) == 0 {
			continue
		}
		w, h := layout.Width, layout.Height
		if i == 1 {
			w, h = width, height
		}
		m, err := qoi.Inspect(encoded)
		if err != nil || int(m.Width) != w || int(m.Height) != h || m.Channels != qoi.ChannelsRGBA || m.Colorspace != qoi.ColorspaceLinear {
			return DecodedPair{}, ErrCompanion
		}
	}
	selected := make([]bool, len(layout.Materials))
	next := 0
	for i, r := range layout.Materials {
		if next < len(p.Descriptor.MaterialIDs) && r.MaterialID == p.Descriptor.MaterialIDs[next] {
			selected[i] = true
			next++
		}
	}
	var result DecodedPair
	for i, encoded := range images {
		if i == 1 && len(encoded) == 0 {
			continue
		}
		_, pixels, err := qoi.Decode(encoded)
		if err != nil || validatePixels(layout, selected, pixels, i == 1) != nil {
			return DecodedPair{}, ErrCompanion
		}
		if i == 0 {
			result.Atlas = pixels
		} else {
			result.MipTail = pixels
		}
	}
	return result, nil
}

func validatePixels(layout worldmaterial.Layout, selected []bool, pixels *image.NRGBA, tail bool) error {
	band := 0
	for y := range pixels.Bounds().Dy() {
		if tail {
			for band+1 < len(layout.Mips) && y >= layout.Mips[band+1].Y {
				band++
			}
		}
		for x := range pixels.Bounds().Dx() {
			active := false
			if tail {
				m := layout.Mips[band]
				cell := m.Size + 2*m.Gutter
				index := ((y-m.Y)/cell)*m.Columns + x/cell
				active = x < m.Columns*cell && index < 2*len(selected) && index%2 == 1 && selected[index/2]
			} else if x >= worldmaterial.Inset && y >= worldmaterial.Inset {
				col, row := (x-worldmaterial.Inset)/worldmaterial.CellSize, (y-worldmaterial.Inset)/worldmaterial.CellSize
				slot := row*worldmaterial.GridSize + col
				active = col < min(len(selected), worldmaterial.GridSize) && slot < len(selected) && selected[slot]
			}
			o := pixels.PixOffset(x, y)
			if pixels.Pix[o+3] != 255 || !active && (pixels.Pix[o] != 85 || pixels.Pix[o+1] != 85 || pixels.Pix[o+2] != 85) {
				return ErrCompanion
			}
		}
	}
	return nil
}
