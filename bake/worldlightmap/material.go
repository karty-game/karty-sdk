package worldlightmapbake

import (
	"context"
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
// not change until Bake or ReflectanceDigest returns. ID zero is the default
// material: omission supplies opaque sRGB (192,192,192); an explicit image
// represents a caller-provided default atlas albedo.
type Material struct {
	ID     uint32
	Albedo image.Image
}

const maxMaterialPixels = 16 * 1024 * 1024

type reflectance struct {
	image           *image.NRGBA
	width, height   float64
	mapping         *world.SurfaceUV
	origin, tangent vec
	cap             bool
}

// Keep original source pixels for digest identity; normalize only sampled
// images once, without filtering, rescaling or changing colour conversion.
func prepareAlbedo(ctx context.Context, source image.Image) (*image.NRGBA, error) {
	if pixels, ok := source.(*image.NRGBA); ok {
		return pixels, nil
	}
	pixels := image.NewNRGBA(source.Bounds())
	b := pixels.Rect
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if (x-b.Min.X)%1024 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
			c := albedoPixel(source, x, y)
			i := pixels.PixOffset(x, y)
			pixels.Pix[i], pixels.Pix[i+1], pixels.Pix[i+2], pixels.Pix[i+3] = c.R, c.G, c.B, c.A
		}
	}
	return pixels, nil
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
		if m.Albedo == nil {
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
	if result[0] == nil {
		// Match the CLI's opaque default atlas material, without a source asset.
		result[0] = &image.NRGBA{Pix: []byte{192, 192, 192, 255}, Stride: 4, Rect: image.Rect(0, 0, 1, 1)}
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
	return reflectanceDigest(context.Background(), &document, surfaces, images)
}

func reflectanceDigest(
	ctx context.Context,
	document *world.Document,
	surfaces []worldlightmap.Surface,
	images map[uint32]image.Image,
) (string, error) {
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
		id, uv, _, _, _ := materialOf(document, s.Binding)
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
				if (x-b.Min.X)%1024 == 0 {
					if err := ctx.Err(); err != nil {
						return "", err
					}
				}
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
			return color.NRGBA(c)
		}
		return color.NRGBA{
			R: uint8(uint32(c.R) * 65535 / uint32(c.A) >> 8),
			G: uint8(uint32(c.G) * 65535 / uint32(c.A) >> 8),
			B: uint8(uint32(c.B) * 65535 / uint32(c.A) >> 8),
			A: c.A,
		}
	case *image.Gray:
		v := im.GrayAt(x, y).Y
		return color.NRGBA{R: v, G: v, B: v, A: 255}
	default:
		return color.NRGBAModel.Convert(im.At(x, y)).(color.NRGBA)
	}
}

var linearBytes = func() [256]float64 {
	var values [256]float64
	for i := range values {
		values[i] = linearValue(uint8(i))
	}
	return values
}()

var diffuseBytes = func() [256]float64 {
	values := linearBytes
	for i := range values {
		values[i] = math.Min(worldlightmap.MaxDiffuseReflectance, values[i])
	}
	return values
}()

func linearValue(byteValue uint8) float64 {
	v := float64(byteValue) / 255
	if v <= .04045 {
		return v / 12.92
	}
	return math.Pow((v+.055)/1.055, 2.4)
}
func (r reflectance) texel(u, v float64) vec {
	b := r.image.Rect
	x := b.Min.X + int(math.Floor((u-math.Floor(u))*r.width))
	y := b.Min.Y + int(math.Floor((v-math.Floor(v))*r.height))
	c := r.image.NRGBAAt(x, y)
	return vec{X: diffuseBytes[c.R], Y: diffuseBytes[c.G], Z: diffuseBytes[c.B]}
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
