package worldmaterial_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"testing"

	"github.com/karty-game/karty-sdk/codec/qoi"
	"github.com/karty-game/karty-sdk/format/asset"
	"github.com/karty-game/karty-sdk/format/worldmaterial"
)

func reflectancePair(t *testing.T, mips bool) worldmaterial.Pair {
	t.Helper()
	pair := testPair(t)
	if mips {
		pair = testMipPair(t)
	}
	pair.Reflectance = solidQOI(pair.Layout.Width, pair.Layout.Height, [4]byte{200, 140, 0, 255}, qoi.ColorspaceLinear)
	if mips {
		width, height := pair.Layout.MipDimensions()
		pixels := image.NewNRGBA(image.Rect(0, 0, width, height))
		for offset := 3; offset < len(pixels.Pix); offset += 4 {
			pixels.Pix[offset] = 255
		}
		for level := 1; level <= worldmaterial.MipLevels; level++ {
			rect, _ := pair.Layout.MipRect(level, 0, true)
			for y := rect.Y - rect.Gutter; y < rect.Y+rect.Height+rect.Gutter; y++ {
				for x := rect.X - rect.Gutter; x < rect.X+rect.Width+rect.Gutter; x++ {
					offset := pixels.PixOffset(x, y)
					pixels.Pix[offset], pixels.Pix[offset+1] = 200, 140
				}
			}
		}
		encoded, _, err := qoi.Encode(pixels, qoi.Options{Channels: qoi.ChannelsRGBA, Colorspace: qoi.ColorspaceLinear})
		if err != nil {
			t.Fatal(err)
		}
		pair.ReflectanceMipTail = encoded
	}
	return pair
}

func TestReflectanceCompanionValidation(t *testing.T) {
	for _, mips := range []bool{false, true} {
		pair := reflectancePair(t, mips)
		if err := pair.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	pair := reflectancePair(t, true)
	for name, mutate := range map[string]func(*worldmaterial.Pair){
		"missing L0":   func(p *worldmaterial.Pair) { p.Reflectance = nil },
		"missing tail": func(p *worldmaterial.Pair) { p.ReflectanceMipTail = nil },
		"wrong dimensions": func(p *worldmaterial.Pair) {
			p.Reflectance = bytes.Clone(p.Reflectance)
			binary.BigEndian.PutUint32(p.Reflectance[4:8], 1)
		},
		"sRGB": func(p *worldmaterial.Pair) {
			p.Reflectance = bytes.Clone(p.Reflectance)
			p.Reflectance[13] = qoi.ColorspaceSRGB
		},
		"RGB": func(p *worldmaterial.Pair) {
			p.Reflectance = bytes.Clone(p.Reflectance)
			p.Reflectance[12] = qoi.ChannelsRGB
		},
		"broken stream": func(p *worldmaterial.Pair) { p.Reflectance = p.Reflectance[:len(p.Reflectance)-1] },
		"reserved B": func(p *worldmaterial.Pair) {
			p.Reflectance = solidQOI(p.Layout.Width, p.Layout.Height, [4]byte{1, 2, 3, 255}, qoi.ColorspaceLinear)
		},
		"reserved alpha": func(p *worldmaterial.Pair) {
			p.Reflectance = solidQOI(p.Layout.Width, p.Layout.Height, [4]byte{1, 2, 0, 1}, qoi.ColorspaceLinear)
		},
		"unused tail slots": func(p *worldmaterial.Pair) {
			w, h := p.Layout.MipDimensions()
			p.ReflectanceMipTail = solidQOI(w, h, [4]byte{1, 2, 0, 255}, qoi.ColorspaceLinear)
		},
	} {
		t.Run(name, func(t *testing.T) {
			p := pair
			mutate(&p)
			if !errors.Is(p.Validate(), worldmaterial.ErrAtlas) {
				t.Fatal("invalid companion accepted")
			}
		})
	}
	if err := testPair(t).Validate(); err != nil {
		t.Fatal("legacy pair changed", err)
	}
	if err := (asset.Capabilities{Runtime: []asset.Capability{asset.CapabilityWorldMaterialReflectanceV1}}).Validate(); err != nil {
		t.Fatal(err)
	}
	if string(asset.CapabilityWorldMaterialReflectanceV1) != worldmaterial.FeatureReflectance {
		t.Fatal("feature identity mismatch")
	}
}

func TestReflectanceAggregateAllocationPreflight(t *testing.T) {
	ids := make([]uint32, worldmaterial.MaxMaterials)
	for i := range ids {
		ids[i] = uint32(i)
	}
	layout, _ := worldmaterial.NewLayout(ids)
	layout, _ = worldmaterial.NewMipLayout(layout)
	header := func(w, h int, space uint8) []byte {
		encoded := solidQOI(1, 1, [4]byte{0, 0, 0, 255}, space)
		binary.BigEndian.PutUint32(encoded[4:8], uint32(w))
		binary.BigEndian.PutUint32(encoded[8:12], uint32(h))
		return encoded
	}
	width, height := layout.MipDimensions()
	pair := worldmaterial.Pair{Layout: layout,
		Albedo:             header(layout.Width, layout.Height, qoi.ColorspaceSRGB),
		Data:               header(layout.Width, layout.Height, qoi.ColorspaceLinear),
		MipTail:            header(width, height, qoi.ColorspaceLinear),
		Reflectance:        header(layout.Width, layout.Height, qoi.ColorspaceLinear),
		ReflectanceMipTail: header(width, height, qoi.ColorspaceLinear)}
	baseline := testing.AllocsPerRun(3, func() { _ = layout.Validate() })
	if !errors.Is(pair.Validate(), worldmaterial.ErrAtlas) {
		t.Fatal("aggregate budget exceeded")
	}
	if got := testing.AllocsPerRun(3, func() { _ = pair.Validate() }); got > baseline {
		t.Fatalf("budget rejection decoded pixels: allocations %g baseline %g", got, baseline)
	}
}
