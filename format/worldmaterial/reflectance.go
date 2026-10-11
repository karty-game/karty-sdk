package worldmaterial

import "image"

// Reflectance companions preserve atlas@1/@2 layout geometry. Their straight
// linear QOI channels are R metallic, G smoothness, B zero, A 255. Tail data
// occupies the ordinary material-data mip cells; every other cell is neutral.
func (l Layout) validateReflectancePixels(pixels *image.NRGBA, tail bool) error {
	band := 0
	for y := 0; y < pixels.Bounds().Dy(); y++ {
		if tail {
			for band+1 < len(l.Mips) && y >= l.Mips[band+1].Y {
				band++
			}
		}
		for x := 0; x < pixels.Bounds().Dx(); x++ {
			offset := pixels.PixOffset(x, y)
			if pixels.Pix[offset+2] != 0 || pixels.Pix[offset+3] != 255 {
				return ErrAtlas
			}
			if tail {
				record := l.Mips[band]
				cell := record.Size + 2*record.Gutter
				index := ((y-record.Y)/cell)*record.Columns + x/cell
				used := x < record.Columns*cell && index < 2*len(l.Materials) && index%2 == 1
				if !used && (pixels.Pix[offset] != 0 || pixels.Pix[offset+1] != 0) {
					return ErrAtlas
				}
			}
		}
	}
	return nil
}
