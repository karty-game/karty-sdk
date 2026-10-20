// Package textureatlas defines immutable, build-time image atlases shared by
// sprites, UI, animation frames, vegetation and particles.
package textureatlas

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/karty-game/karty-sdk/codec/qoi"
	"github.com/karty-game/karty-sdk/format/asset"
)

const (
	Feature        = "texture/atlas@1"
	MetadataKey    = "kartyTextureAtlas"
	LayoutEntry    = "@atlas/layout"
	PagePrefix     = "@atlas/page/"
	MaxPages       = 4096
	MaxRegions     = 4096
	MaxLayoutBytes = 1024 * 1024
	MaxPageBytes   = 8 * 1024 * 1024
	Gutter         = 2
	MaxMips        = 9
	MaxRects       = 8192
	FilterNearest  = 0
	FilterColor    = 1
	FilterNormal   = 2
)

var ErrAtlas = errors.New("invalid compiled texture atlas")

type Page struct{ Width, Height int }

// MipRect identifies one independently filtered mip interior in its region's page.
type MipRect struct{ X, Y, Width, Height int }

// Region retains logical dimensions, straight-alpha pixels and build-time mips.
// Names are game asset names or canonical level IDs. A zero gutter requires a
// dedicated full-page image. Mips share the base region's physical page.
type Region struct {
	Filter, MipCount                  int
	Mips                              [MaxMips]MipRect
	Name                              string
	Page, X, Y, Width, Height, Gutter int
}

type Layout struct {
	Pages   []Page
	Regions []Region
}

// Bundle borrows immutable encoded pages. Validate scans all pages before a host
// allocates pixels or publishes any resource; each physical page is charged once.
type Bundle struct {
	Layout Layout
	Pages  [][]byte
}

func PageEntryName(index int) string { return fmt.Sprintf("%s%08x", PagePrefix, index) }

func IsEntry(name string) bool { return strings.HasPrefix(name, "@atlas/") }

func (layout Layout) DecodedBytes() uint64 {
	var total uint64
	for _, page := range layout.Pages {
		if page.Width < 1 || page.Height < 1 || page.Width > asset.MaxTextureDimension || page.Height > asset.MaxTextureDimension {
			return asset.MaxDecodedTextures + 1
		}
		total += uint64(page.Width) * uint64(page.Height) * 4
	}
	return total
}

func (layout Layout) Validate() error {
	if len(layout.Pages) == 0 || len(layout.Pages) > MaxPages || len(layout.Regions) == 0 || len(layout.Regions) > MaxRegions ||
		layout.DecodedBytes() > asset.MaxDecodedTextures {
		return ErrAtlas
	}
	used := make([]bool, len(layout.Pages))
	rectangles := make([]Region, 0, len(layout.Regions))
	previous := ""
	for _, region := range layout.Regions {
		if region.Name <= previous || len(region.Name) > 1024 || !utf8.ValidString(region.Name) || IsEntry(region.Name) ||
			region.Filter < FilterNearest ||
			region.Filter > FilterNormal ||
			region.MipCount < 0 ||
			region.MipCount > MaxMips ||
			region.Filter == FilterNearest && region.MipCount != 0 {
			return ErrAtlas
		}
		if err := layout.validateRect(region); err != nil {
			return err
		}
		rectangles = append(rectangles, region)
		width, height := region.Width, region.Height
		expected := 0
		if region.Filter != FilterNearest {
			for min(width/2, height/2) >= 8 {
				width, height = width/2, height/2
				expected++
			}
		}
		if region.MipCount != expected {
			return ErrAtlas
		}
		width, height = region.Width, region.Height
		for i, mip := range region.Mips {
			if i >= region.MipCount {
				if mip != (MipRect{}) {
					return ErrAtlas
				}
				continue
			}
			width, height = width/2, height/2
			if mip.Width != width || mip.Height != height {
				return ErrAtlas
			}
			rect := Region{Page: region.Page, X: mip.X, Y: mip.Y, Width: mip.Width, Height: mip.Height, Gutter: Gutter}
			if err := layout.validateRect(rect); err != nil {
				return err
			}
			rectangles = append(rectangles, rect)
		}
		if len(rectangles) > MaxRects {
			return ErrAtlas
		}
		used[region.Page], previous = true, region.Name
	}
	for _, present := range used {
		if !present {
			return ErrAtlas
		}
	}
	for i, rect := range rectangles {
		for _, other := range rectangles[:i] {
			if other.Page != rect.Page || identicalRect(other, rect) {
				continue
			}
			if rect.X-rect.Gutter < other.X+other.Width+other.Gutter && other.X-other.Gutter < rect.X+rect.Width+rect.Gutter &&
				rect.Y-rect.Gutter < other.Y+other.Height+other.Gutter &&
				other.Y-other.Gutter < rect.Y+rect.Height+rect.Gutter {
				return ErrAtlas
			}
		}
	}
	return nil
}

func (layout Layout) validateRect(region Region) error {
	if region.Page < 0 || region.Page >= len(layout.Pages) || region.Width < 1 || region.Height < 1 ||
		(region.Gutter != 0 && region.Gutter != Gutter) {
		return ErrAtlas
	}
	page := layout.Pages[region.Page]
	if uint64(page.Width)*uint64(page.Height) > asset.MaxTexturePixels || region.X < region.Gutter || region.Y < region.Gutter ||
		region.Width > page.Width-region.Gutter ||
		region.Height > page.Height-region.Gutter ||
		region.X > page.Width-region.Width-region.Gutter ||
		region.Y > page.Height-region.Height-region.Gutter {
		return ErrAtlas
	}
	if region.Gutter == 0 && (region.X != 0 || region.Y != 0 || region.Width != page.Width || region.Height != page.Height) {
		return ErrAtlas
	}
	return nil
}

func identicalRect(a, b Region) bool {
	return a.X == b.X && a.Y == b.Y && a.Width == b.Width && a.Height == b.Height && a.Gutter == b.Gutter
}

func (bundle Bundle) Validate() error {
	if err := bundle.Layout.Validate(); err != nil {
		return err
	}
	if len(bundle.Pages) != len(bundle.Layout.Pages) {
		return ErrAtlas
	}
	for index, encoded := range bundle.Pages {
		if len(encoded) == 0 || len(encoded) > MaxPageBytes {
			return ErrAtlas
		}
		metadata, err := qoi.Inspect(encoded)
		page := bundle.Layout.Pages[index]
		if err != nil || int(metadata.Width) != page.Width || int(metadata.Height) != page.Height ||
			metadata.Channels != qoi.ChannelsRGBA || metadata.Colorspace != qoi.ColorspaceSRGB {
			return ErrAtlas
		}
	}
	for _, encoded := range bundle.Pages {
		if _, err := qoi.Validate(encoded); err != nil {
			return fmt.Errorf("%w: %w", ErrAtlas, err)
		}
	}
	return nil
}
