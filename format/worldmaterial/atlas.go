// Package worldmaterial defines the build-time world material atlas contract.
package worldmaterial

import (
	"errors"

	"github.com/karty-game/karty-sdk/codec/qoi"
	"github.com/karty-game/karty-sdk/format/asset"
	"github.com/karty-game/karty-sdk/internal/binwire"
)

const (
	Feature                 = "world/material-atlas@1"
	FeatureV2               = "world/material-atlas@2"
	FeatureReflectance      = "world/material-reflectance@1"
	ReflectanceEntry        = "@world/material-reflectance"
	ReflectanceMipTailEntry = "@world/material-reflectance-mips"
	SchemaV2                = "karty.world-material-atlas@2"
	CoverageOpaque          = "opaque"
	CoverageMasked          = "masked"
	Schema                  = "karty.world-material-atlas@1"
	MetadataKey             = "kartyWorldMaterialAtlas"
	LayoutEntry             = "@world/material-layout"
	AlbedoEntry             = "@world/material-albedo"
	DataEntry               = "@world/material-data"
	MipTailEntry            = "@world/material-mips"
	MipLevels               = 8
	TileSize                = 256
	Gutter                  = 16
	CellSize                = TileSize + 2*Gutter
	GridSize                = 14
	Inset                   = 32
	MaxMaterials            = GridSize * GridSize
	MaxLayoutBytes          = 32 * 1024
	BinaryMagic             = "KMA1"
)

var ErrAtlas = errors.New("world material atlas is invalid")

// Rect describes the inner tile, excluding its extruded gutter. MaterialID is
// the compiled world's level-local source texture ID, not a dense atlas slot
// or cartridge-wide asset ID. Zero denotes the opaque default material.
type Rect struct {
	MaterialID uint32
	X          int
	Y          int
	Width      int
	Height     int
	Gutter     int
	Strengths  *Strengths
	Coverage   string
}

// Layout maps level-local material IDs to a canonical row-major atlas. It has
// no paths: the paired payloads use the fixed logical level entry names above.
type Layout struct {
	Schema    string
	Width     int
	Height    int
	Materials []Rect
	Mips      []MipLevel
}

// NewLayout preserves the supplied unique IDs; ID zero is the opaque default
// material. Callers derive their first-use order from the compiled world:
// floor, ceiling, then walls per sector. This function has no world to check.
func NewLayout(ids []uint32) (Layout, error) {
	if len(ids) == 0 || len(ids) > MaxMaterials {
		return Layout{}, ErrAtlas
	}
	l := Layout{Schema: Schema, Width: min(len(ids), GridSize)*CellSize + 2*Inset,
		Height: ((len(ids)+GridSize-1)/GridSize)*CellSize + 2*Inset}
	seen := make(map[uint32]bool, len(ids))
	for i, id := range ids {
		if seen[id] {
			return Layout{}, ErrAtlas
		}
		seen[id] = true
		l.Materials = append(l.Materials, Rect{MaterialID: id, X: Inset + (i%GridSize)*CellSize + Gutter,
			Y: Inset + (i/GridSize)*CellSize + Gutter, Width: TileSize, Height: TileSize, Gutter: Gutter})
	}
	return l, nil
}

// NewLayoutV2 creates a coverage-aware atlas with opaque slots by default.
func NewLayoutV2(ids []uint32) (Layout, error) {
	l, err := NewLayout(ids)
	if err != nil {
		return Layout{}, err
	}
	l.Schema = SchemaV2
	for i := range l.Materials {
		l.Materials[i].Coverage = CoverageOpaque
	}
	return l, nil
}

func (l Layout) Validate() error {
	// Bound the list before allocating or doing coordinate arithmetic. Comparing
	// against the canonical grid rejects overlaps, gaps, out-of-bounds gutters,
	// negative/overflowing coordinates and alternate packing interpretations.
	count := len(l.Materials)
	if count == 0 || count > MaxMaterials || (l.Schema != Schema && l.Schema != SchemaV2) ||
		l.Width != min(count, GridSize)*CellSize+2*Inset ||
		l.Height != ((count+GridSize-1)/GridSize)*CellSize+2*Inset {
		return ErrAtlas
	}
	seen := make(map[uint32]bool, count)
	for i, r := range l.Materials {
		want := Rect{MaterialID: r.MaterialID, X: Inset + (i%GridSize)*CellSize + Gutter,
			Y: Inset + (i/GridSize)*CellSize + Gutter, Width: TileSize, Height: TileSize, Gutter: Gutter}
		geometry := r
		geometry.Strengths = nil
		geometry.Coverage = ""
		if l.Schema == Schema && r.Coverage != "" || l.Schema == SchemaV2 && r.Coverage != CoverageOpaque && r.Coverage != CoverageMasked {
			return ErrAtlas
		}
		if seen[r.MaterialID] || geometry != want || (r.Strengths != nil && r.Strengths.Validate() != nil) {
			return ErrAtlas
		}
		seen[r.MaterialID] = true
	}
	if l.validateMips() != nil {
		return ErrAtlas
	}
	if _, err := encodeLayoutBinary(l); err != nil {
		return err
	}
	return nil
}

// EncodeLayout emits canonical typed binary accepted by DecodeLayout.
func EncodeLayout(l Layout) ([]byte, error) {
	if err := l.Validate(); err != nil {
		return nil, err
	}
	return encodeLayoutBinary(l)
}

func encodeLayoutBinary(l Layout) ([]byte, error) {
	w := binwire.NewWriter(BinaryMagic, MaxLayoutBytes)
	writeWorldmaterialLayout(w, &l)
	encoded, err := w.Bytes()
	if err != nil {
		return nil, ErrAtlas
	}
	return encoded, nil
}

// DecodeLayout bounds the complete binary payload before typed allocation.
// JSON, noncanonical primitives, oversized counts and trailing bytes fail.
func DecodeLayout(encoded []byte) (Layout, error) {
	if len(encoded) == 0 || len(encoded) > MaxLayoutBytes {
		return Layout{}, ErrAtlas
	}
	scan := binwire.NewReader(encoded, BinaryMagic, MaxLayoutBytes)
	scanWorldmaterialLayout(scan)
	if scan.Done() != nil {
		return Layout{}, ErrAtlas
	}
	var l Layout
	reader := binwire.NewReader(encoded, BinaryMagic, MaxLayoutBytes)
	readWorldmaterialLayout(reader, &l)
	if reader.Done() != nil || l.Validate() != nil {
		return Layout{}, ErrAtlas
	}
	return l, nil
}

// Pair groups encoded QOI bytes with their shared layout; callers retain slice
// ownership. It is not a serialized level format: levels carry three entries and an optional
// fourth mip tail, plus optional reflectance companion entries. Tail albedo cells
// hold sRGB bytes; raw data cells hold the
// linear channels below. The combined tail has a linear raw-storage QOI tag.
// Albedo is opaque sRGB RGBA. Data uses straight linear RGBA channels:
// RG OpenGL tangent normal XY (128 is zero, signed decode clamps
// (byte-128)/127 to [-1,1]), B height, A ambient occlusion, NOT opacity.
type Pair struct {
	Layout             Layout
	Albedo             []byte
	Data               []byte
	MipTail            []byte
	Reflectance        []byte
	ReflectanceMipTail []byte
}

// Validate preflights every allocation and completely validates every QOI.
// Albedo and reflectance images require decoded pixels for channel checks.
func (p Pair) Validate() error {
	if p.Layout.Validate() != nil || (len(p.Layout.Mips) != 0) != (len(p.MipTail) != 0) ||
		(len(p.ReflectanceMipTail) != 0 && len(p.Reflectance) == 0) ||
		(len(p.Reflectance) != 0 && (len(p.Layout.Mips) != 0) != (len(p.ReflectanceMipTail) != 0)) {
		return ErrAtlas
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
			return ErrAtlas
		}
		total += m.DecodedBytes
	}
	for i, encoded := range images {
		if i >= 2 && len(encoded) == 0 {
			continue
		}
		if i == 1 {
			if _, err := qoi.Validate(encoded); err != nil {
				return ErrAtlas
			}
			continue
		}
		_, pixels, err := qoi.Decode(encoded)
		if err != nil {
			return ErrAtlas
		}
		if i == 0 {
			for o := 3; o < len(pixels.Pix); o += 4 {
				if pixels.Pix[o] != 255 && !p.Layout.maskedPixel((o/4)%p.Layout.Width, (o/4)/p.Layout.Width) {
					return ErrAtlas
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
								return ErrAtlas
							}
						}
					}
				}
			}
		} else if i >= 3 {
			if p.Layout.validateReflectancePixels(pixels, i == 4) != nil {
				return ErrAtlas
			}
		}
	}
	return nil
}

// maskedPixel recognizes only declared masked cells, leaving outer padding opaque.
func (l Layout) maskedPixel(x, y int) bool {
	if l.Schema != SchemaV2 || x < Inset || y < Inset {
		return false
	}
	col, row := (x-Inset)/CellSize, (y-Inset)/CellSize
	if col >= GridSize || col >= min(len(l.Materials), GridSize) {
		return false
	}
	slot := row*GridSize + col
	return slot >= 0 && slot < len(l.Materials) && l.Materials[slot].Coverage == CoverageMasked
}
