package textureatlas

import "github.com/karty-game/karty-sdk/internal/binwire"

const binaryMagic = "KTA1"

func EncodeLayout(layout Layout) ([]byte, error) {
	if err := layout.Validate(); err != nil {
		return nil, err
	}
	w := binwire.NewWriter(binaryMagic, MaxLayoutBytes)
	w.Count(len(layout.Pages), false, MaxPages)
	for _, page := range layout.Pages {
		w.I32(page.Width)
		w.I32(page.Height)
	}
	w.Count(len(layout.Regions), false, MaxRegions)
	for _, region := range layout.Regions {
		w.String(region.Name, 1024)
		w.I32(region.Page)
		w.I32(region.X)
		w.I32(region.Y)
		w.I32(region.Width)
		w.I32(region.Height)
		w.I32(region.Gutter)
		w.I32(region.Filter)
		w.Count(region.MipCount, false, MaxMips)
		for _, mip := range region.Mips[:region.MipCount] {
			w.I32(mip.X)
			w.I32(mip.Y)
			w.I32(mip.Width)
			w.I32(mip.Height)
		}
	}
	return w.Bytes()
}

// DecodeLayout scans the entire bounded record before allocating its tables.
// Region geometry and aliases are then validated as one complete publication.
func DecodeLayout(encoded []byte) (Layout, error) {
	r := binwire.NewReader(encoded, binaryMagic, MaxLayoutBytes)
	pages, _ := r.Count(MaxPages, 8, 16)
	for range pages {
		r.I32()
		r.I32()
	}
	regions, _ := r.Count(MaxRegions, 37, 384)
	for range regions {
		r.SkipString(1024)
		for range 7 {
			r.I32()
		}
		mips, _ := r.Count(MaxMips, 16, 0)
		for range mips {
			for range 4 {
				r.I32()
			}
		}
	}
	if err := r.Done(); err != nil {
		return Layout{}, err
	}
	r = binwire.NewReader(encoded, binaryMagic, MaxLayoutBytes)
	pages, _ = r.Count(MaxPages, 8, 16)
	layout := Layout{Pages: make([]Page, pages)}
	for i := range layout.Pages {
		layout.Pages[i] = Page{Width: r.I32(), Height: r.I32()}
	}
	regions, _ = r.Count(MaxRegions, 37, 384)
	layout.Regions = make([]Region, regions)
	for i := range layout.Regions {
		layout.Regions[i] = Region{
			Name:   r.String(1024),
			Page:   r.I32(),
			X:      r.I32(),
			Y:      r.I32(),
			Width:  r.I32(),
			Height: r.I32(),
			Gutter: r.I32(),
			Filter: r.I32(),
		}
		count, _ := r.Count(MaxMips, 16, 0)
		layout.Regions[i].MipCount = count
		for m := range count {
			layout.Regions[i].Mips[m] = MipRect{X: r.I32(), Y: r.I32(), Width: r.I32(), Height: r.I32()}
		}
	}
	if err := r.Done(); err != nil {
		return Layout{}, err
	}
	if err := layout.Validate(); err != nil {
		return Layout{}, err
	}
	return layout, nil
}
