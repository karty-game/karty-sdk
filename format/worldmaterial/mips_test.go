package worldmaterial_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image/color"
	"math"
	"reflect"
	"strconv"
	"testing"

	"github.com/karty-game/karty-sdk/codec/qoi"
	"github.com/karty-game/karty-sdk/format/asset"
	"github.com/karty-game/karty-sdk/format/worldmaterial"
)

func TestMipLayoutCanonicalPackingAllCounts(t *testing.T) {
	for count := 1; count <= worldmaterial.MaxMaterials; count++ {
		ids := make([]uint32, count)
		for slot := range ids {
			ids[slot] = uint32(slot)
		}
		base, _ := worldmaterial.NewLayout(ids)
		layout, err := worldmaterial.NewMipLayout(base)
		if err != nil || !reflect.DeepEqual(base.Materials, layout.Materials) {
			t.Fatalf("count %d: L0 changed: %v", count, err)
		}
		width, height := layout.MipDimensions()
		if width < 1 || height < 1 || width > 4096 || height > 4096 {
			t.Fatalf("unbounded tail %dx%d", width, height)
		}
		cost := uint64(base.Width)*uint64(base.Height)*8 + uint64(width)*uint64(height)*4
		if cost > asset.MaxDecodedTextures {
			t.Fatalf("count %d allocation %d", count, cost)
		}
		encoded, err := worldmaterial.EncodeLayout(layout)
		if err != nil || len(encoded) > worldmaterial.MaxLayoutBytes {
			t.Fatalf("count %d encoding %d: %v", count, len(encoded), err)
		}
		decoded, err := worldmaterial.DecodeLayout(encoded)
		if err != nil || !reflect.DeepEqual(decoded, layout) {
			t.Fatalf("count %d canonical roundtrip: %v", count, err)
		}
		lastY := 0
		for level := 1; level <= worldmaterial.MipLevels; level++ {
			record := layout.Mips[level-1]
			if record.Y != lastY || record.Size != 256>>level || record.Gutter != max(1, 16>>level) {
				t.Fatal("noncanonical shelf band")
			}
			cell := record.Size + 2*record.Gutter
			lastY += ((2*count + record.Columns - 1) / record.Columns) * cell
			for slot := range ids {
				for _, data := range []bool{false, true} {
					r, err := layout.MipRect(level, slot, data)
					index := slot * 2
					if data {
						index++
					}
					if err != nil || r.MaterialID != ids[slot] || r.X != (index%record.Columns)*cell+record.Gutter || r.Y != record.Y+(index/record.Columns)*cell+record.Gutter || r.X-r.Gutter < 0 || r.Y-r.Gutter < 0 || r.X+r.Width+r.Gutter > width || r.Y+r.Height+r.Gutter > height {
						t.Fatalf("escaped/overlapping cell: %+v", r)
					}
				}
			}
		}
		if lastY != height {
			t.Fatal("tail allocation omits padding")
		}
	}
}

func TestMipLayoutRejectsMalformedRecordsAndOwnsCopies(t *testing.T) {
	base, _ := worldmaterial.NewLayout([]uint32{99, 0})
	base.Materials[0].Strengths = &worldmaterial.Strengths{Normal: 1, Height: 1, AO: 1, Rim: 1}
	l, _ := worldmaterial.NewMipLayout(base)
	l.Materials[0].Strengths.Normal = 0
	l.Materials[1].MaterialID = 7
	if base.Materials[0].Strengths.Normal != 1 || base.Materials[1].MaterialID != 0 {
		t.Fatal("caller-owned placements or strengths changed")
	}
	for name, mutate := range map[string]func(*worldmaterial.Layout){
		"missing final": func(l *worldmaterial.Layout) { l.Mips = l.Mips[:7] },
		"extra":         func(l *worldmaterial.Layout) { l.Mips = append(l.Mips, l.Mips[7]) },
		"wrong size":    func(l *worldmaterial.Layout) { l.Mips[7].Size = 2 },
		"wrong gutter":  func(l *worldmaterial.Layout) { l.Mips[7].Gutter = 0 },
		"overlap":       func(l *worldmaterial.Layout) { l.Mips[7].Y-- },
		"columns":       func(l *worldmaterial.Layout) { l.Mips[0].Columns++ },
		"level":         func(l *worldmaterial.Layout) { l.Mips[7].Level = 9 },
		"overflow":      func(l *worldmaterial.Layout) { l.Mips[7].Y = int(^uint(0) >> 1) },
	} {
		t.Run(name, func(t *testing.T) {
			layout, _ := worldmaterial.NewMipLayout(base)
			mutate(&layout)
			if !errors.Is(layout.Validate(), worldmaterial.ErrAtlas) {
				t.Fatal("malformed mip records accepted")
			}
			if width, height := layout.MipDimensions(); width != 0 || height != 0 {
				t.Fatal("malformed dimensions used untrusted arithmetic")
			}
			if _, err := layout.MipRect(1, 0, false); !errors.Is(err, worldmaterial.ErrAtlas) {
				t.Fatal("malformed mip rectangle accepted")
			}
		})
	}
	for _, index := range [][2]int{{0, 0}, {9, 0}, {1, -1}, {1, 2}} {
		if _, err := l.MipRect(index[0], index[1], false); !errors.Is(err, worldmaterial.ErrAtlas) {
			t.Fatal("invalid mip address accepted")
		}
	}
}

func testMipPair(t *testing.T) worldmaterial.Pair {
	t.Helper()
	p := testPair(t)
	l, err := worldmaterial.NewMipLayout(p.Layout)
	if err != nil {
		t.Fatal(err)
	}
	p.Layout = l
	width, height := l.MipDimensions()
	p.MipTail = solidQOI(width, height, [4]byte{128, 128, 0, 255}, qoi.ColorspaceLinear)
	return p
}

func TestMipPairPresenceFullStreamAndAlbedoOpacity(t *testing.T) {
	pair := testMipPair(t)
	if err := pair.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*worldmaterial.Pair){
		"missing tail":    func(p *worldmaterial.Pair) { p.MipTail = nil },
		"undeclared tail": func(p *worldmaterial.Pair) { p.Layout.Mips = nil },
		"wrong width":     func(p *worldmaterial.Pair) { binary.BigEndian.PutUint32(p.MipTail[4:8], 1) },
		"sRGB header":     func(p *worldmaterial.Pair) { p.MipTail[13] = qoi.ColorspaceSRGB },
		"RGB header":      func(p *worldmaterial.Pair) { p.MipTail[12] = qoi.ChannelsRGB },
		"truncated":       func(p *worldmaterial.Pair) { p.MipTail = p.MipTail[:len(p.MipTail)-1] },
		"bad marker":      func(p *worldmaterial.Pair) { p.MipTail[len(p.MipTail)-1] = 0 },
		"trailing operation": func(p *worldmaterial.Pair) {
			p.MipTail = append(append(bytes.Clone(p.MipTail[:len(p.MipTail)-8]), 0xc0), p.MipTail[len(p.MipTail)-8:]...)
		},
	} {
		t.Run(name, func(t *testing.T) {
			p := pair
			p.MipTail = bytes.Clone(p.MipTail)
			mutate(&p)
			if !errors.Is(p.Validate(), worldmaterial.ErrAtlas) {
				t.Fatal("malformed mip pair accepted")
			}
		})
	}
	_, pixels, err := qoi.Decode(pair.MipTail)
	if err != nil {
		t.Fatal(err)
	}
	for level := 1; level <= worldmaterial.MipLevels; level++ {
		r, _ := pair.Layout.MipRect(level, 0, true)
		for y := r.Y - r.Gutter; y < r.Y+r.Height+r.Gutter; y++ {
			for x := r.X - r.Gutter; x < r.X+r.Width+r.Gutter; x++ {
				pixels.SetNRGBA(x, y, color.NRGBA{R: 255, G: 128, B: 73, A: 0})
			}
		}
	}
	pair.MipTail, _, err = qoi.Encode(pixels, qoi.Options{Channels: qoi.ChannelsRGBA, Colorspace: qoi.ColorspaceLinear})
	if err != nil || pair.Validate() != nil {
		t.Fatal("raw data RGB with AO0 rejected")
	}
	for _, gutter := range []bool{false, true} {
		r, _ := pair.Layout.MipRect(8, 0, false)
		x, y := r.X, r.Y
		if gutter {
			x--
			y--
		}
		pixels.Pix[pixels.PixOffset(x, y)+3] = 0
		pair.MipTail, _, err = qoi.Encode(pixels, qoi.Options{Channels: qoi.ChannelsRGBA, Colorspace: qoi.ColorspaceLinear})
		if err != nil || !errors.Is(pair.Validate(), worldmaterial.ErrAtlas) {
			t.Fatal("transparent tail albedo accepted")
		}
		pixels.Pix[pixels.PixOffset(x, y)+3] = 255
	}
}

func TestStrengthsBoundsCanonicalCompletenessAndLayoutSize(t *testing.T) {
	l, _ := worldmaterial.NewLayout([]uint32{0})
	for _, value := range []float64{0, 1, 4} {
		l.Materials[0].Strengths = &worldmaterial.Strengths{Normal: value, Height: value, AO: value, Rim: value}
		encoded, err := worldmaterial.EncodeLayout(l)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := worldmaterial.DecodeLayout(encoded); err != nil {
			t.Fatal(err)
		}
		missing := bytes.Replace(encoded, []byte(`"normal":`+strconv.FormatFloat(value, 'f', -1, 64)+`,`), nil, 1)
		if _, err := worldmaterial.DecodeLayout(missing); !errors.Is(err, worldmaterial.ErrAtlas) {
			t.Fatal("missing explicit strength channel accepted")
		}
	}
	for _, value := range []float64{-0.001, 4.001, math.NaN(), math.Inf(1), math.Inf(-1)} {
		for channel := range 4 {
			v := worldmaterial.Strengths{Normal: 1, Height: 1, AO: 1, Rim: 1}
			targets := [4]*float64{&v.Normal, &v.Height, &v.AO, &v.Rim}
			*targets[channel] = value
			l.Materials[0].Strengths = &v
			if !errors.Is(l.Validate(), worldmaterial.ErrAtlas) {
				t.Fatal("invalid strength accepted")
			}
		}
	}
	ids := make([]uint32, worldmaterial.MaxMaterials)
	for i := range ids {
		ids[i] = uint32(i)
	}
	l, _ = worldmaterial.NewLayout(ids)
	l, _ = worldmaterial.NewMipLayout(l)
	for i := range l.Materials {
		l.Materials[i].Strengths = &worldmaterial.Strengths{Normal: 1, Height: 1, AO: 1, Rim: 1}
	}
	if encoded, err := worldmaterial.EncodeLayout(l); err != nil || len(encoded) > worldmaterial.MaxLayoutBytes {
		t.Fatal("maximum default strength/mip metadata exceeds bound")
	}
	for i := range l.Materials {
		l.Materials[i].Strengths = &worldmaterial.Strengths{Normal: 1.123456789012345, Height: 1.123456789012345, AO: 1.123456789012345, Rim: 1.123456789012345}
	}
	if !errors.Is(l.Validate(), worldmaterial.ErrAtlas) {
		t.Fatal("oversize precise strength metadata passed allocation preflight")
	}
	if _, err := worldmaterial.EncodeLayout(l); !errors.Is(err, worldmaterial.ErrAtlas) {
		t.Fatal("oversize precise strength metadata encoded")
	}
}

func TestMipTailHeaderPreflightBeforeAnyPixelAllocation(t *testing.T) {
	ids := make([]uint32, worldmaterial.MaxMaterials)
	for index := range ids {
		ids[index] = uint32(index)
	}
	layout, _ := worldmaterial.NewLayout(ids)
	layout, _ = worldmaterial.NewMipLayout(layout)
	width, height := layout.MipDimensions()
	pair := worldmaterial.Pair{Layout: layout,
		Albedo:  solidQOI(layout.Width, layout.Height, [4]byte{192, 192, 192, 255}, qoi.ColorspaceSRGB),
		Data:    solidQOI(layout.Width, layout.Height, [4]byte{128, 128, 73, 0}, qoi.ColorspaceLinear),
		MipTail: solidQOI(width, height, [4]byte{128, 128, 0, 255}, qoi.ColorspaceLinear)}
	binary.BigEndian.PutUint32(pair.MipTail[4:8], uint32(width-1))
	if !errors.Is(pair.Validate(), worldmaterial.ErrAtlas) {
		t.Fatal("invalid third header accepted")
	}
	baseline := testing.AllocsPerRun(3, func() { _ = layout.Validate() })
	if got := testing.AllocsPerRun(3, func() { _ = pair.Validate() }); got > baseline {
		t.Fatalf("third-header rejection allocated earlier textures: %g allocations, layout baseline %g", got, baseline)
	}
}
