package worldmaterial

import (
	"image"

	"github.com/karty-game/karty-sdk/codec/qoi"
	"github.com/karty-game/karty-sdk/format/asset"
)

// DecodedPair owns validated straight-alpha pixels. Missing optional images are
// nil. Pixels do not alias the encoded Pair, and each Decode call owns its output.
type DecodedPair struct{ images [5]*image.NRGBA }

func (p DecodedPair) Albedo() *image.NRGBA             { return p.images[0] }
func (p DecodedPair) Data() *image.NRGBA               { return p.images[1] }
func (p DecodedPair) MipTail() *image.NRGBA            { return p.images[2] }
func (p DecodedPair) Reflectance() *image.NRGBA        { return p.images[3] }
func (p DecodedPair) ReflectanceMipTail() *image.NRGBA { return p.images[4] }

// Validate preflights every allocation and completely validates every QOI.
// It discards pixels needed for channel validation; use Decode to retain them.
func (p Pair) Validate() error { _, err := p.decode(false); return err }

// Decode performs the same complete validation as Validate and retains every
// image for a consumer that needs pixels. No partial result escapes on failure.
func (p Pair) Decode() (DecodedPair, error) { return p.decode(true) }

func (p Pair) decode(retain bool) (DecodedPair, error) {
	var result DecodedPair
	if p.Layout.Validate() != nil || (len(p.Layout.Mips) != 0) != (len(p.MipTail) != 0) ||
		(len(p.ReflectanceMipTail) != 0 && len(p.Reflectance) == 0) ||
		(len(p.Reflectance) != 0 && (len(p.Layout.Mips) != 0) != (len(p.ReflectanceMipTail) != 0)) {
		return DecodedPair{}, ErrAtlas
	}
	var total uint64
	width, height := p.Layout.MipDimensions()
	images := [5][]byte{p.Albedo, p.Data, p.MipTail, p.Reflectance, p.ReflectanceMipTail}
	for i, encoded := range images {
		if i >= 2 && len(encoded) == 0 {
			continue
		}
		m, err := qoi.Inspect(encoded)
		space := qoi.ColorspaceLinear
		w, h := p.Layout.Width, p.Layout.Height
		if i == 0 {
			space = qoi.ColorspaceSRGB
		}
		if i == 2 || i == 4 {
			w, h = width, height
		}
		if err != nil || int(m.Width) != w || int(m.Height) != h ||
			m.Channels != qoi.ChannelsRGBA || m.Colorspace != space ||
			m.DecodedBytes > asset.MaxDecodedTextures-total {
			return DecodedPair{}, ErrAtlas
		}
		total += m.DecodedBytes
	}
	for i, encoded := range images {
		if i >= 2 && len(encoded) == 0 {
			continue
		}
		if i == 1 && !retain {
			if _, err := qoi.Validate(encoded); err != nil {
				return DecodedPair{}, ErrAtlas
			}
			continue
		}
		_, pixels, err := qoi.Decode(encoded)
		if err != nil {
			return DecodedPair{}, ErrAtlas
		}
		if retain {
			result.images[i] = pixels
		}
		if i == 0 {
			for o := 3; o < len(pixels.Pix); o += 4 {
				if pixels.Pix[o] != 255 && !p.Layout.maskedPixel((o/4)%p.Layout.Width, (o/4)/p.Layout.Width) {
					return DecodedPair{}, ErrAtlas
				}
			}
		} else if i == 2 {
			for level := 1; level <= MipLevels; level++ {
				for slot := range p.Layout.Materials {
					if p.Layout.Materials[slot].Coverage == CoverageMasked {
						continue
					}
					r, _ := p.Layout.MipRect(level, slot, false)
					for y := r.Y - r.Gutter; y < r.Y+r.Height+r.Gutter; y++ {
						for x := r.X - r.Gutter; x < r.X+r.Width+r.Gutter; x++ {
							if pixels.Pix[pixels.PixOffset(x, y)+3] != 255 {
								return DecodedPair{}, ErrAtlas
							}
						}
					}
				}
			}
		} else if i >= 3 {
			if p.Layout.validateReflectancePixels(pixels, i == 4) != nil {
				return DecodedPair{}, ErrAtlas
			}
		}
	}
	return result, nil
}
