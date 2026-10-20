package textureatlas_test

import (
	"bytes"
	"image"
	"testing"

	"github.com/karty-game/karty-sdk/codec/qoi"
	"github.com/karty-game/karty-sdk/format/textureatlas"
)

func TestAtlasRoundTripAliasesAndCompleteEntries(t *testing.T) {
	t.Parallel()
	layout := textureatlas.Layout{Pages: []textureatlas.Page{{Width: 8, Height: 8}}, Regions: []textureatlas.Region{
		{Name: "a", Page: 0, X: 2, Y: 2, Width: 4, Height: 4, Gutter: 2},
		{Name: "b", Page: 0, X: 2, Y: 2, Width: 4, Height: 4, Gutter: 2},
	}}
	encoded, err := textureatlas.EncodeLayout(layout)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := textureatlas.DecodeLayout(encoded)
	if err != nil || decoded.Regions[1].Name != "b" {
		t.Fatalf("round trip: %+v %v", decoded, err)
	}
	page, _, err := qoi.Encode(
		image.NewNRGBA(image.Rect(0, 0, 8, 8)),
		qoi.Options{Channels: qoi.ChannelsRGBA, Colorspace: qoi.ColorspaceSRGB},
	)
	if err != nil {
		t.Fatal(err)
	}
	bundle := textureatlas.Bundle{Layout: layout, Pages: [][]byte{page}}
	entries, err := bundle.Entries()
	if err != nil {
		t.Fatal(err)
	}
	got, present, err := textureatlas.ReadEntries(entries)
	if err != nil || !present || !bytes.Equal(got.Pages[0], page) {
		t.Fatalf("entries: %v %v", present, err)
	}
	for _, broken := range [][]textureatlas.Entry{entries[:1], entries[1:], append(append([]textureatlas.Entry(nil), entries...), entries[1])} {
		if _, _, err := textureatlas.ReadEntries(broken); err == nil {
			t.Fatal("partial/duplicate atlas published")
		}
	}
	for end := range len(encoded) {
		if _, err := textureatlas.DecodeLayout(encoded[:end]); err == nil {
			t.Fatalf("accepted truncation %d", end)
		}
	}
	if _, err := textureatlas.DecodeLayout(append(encoded, 0)); err == nil {
		t.Fatal("trailing bytes accepted")
	}
}

func TestAtlasRejectsInvalidGeometry(t *testing.T) {
	t.Parallel()
	for _, region := range []textureatlas.Region{
		{Name: "a", Page: -1, Width: 1, Height: 1},
		{Name: "a", Page: 0, X: 7, Y: 2, Width: 4, Height: 4, Gutter: 2},
		{Name: "a", Page: 0, X: 2, Y: 2, Width: 4, Height: 4, Gutter: 0},
		{Name: "a", Page: 0, X: 2, Y: 2, Width: 4, Height: 4, Gutter: 3},
	} {
		if err := (textureatlas.Layout{Pages: []textureatlas.Page{{Width: 8, Height: 8}}, Regions: []textureatlas.Region{region}}).Validate(); err == nil {
			t.Fatalf("accepted %+v", region)
		}
	}
	layout := textureatlas.Layout{Pages: []textureatlas.Page{{Width: 16, Height: 8}}, Regions: []textureatlas.Region{
		{Name: "a", X: 2, Y: 2, Width: 4, Height: 4, Gutter: 2},
		{Name: "b", X: 5, Y: 2, Width: 4, Height: 4, Gutter: 2},
	}}
	if err := layout.Validate(); err == nil {
		t.Fatal("overlapping non-alias rectangles accepted")
	}
}

func TestAtlasRejectsMissingOverlappingAndOutOfBoundsMips(t *testing.T) {
	t.Parallel()
	base := textureatlas.Region{
		Name:     "leaf",
		Page:     0,
		X:        2,
		Y:        2,
		Width:    16,
		Height:   16,
		Gutter:   2,
		Filter:   textureatlas.FilterColor,
		MipCount: 1,
	}
	base.Mips[0] = textureatlas.MipRect{X: 22, Y: 2, Width: 8, Height: 8}
	layout := textureatlas.Layout{Pages: []textureatlas.Page{{Width: 32, Height: 20}}, Regions: []textureatlas.Region{base}}
	if err := layout.Validate(); err != nil {
		t.Fatal(err)
	}
	encoded, err := textureatlas.EncodeLayout(layout)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := textureatlas.DecodeLayout(encoded)
	if err != nil || decoded.Regions[0] != base {
		t.Fatal("mip chain round trip", err)
	}
	for _, mutate := range []func(*textureatlas.Region){
		func(region *textureatlas.Region) { region.MipCount = 0 },
		func(region *textureatlas.Region) { region.Mips[0].X = 4 },
		func(region *textureatlas.Region) { region.Mips[0].Width = 9 },
		func(region *textureatlas.Region) { region.Mips[0].Y = 16 },
		func(region *textureatlas.Region) { region.Filter = textureatlas.FilterNearest },
	} {
		region := base
		mutate(&region)
		layout.Regions[0] = region
		if err := layout.Validate(); err == nil {
			t.Fatal("malformed filtered chain accepted")
		}
	}
	for end := range len(encoded) {
		if _, err := textureatlas.DecodeLayout(encoded[:end]); err == nil {
			t.Fatal("truncated chain accepted", end)
		}
	}
}
