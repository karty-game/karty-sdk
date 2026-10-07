package worldmaterial_test

import (
	"bytes"
	"errors"
	"image"
	"reflect"
	"testing"

	"github.com/karty-game/karty-sdk/codec/qoi"
	"github.com/karty-game/karty-sdk/format/worldmaterial"
)

func TestCoverageLayoutV2CanonicalAndLimits(t *testing.T) {
	t.Parallel()
	ids := make([]uint32, worldmaterial.MaxMaterials)
	for i := range ids {
		ids[i] = uint32(i)
	}
	l, err := worldmaterial.NewLayoutV2(ids)
	if err != nil {
		t.Fatal(err)
	}
	for i := range l.Materials {
		l.Materials[i].Coverage = worldmaterial.CoverageMasked
	}
	l, err = worldmaterial.NewMipLayout(l)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := worldmaterial.EncodeLayout(l)
	if err != nil || len(encoded) > worldmaterial.MaxLayoutBytes {
		t.Fatalf("bounded coverage layout: %v", err)
	}
	decoded, err := worldmaterial.DecodeLayout(encoded)
	if err != nil || !reflect.DeepEqual(l, decoded) {
		t.Fatalf("roundtrip: %v", err)
	}
	for _, mutate := range []func(*worldmaterial.Layout){
		func(l *worldmaterial.Layout) { l.Schema = worldmaterial.Schema },
		func(l *worldmaterial.Layout) { l.Materials[len(l.Materials)-1].Coverage = "" },
		func(l *worldmaterial.Layout) { l.Materials[len(l.Materials)-1].Coverage = "blend" },
	} {
		bad := l
		bad.Materials = append([]worldmaterial.Rect(nil), l.Materials...)
		mutate(&bad)
		if bad.Validate() == nil {
			t.Fatal("accepted malformed coverage declaration")
		}
	}
	for _, bad := range [][]byte{
		bytes.Replace(encoded, []byte(`"coverage":"masked"`), []byte(`"coverage":null`), 1),
		bytes.Replace(encoded, []byte(`"coverage":"masked"`), []byte(`"coverage":"masked","coverage":"masked"`), 1),
	} {
		if _, err := worldmaterial.DecodeLayout(bad); err == nil {
			t.Fatal("accepted noncanonical coverage field")
		}
	}
}
func TestCoveragePairV2OpacityAndStraightBytes(t *testing.T) {
	t.Parallel()
	pair := testPair(t, 3, 11)
	layout, err := worldmaterial.NewLayoutV2([]uint32{3, 11})
	if err != nil {
		t.Fatal(err)
	}
	layout.Materials[1].Coverage = worldmaterial.CoverageMasked
	pair.Layout = layout
	_, pixels, err := qoi.Decode(pair.Albedo)
	if err != nil {
		t.Fatal(err)
	}
	mask := layout.Materials[1]
	points := [][2]int{
		{mask.X, mask.Y},
		{mask.X - mask.Gutter, mask.Y - mask.Gutter},
		{mask.X + mask.Width + mask.Gutter - 1, mask.Y + mask.Height + mask.Gutter - 1},
	}
	for _, point := range points {
		o := pixels.PixOffset(point[0], point[1])
		pixels.Pix[o], pixels.Pix[o+1], pixels.Pix[o+2], pixels.Pix[o+3] = 220, 91, 37, 64
	}
	pair.Albedo, _, err = qoi.Encode(pixels, qoi.Options{Channels: qoi.ChannelsRGBA, Colorspace: qoi.ColorspaceSRGB})
	if err != nil {
		t.Fatal(err)
	}
	if err := pair.Validate(); err != nil {
		t.Fatal(err)
	}
	_, decoded, err := qoi.Decode(pair.Albedo)
	if err != nil {
		t.Fatal(err)
	}
	o := decoded.PixOffset(mask.X, mask.Y)
	if !bytes.Equal(decoded.Pix[o:o+4], []byte{220, 91, 37, 64}) {
		t.Fatal("straight RGB coverage changed")
	}
	for _, point := range [][2]int{{0, 0}, {layout.Materials[0].X, layout.Materials[0].Y}, {layout.Materials[0].X - layout.Materials[0].Gutter, layout.Materials[0].Y}} {
		bad := pair
		changed := &image.NRGBA{Pix: bytes.Clone(pixels.Pix), Stride: pixels.Stride, Rect: pixels.Rect}
		changed.Pix[changed.PixOffset(point[0], point[1])+3] = 64
		bad.Albedo, _, err = qoi.Encode(changed, qoi.Options{Channels: qoi.ChannelsRGBA, Colorspace: qoi.ColorspaceSRGB})
		if err != nil {
			t.Fatal(err)
		}
		if !errors.Is(bad.Validate(), worldmaterial.ErrAtlas) {
			t.Fatal("accepted opaque slot/padding coverage")
		}
	}
	legacy := pair
	legacy.Layout.Schema = worldmaterial.Schema
	for i := range legacy.Layout.Materials {
		legacy.Layout.Materials[i].Coverage = ""
	}
	if !errors.Is(legacy.Validate(), worldmaterial.ErrAtlas) {
		t.Fatal("v1 accepted coverage pixels")
	}
}
func TestCoverageMipPairV2ChecksLastOpaqueTile(t *testing.T) {
	t.Parallel()
	pair := testPair(t, 3, 11)
	var mipErr error
	pair.Layout, mipErr = worldmaterial.NewMipLayout(pair.Layout)
	if mipErr != nil {
		t.Fatal(mipErr)
	}
	width, height := pair.Layout.MipDimensions()
	pair.MipTail = solidQOI(width, height, [4]byte{128, 128, 0, 255}, qoi.ColorspaceLinear)
	pair.Layout.Schema = worldmaterial.SchemaV2
	for i := range pair.Layout.Materials {
		pair.Layout.Materials[i].Coverage = worldmaterial.CoverageOpaque
	}
	pair.Layout.Materials[0].Coverage = worldmaterial.CoverageMasked
	_, pixels, err := qoi.Decode(pair.MipTail)
	if err != nil {
		t.Fatal(err)
	}
	for level := 1; level <= worldmaterial.MipLevels; level++ {
		r, _ := pair.Layout.MipRect(level, 0, false)
		pixels.Pix[pixels.PixOffset(r.X-r.Gutter, r.Y-r.Gutter)+3] = 0
	}
	pair.MipTail, _, err = qoi.Encode(pixels, qoi.Options{Channels: qoi.ChannelsRGBA, Colorspace: qoi.ColorspaceLinear})
	if err != nil {
		t.Fatal(err)
	}
	if err := pair.Validate(); err != nil {
		t.Fatal(err)
	}
	last, _ := pair.Layout.MipRect(worldmaterial.MipLevels, len(pair.Layout.Materials)-1, false)
	pixels.Pix[pixels.PixOffset(last.X, last.Y)+3] = 254
	pair.MipTail, _, err = qoi.Encode(pixels, qoi.Options{Channels: qoi.ChannelsRGBA, Colorspace: qoi.ColorspaceLinear})
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(pair.Validate(), worldmaterial.ErrAtlas) {
		t.Fatal("accepted malformed final opaque mip tile")
	}
}
