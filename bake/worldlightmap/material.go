package worldlightmapbake

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

// Material is the original sRGB albedo texture. Normal, height and AO textures
// do not alter diffuse reflectance. Inputs remain owned by the caller and must
// not change until Bake or ReflectanceDigest returns.
type Material struct {
	ID     uint32
	Albedo image.Image
}

const maxMaterialPixels = 16 * 1024 * 1024

type reflectance struct {
	image           image.Image
	mapping         *world.SurfaceUV
	origin, tangent vec
	cap             bool
}

func materialOf(d *world.Document, b worldlightmap.Binding) (uint32, *world.SurfaceUV, vec, vec, bool) {
	if b.Kind == "sector-floor" {
		s := d.Sectors[b.Index]
		return s.FloorMaterial, s.FloorUV, vec{}, vec{}, true
	}
	if b.Kind == "sector-ceiling" {
		s := d.Sectors[b.Index]
		return s.CeilingMaterial, s.CeilingUV, vec{}, vec{}, true
	}
	if b.Kind == "sector-wall" {
		w := d.Sectors[b.Index].Walls[b.Edge]
		return w.Material, w.UV, vec{X: w.Start.X, Y: w.Start.Y}, unit(vec{X: w.End.X - w.Start.X, Y: w.End.Y - w.Start.Y}), false
	}
	s := d.StaticSolids.Items[b.Index]
	if b.Kind == "solid-top" {
		return s.TopMaterial, s.TopUV, vec{}, vec{}, true
	}
	if b.Kind == "solid-bottom" {
		return s.BottomMaterial, s.BottomUV, vec{}, vec{}, true
	}
	a, c := s.Footprint[b.Edge], s.Footprint[(b.Edge+1)%len(s.Footprint)]
	return s.SideMaterial, s.SideUV, vec{X: a.X, Y: a.Y}, unit(vec{X: c.X - a.X, Y: c.Y - a.Y}), false
}
func materialImages(materials []Material) (map[uint32]image.Image, error) {
	if len(materials) > 16384 {
		return nil, fmt.Errorf("albedo material count exceeds 16384")
	}
	result := make(map[uint32]image.Image, len(materials))
	pixels := int64(0)
	for _, m := range materials {
		if m.ID == 0 || m.Albedo == nil {
			return nil, fmt.Errorf("invalid albedo material %d", m.ID)
		}
		if _, ok := result[m.ID]; ok {
			return nil, fmt.Errorf("duplicate albedo material %d", m.ID)
		}
		b := m.Albedo.Bounds()
		w, h := int64(b.Dx()), int64(b.Dy())
		if w <= 0 || h <= 0 || w > maxMaterialPixels || h > maxMaterialPixels || w*h > maxMaterialPixels-pixels {
			return nil, fmt.Errorf("albedo textures exceed %d pixels", maxMaterialPixels)
		}
		pixels += w * h
		result[m.ID] = m.Albedo
	}
	return result, nil
}

// ReflectanceDigest identifies decoded texture pixels and their material/UV
// assignments. It deliberately excludes actors, ambient and material AO.
// Unused materials do not invalidate a bake. No private engine is required.
func ReflectanceDigest(document world.Document, materials []Material) (string, error) {
	surfaces, err := worldlightmap.Surfaces(&document)
	if err != nil {
		return "", err
	}
	images, err := materialImages(materials)
	if err != nil {
		return "", err
	}
	type assignment struct {
		Binding  worldlightmap.Binding
		Material uint32
		UV       *world.SurfaceUV
	}
	assignments := make([]assignment, 0, len(surfaces))
	used := make(map[uint32]bool)
	for _, s := range surfaces {
		if len(s.Polygons) == 0 {
			continue
		}
		id, uv, _, _, _ := materialOf(&document, s.Binding)
		if images[id] == nil {
			return "", fmt.Errorf("missing albedo material %d", id)
		}
		assignments = append(assignments, assignment{s.Binding, id, uv})
		used[id] = true
	}
	encoded, err := json.Marshal(assignments)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	hash.Write([]byte("karty.offline-reflectance@1\x00"))
	hash.Write(encoded)
	ids := make([]uint32, 0, len(used))
	for id := range used {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var header [12]byte
	var pixels [4096]byte
	for _, id := range ids {
		im := images[id]
		b := im.Bounds()
		binary.LittleEndian.PutUint32(header[:4], id)
		binary.LittleEndian.PutUint32(header[4:8], uint32(b.Dx()))
		binary.LittleEndian.PutUint32(header[8:], uint32(b.Dy()))
		hash.Write(header[:])
		filled := 0
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				c := albedoPixel(im, x, y)
				pixels[filled], pixels[filled+1], pixels[filled+2], pixels[filled+3] = c.R, c.G, c.B, c.A
				filled += 4
				if filled == len(pixels) {
					hash.Write(pixels[:])
					filled = 0
				}
			}
		}
		if filled > 0 {
			hash.Write(pixels[:filled])
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// Common decoded PNG/JPEG forms avoid boxing a colour interface for every
// bounce ray or digest pixel. Fallback retains arbitrary image.Image support.
func albedoPixel(im image.Image, x, y int) color.NRGBA {
	switch im := im.(type) {
	case *image.NRGBA:
		return im.NRGBAAt(x, y)
	case *image.YCbCr:
		c := im.YCbCrAt(x, y)
		r, g, b := color.YCbCrToRGB(c.Y, c.Cb, c.Cr)
		return color.NRGBA{R: r, G: g, B: b, A: 255}
	case *image.RGBA:
		c := im.RGBAAt(x, y)
		if c.A == 0 {
			return color.NRGBA{}
		}
		if c.A == 255 {
			return color.NRGBA{R: c.R, G: c.G, B: c.B, A: c.A}
		}
		return color.NRGBA{R: uint8(uint32(c.R) * 65535 / uint32(c.A) >> 8), G: uint8(uint32(c.G) * 65535 / uint32(c.A) >> 8), B: uint8(uint32(c.B) * 65535 / uint32(c.A) >> 8), A: c.A}
	case *image.Gray:
		v := im.GrayAt(x, y).Y
		return color.NRGBA{R: v, G: v, B: v, A: 255}
	default:
		return color.NRGBAModel.Convert(im.At(x, y)).(color.NRGBA)
	}
}
func linear(byteValue uint8) float64 {
	v := float64(byteValue) / 255
	if v <= .04045 {
		return v / 12.92
	}
	return math.Pow((v+.055)/1.055, 2.4)
}
func (r reflectance) texel(u, v float64) vec {
	b := r.image.Bounds()
	x := b.Min.X + int(math.Floor((u-math.Floor(u))*float64(b.Dx())))
	y := b.Min.Y + int(math.Floor((v-math.Floor(v))*float64(b.Dy())))
	c := albedoPixel(r.image, x, y)
	return vec{X: math.Min(worldlightmap.MaxDiffuseReflectance, linear(c.R)), Y: math.Min(worldlightmap.MaxDiffuseReflectance, linear(c.G)), Z: math.Min(worldlightmap.MaxDiffuseReflectance, linear(c.B))}
}
func uvValue(p world.UVPlane, v vec) float64 { return p.X*v.X + p.Y*v.Y + p.Z*v.Z + p.Offset }
func (r reflectance) sample(p vec) vec {
	if r.mapping != nil {
		result := vec{}
		for i, projection := range r.mapping.Projections {
			if w := r.mapping.Weights[i]; w > 0 {
				result = add(result, scale(r.texel(uvValue(projection.U, p), uvValue(projection.V, p)), w))
			}
		}
		return result
	}
	if r.cap {
		return r.texel(p.X, p.Y)
	}
	return r.texel(dot(sub(p, r.origin), r.tangent), p.Z)
}
