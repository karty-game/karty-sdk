package worldterrain

import (
	"bytes"

	"github.com/karty-game/karty-sdk/internal/binwire"
)

// Encode writes canonical KWT1 binary without compression or filesystem paths.
func Encode(d Document) ([]byte, error) {
	if err := Validate(&d); err != nil {
		return nil, err
	}
	w := binwire.NewWriter("KWT1", MaxEncodedSize)
	w.U16(d.Version)
	for _, v := range d.Origin {
		w.F32(v)
	}
	w.F32(d.CellSize)
	w.U16(d.Columns)
	w.U16(d.Rows)
	w.U16(uint16(len(d.ExteriorSectors)))
	w.U16(uint16(len(d.Materials)))
	w.U16(uint16(len(d.Patches)))
	for _, h := range d.Heights {
		w.F32(h)
	}
	for _, n := range d.Normals {
		for _, v := range n {
			w.F32(v)
		}
	}
	w.Raw(d.Holes)
	for _, id := range d.ExteriorSectors {
		w.U16(id)
	}
	for _, m := range d.Materials {
		writeImage(w, m.Albedo)
		writeImage(w, m.Normal)
		w.F32(m.Scale)
		w.F32(m.NormalStrength)
		w.Bool(m.SideProjection)
	}
	for _, p := range d.Patches {
		w.U16(p.X)
		w.U16(p.Y)
		w.U16(p.Columns)
		w.U16(p.Rows)
		for _, id := range p.Palette {
			w.U16(id)
		}
		writeImage(w, p.Weights)
	}
	writeImage(w, d.FarColour)
	raw, err := w.Bytes()
	if err != nil {
		return nil, ErrBounds
	}
	return raw, nil
}
func writeImage(w *binwire.Writer, im Image) { w.U16(im.Width); w.U16(im.Height); w.Raw(im.Pixels) }

// Decode scans the complete bounded payload before allocating typed slices.
// Returned data owns its pixels and lattice independently of the input bytes.
func Decode(raw []byte) (Document, error) {
	if len(raw) > MaxEncodedSize {
		return Document{}, ErrBounds
	}
	if _, err := readDocument(raw, false); err != nil {
		return Document{}, err
	}
	d, err := readDocument(raw, true)
	if err != nil {
		return Document{}, err
	}
	if err = Validate(&d); err != nil {
		return Document{}, err
	}
	return d, nil
}
func readImage(r *binwire.Reader, own bool) (Image, error) {
	im := Image{Width: r.U16(), Height: r.U16()}
	if im.Width < 2 || im.Height < 2 || im.Width > MaxImageDimension || im.Height > MaxImageDimension {
		return Image{}, ErrBounds
	}
	size := int(im.Width) * int(im.Height) * 4
	r.Allocate(uint64(size))
	im.Pixels = r.Raw(size)
	if own {
		im.Pixels = bytes.Clone(im.Pixels)
	}
	if r.Err() != nil {
		return Image{}, ErrTerrain
	}
	return im, nil
}
func readDocument(raw []byte, own bool) (Document, error) {
	r := binwire.NewReader(raw, "KWT1", MaxEncodedSize)
	d := Document{Version: r.U16()}
	if r.Err() != nil {
		return Document{}, ErrTerrain
	}
	if d.Version != Version {
		return Document{}, ErrVersion
	}
	for i := range d.Origin {
		d.Origin[i] = r.F32()
	}
	d.CellSize = r.F32()
	d.Columns = r.U16()
	d.Rows = r.U16()
	ne, nm, np := int(r.U16()), int(r.U16()), int(r.U16())
	if d.Columns < 2 || d.Rows < 2 || d.Columns > MaxDimension || d.Rows > MaxDimension || ne < 1 || ne > MaxExteriorSectors || nm < 1 ||
		nm > MaxMaterials ||
		np < 1 ||
		np > MaxPatches {
		return Document{}, ErrBounds
	}
	nv := int(d.Columns) * int(d.Rows)
	nc := (int(d.Columns) - 1) * (int(d.Rows) - 1)
	r.Allocate(uint64(nv)*16 + uint64(nc) + uint64(ne)*2 + uint64(nm)*128 + uint64(np)*64)
	if own {
		d.Heights = make([]float32, nv)
		d.Normals = make([][3]float32, nv)
		d.ExteriorSectors = make([]uint16, ne)
		d.Materials = make([]Material, nm)
		d.Patches = make([]Patch, np)
	}
	for i := range nv {
		v := r.F32()
		if own {
			d.Heights[i] = v
		}
	}
	for i := range nv {
		for a := range 3 {
			v := r.F32()
			if own {
				d.Normals[i][a] = v
			}
		}
	}
	d.Holes = r.Raw(nc)
	if own {
		d.Holes = bytes.Clone(d.Holes)
	}
	for i := range ne {
		v := r.U16()
		if own {
			d.ExteriorSectors[i] = v
		}
	}
	for i := range nm {
		a, err := readImage(r, own)
		if err != nil {
			return Document{}, err
		}
		n, err := readImage(r, own)
		if err != nil {
			return Document{}, err
		}
		m := Material{Albedo: a, Normal: n, Scale: r.F32(), NormalStrength: r.F32(), SideProjection: r.Bool()}
		if own {
			d.Materials[i] = m
		}
	}
	for i := range np {
		p := Patch{X: r.U16(), Y: r.U16(), Columns: r.U16(), Rows: r.U16()}
		for a := range p.Palette {
			p.Palette[a] = r.U16()
		}
		im, err := readImage(r, own)
		if err != nil {
			return Document{}, err
		}
		p.Weights = im
		if own {
			d.Patches[i] = p
		}
	}
	far, err := readImage(r, own)
	if err != nil {
		return Document{}, err
	}
	d.FarColour = far
	if r.Done() != nil {
		return Document{}, ErrTerrain
	}
	return d, nil
}

// Clone copies all mutable backing slices for public resource ownership.
func Clone(d *Document) *Document {
	if d == nil {
		return nil
	}
	c := *d
	c.Heights = append([]float32(nil), d.Heights...)
	c.Normals = append([][3]float32(nil), d.Normals...)
	c.Holes = bytes.Clone(d.Holes)
	c.ExteriorSectors = append([]uint16(nil), d.ExteriorSectors...)
	c.Materials = append([]Material(nil), d.Materials...)
	c.Patches = append([]Patch(nil), d.Patches...)
	c.FarColour.Pixels = bytes.Clone(d.FarColour.Pixels)
	for i := range c.Materials {
		c.Materials[i].Albedo.Pixels = bytes.Clone(d.Materials[i].Albedo.Pixels)
		c.Materials[i].Normal.Pixels = bytes.Clone(d.Materials[i].Normal.Pixels)
	}
	for i := range c.Patches {
		c.Patches[i].Weights.Pixels = bytes.Clone(d.Patches[i].Weights.Pixels)
	}
	return &c
}
