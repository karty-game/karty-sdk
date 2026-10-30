package worldlightmapbake

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"image"
	"slices"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldvegetation"
	"github.com/karty-game/karty-sdk/internal/binwire"
)

// Vegetation supplies immutable, processed cutout albedos and the same seeded
// layers used for visual foliage. Shadows and diffuse bounce use the rest pose;
// visual wind and distance thinning do not alter this fixed illumination.
// This build-only input adds no cartridge data or runtime shadow resources.
type Vegetation struct {
	Document  worldvegetation.Document
	Materials []Material
}

// MaxVegetationTriangles bounds additional offline transport working memory.
// Complete layer candidate counts are checked before expanding any species.
const MaxVegetationTriangles = 262144

func vegetationInputDigest(input *Vegetation) (string, error) {
	if input == nil {
		return "", nil
	}
	encoded, err := worldvegetation.Encode(input.Document)
	if err != nil {
		return "", err
	}
	images, err := materialImages(input.Materials)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	h.Write([]byte("karty.static-vegetation-rest@1\x00"))
	h.Write(encoded)
	ids := make([]uint32, 0, len(input.Document.Presets))
	for _, preset := range input.Document.Presets {
		ids = append(ids, preset.Texture)
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	var header [12]byte
	var pixels [4096]byte
	for _, id := range ids {
		source := images[id]
		if source == nil {
			return "", fmt.Errorf("vegetation albedo %d is absent", id)
		}
		bounds := source.Bounds()
		// #nosec G115 -- materialImages bounds positive dimensions to 16 Mi-pixels.
		binary.LittleEndian.PutUint32(header[0:4], id)
		binary.LittleEndian.PutUint32(header[4:8], uint32(bounds.Dx()))
		binary.LittleEndian.PutUint32(header[8:12], uint32(bounds.Dy()))
		h.Write(header[:])
		filled := 0
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				color := albedoPixel(source, x, y)
				pixels[filled], pixels[filled+1], pixels[filled+2], pixels[filled+3] = color.R, color.G, color.B, color.A
				filled += 4
				if filled == len(pixels) {
					h.Write(pixels[:])
					filled = 0
				}
			}
		}
		h.Write(pixels[:filled])
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func writeVegetationInput(w *binwire.Writer, input *Vegetation) error {
	if input == nil {
		return nil
	}
	digest, err := vegetationInputDigest(input)
	if err != nil {
		return err
	}
	w.String("static-vegetation-rest@1", 128)
	w.String(digest, 64)
	return nil
}

func (p *preparedTransport) appendVegetation(ctx context.Context, document *world.Document, options Options) error {
	input := options.Vegetation
	if input == nil {
		return nil
	}
	layers, bound, err := prepareVegetationLayers(document, input, options.Terrain)
	if err != nil {
		return err
	}
	images, err := materialImages(input.Materials)
	if err != nil {
		return err
	}
	albedos := make(map[uint32]*image.NRGBA)
	p.accel.vegetationStart = len(p.surfaces)
	p.accel.vegetation = make([]vegetationReflectance, 0, bound)
	var species [4][]float32
	for _, layer := range layers {
		if err := ctx.Err(); err != nil {
			return err
		}
		albedo := albedos[layer.preset.Texture]
		if albedo == nil {
			source := images[layer.preset.Texture]
			if source == nil {
				return fmt.Errorf("vegetation albedo %d is absent", layer.preset.Texture)
			}
			albedo, err = prepareAlbedo(ctx, source)
			if err != nil {
				return err
			}
			albedos[layer.preset.Texture] = albedo
		}
		geometry := layer.preset.Geometry
		if species[geometry] == nil {
			species[geometry] = worldvegetation.SpeciesVertices(geometry)
		}
		if err := p.appendVegetationLayer(ctx, layer, species[geometry], albedo); err != nil {
			return err
		}
	}
	return nil
}

func validateVegetationForBake(document *world.Document, options Options, worldImages map[uint32]image.Image) error {
	if options.Vegetation == nil {
		return nil
	}
	if _, _, err := prepareVegetationLayers(document, options.Vegetation, options.Terrain); err != nil {
		return err
	}
	images, err := materialImages(options.Vegetation.Materials)
	if err != nil {
		return err
	}
	var pixels int64
	for _, set := range []map[uint32]image.Image{worldImages, images} {
		for _, source := range set {
			bounds := source.Bounds()
			pixels += int64(bounds.Dx()) * int64(bounds.Dy())
		}
	}
	if pixels > maxMaterialPixels {
		return fmt.Errorf("world and vegetation albedos exceed %d pixels", maxMaterialPixels)
	}
	return nil
}

func vegetationReflectanceDigest(base string, input *Vegetation) (string, error) {
	if input == nil {
		return base, nil
	}
	digest, err := vegetationInputDigest(input)
	if err != nil {
		return "", err
	}
	combined := sha256.Sum256([]byte(base + digest))
	return hex.EncodeToString(combined[:]), nil
}
