package worldterrain

import (
	"math"

	"github.com/karty-game/karty-sdk/format/world"
)

func finite(v float32) bool {
	return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0) && (v != 0 || !math.Signbit(float64(v)))
}
func imageValid(im Image) bool {
	return im.Width >= 2 && im.Height >= 2 && im.Width <= MaxImageDimension && im.Height <= MaxImageDimension &&
		len(im.Pixels) == int(im.Width)*int(im.Height)*4
}

// Validate checks all data, including unused materials and hidden cells.
func Validate(d *Document) error {
	if d == nil || d.Version != Version {
		return ErrVersion
	}
	if d.Columns < 2 || d.Rows < 2 || d.Columns > MaxDimension || d.Rows > MaxDimension || !finite(d.CellSize) || d.CellSize < 0.001 ||
		d.CellSize > 1e6 {
		return ErrBounds
	}
	nv := int(d.Columns) * int(d.Rows)
	nc := (int(d.Columns) - 1) * (int(d.Rows) - 1)
	if len(d.Heights) != nv || len(d.Normals) != nv || len(d.Holes) != nc || len(d.Materials) == 0 || len(d.Materials) > MaxMaterials ||
		len(d.Patches) == 0 ||
		len(d.Patches) > MaxPatches ||
		len(d.ExteriorSectors) == 0 ||
		len(d.ExteriorSectors) > MaxExteriorSectors ||
		!imageValid(d.FarColour) {
		return ErrBounds
	}
	for axis := range 2 {
		count := int(d.Columns)
		if axis == 1 {
			count = int(d.Rows)
		}
		if !finite(d.Origin[axis]) || math.Abs(float64(d.Origin[axis])) > world.MaxCoordinate ||
			math.Abs(float64(d.Origin[axis])+float64(d.CellSize)*float64(count-1)) > world.MaxCoordinate {
			return ErrBounds
		}
		previous := d.Origin[axis]
		for index := 1; index < count; index++ {
			position := d.Origin[axis] + float32(index)*d.CellSize
			if position <= previous || math.Abs(float64(position)) > world.MaxCoordinate {
				return ErrBounds
			}
			previous = position
		}
	}
	for i, h := range d.Heights {
		if !finite(h) || math.Abs(float64(h)) > world.MaxCoordinate {
			return ErrTerrain
		}
		n := d.Normals[i]
		l := float64(n[0]*n[0] + n[1]*n[1] + n[2]*n[2])
		if !finite(n[0]) || !finite(n[1]) || !finite(n[2]) || n[2] <= 0 || math.Abs(l-1) > 0.002 {
			return ErrTerrain
		}
	}
	active := 0
	for _, v := range d.Holes {
		if v > 1 {
			return ErrTerrain
		}
		if v == 0 {
			active++
		}
	}
	if active == 0 {
		return ErrTerrain
	}
	if active > MaxTriangles/2 {
		return ErrBounds
	}
	for i, id := range d.ExteriorSectors {
		if id >= world.MaxSectors || i > 0 && d.ExteriorSectors[i-1] >= id {
			return ErrTerrain
		}
	}
	for _, m := range d.Materials {
		if !imageValid(m.Albedo) || !imageValid(m.Normal) || m.Albedo.Width != MaterialSize || m.Albedo.Height != MaterialSize ||
			m.Normal.Width != MaterialSize ||
			m.Normal.Height != MaterialSize ||
			!finite(m.Scale) ||
			m.Scale < 0.001 ||
			m.Scale > 1e6 ||
			!finite(m.NormalStrength) ||
			m.NormalStrength < 0 ||
			m.NormalStrength > 4 {
			return ErrTerrain
		}
		for i := 3; i < len(m.Albedo.Pixels); i += 4 {
			if m.Albedo.Pixels[i] != 255 || m.Normal.Pixels[i] != 255 {
				return ErrTerrain
			}
		}
	}
	covered := make([]bool, nc)
	previous := -1
	for _, p := range d.Patches {
		if p.Columns == 0 || p.Rows == 0 || int(p.X)+int(p.Columns) >= int(d.Columns) || int(p.Y)+int(p.Rows) >= int(d.Rows) ||
			!imageValid(p.Weights) {
			return ErrBounds
		}
		order := int(p.Y)*int(d.Columns) + int(p.X)
		if order <= previous {
			return ErrTerrain
		}
		previous = order
		for _, id := range p.Palette {
			if int(id) >= len(d.Materials) {
				return ErrTerrain
			}
		}
		for i := 0; i < len(p.Weights.Pixels); i += 4 {
			sum := 0
			for c := range 4 {
				sum += int(p.Weights.Pixels[i+c])
			}
			if sum != 255 {
				return ErrTerrain
			}
		}
		for y := int(p.Y); y < int(p.Y+p.Rows); y++ {
			for x := int(p.X); x < int(p.X+p.Columns); x++ {
				cell := y*(int(d.Columns)-1) + x
				if covered[cell] {
					return ErrTerrain
				}
				covered[cell] = true
			}
		}
	}
	for _, v := range covered {
		if !v {
			return ErrTerrain
		}
	}
	for i := 3; i < len(d.FarColour.Pixels); i += 4 {
		if d.FarColour.Pixels[i] != 255 {
			return ErrTerrain
		}
	}
	if DecodedBytes(d) > MaxDecodedBytes {
		return ErrBounds
	}
	return nil
}

// DecodedBytes counts scalar document storage, excluding Go object padding and
// separately generated render/collision meshes. Hosts account for both.
func DecodedBytes(d *Document) uint64 {
	if d == nil {
		return 0
	}
	n := uint64(
		len(d.Heights),
	)*4 + uint64(
		len(d.Normals),
	)*12 + uint64(
		len(d.Holes),
	) + uint64(
		len(d.ExteriorSectors),
	)*2 + uint64(
		len(d.FarColour.Pixels),
	)
	for _, m := range d.Materials {
		n += uint64(len(m.Albedo.Pixels)+len(m.Normal.Pixels)) + 17
	}
	for _, p := range d.Patches {
		n += uint64(len(p.Weights.Pixels)) + 20
	}
	return n
}
