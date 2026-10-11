package worldchunk

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
	"github.com/karty-game/karty-sdk/internal/binwire"
)

const (
	ShadowSchema         = "karty.world-chunk-shadow@1"
	ShadowEntryName      = "@world/chunks/shadow"
	MaxShadowEncodedSize = MaxEncodedSize
	MaxShadowTriangles   = 8192
)

// ErrShadowBudget identifies a valid world whose complete opaque caster asset
// exceeds the supported bound. Producers may omit this optional root asset for
// render-only worlds; malformed source and other errors must still fail.
var ErrShadowBudget = fmt.Errorf("opaque caster triangle budget: %w", ErrChunk)

// ShadowProxy is one bounded root-owned caster asset, independent of render
// chunk residency. It contains authored opaque rest geometry, without material
// seam junctions, center vertices, displaced-liquid subdivision or actors.
type ShadowProxy struct {
	Schema         string
	GeometrySHA256 string
	SurfaceSHA256  string
	Triangles      []ShadowTriangle
}

type ShadowTriangle struct {
	Positions   [3][3]float32
	Normal      [3]float32
	Material    uint32
	Sector      int
	Solid       int
	Surface     string
	SurfaceEdge int
}

// DecodedShadowBytes counts 12 float32 coordinates and five fixed scalar
// identity words per triangle. Hosts account for their object and GPU storage
// separately, rather than estimating from the encoded payload size.
func DecodedShadowBytes(proxy ShadowProxy) uint64 {
	return uint64(len(proxy.Triangles)) * 68
}

// CompileShadowProxy triangulates the complete opaque authored caster surfaces
// offline. Corner fans preserve legacy shadow semantics: static wall-frame UV
// bands do not change the underlying opening, sky has no caster, and a liquid
// material with opacity below one does not cast an opaque shadow.
func CompileShadowProxy(document world.Document) (ShadowProxy, error) {
	surfaces, excluded, geometry, identity, err := shadowSources(&document)
	if err != nil {
		return ShadowProxy{}, err
	}
	proxy := ShadowProxy{Schema: ShadowSchema, GeometrySHA256: geometry, SurfaceSHA256: identity,
		Triangles: make([]ShadowTriangle, 0)}
	for _, surface := range surfaces {
		material, _, _ := surfaceMaterial(&document, surface.Binding)
		if excluded[material] {
			continue
		}
		for _, polygon := range surface.Polygons {
			for corner := 1; corner+1 < len(polygon); corner++ {
				triangle, nondegenerate := shadowCorner(surface, polygon, corner, material)
				if !nondegenerate {
					continue
				}
				if len(proxy.Triangles) == MaxShadowTriangles {
					return ShadowProxy{}, ErrShadowBudget
				}
				proxy.Triangles = append(proxy.Triangles, triangle)
			}
		}
	}
	return proxy, nil
}

func EncodeShadowProxy(proxy ShadowProxy, document *world.Document) ([]byte, error) {
	if err := ValidateShadowProxy(&proxy, document); err != nil {
		return nil, err
	}
	return encodeShadowBinary(proxy)
}

func DecodeShadowProxy(encoded []byte, document *world.Document) (ShadowProxy, error) {
	var proxy ShadowProxy
	if err := decodeShadowBinary(encoded, &proxy); err != nil {
		return ShadowProxy{}, err
	}
	if err := ValidateShadowProxy(&proxy, document); err != nil {
		return ShadowProxy{}, err
	}
	return proxy, nil
}

// ValidateShadowProxy checks each supplied triangle directly against its
// canonical authored polygon corners. It checks complete semantic coverage and
// winding without constructing a mesh or any new triangle/vertex arrays.
func ValidateShadowProxy(proxy *ShadowProxy, document *world.Document) error {
	var work cooperativeWork
	if proxy == nil || proxy.Schema != ShadowSchema || proxy.Triangles == nil || len(proxy.Triangles) > MaxShadowTriangles {
		return ErrChunk
	}
	surfaces, excluded, geometry, identity, err := shadowSources(document)
	if err != nil {
		return err
	}
	if proxy.GeometrySHA256 != geometry || proxy.SurfaceSHA256 != identity {
		return fmt.Errorf("shadow source identity: %w", ErrChunk)
	}
	index := 0
	for _, surface := range surfaces {
		material, _, _ := surfaceMaterial(document, surface.Binding)
		if excluded[material] {
			continue
		}
		for _, polygon := range surface.Polygons {
			for corner := 1; corner+1 < len(polygon); corner++ {
				work.checkpoint(1024)
				expected, nondegenerate := shadowCorner(surface, polygon, corner, material)
				if !nondegenerate {
					continue
				}
				if index == len(proxy.Triangles) || proxy.Triangles[index] != expected {
					return fmt.Errorf("shadow semantic coverage at triangle %d: %w", index, ErrChunk)
				}
				index++
			}
		}
	}
	if index != len(proxy.Triangles) {
		return fmt.Errorf("extra shadow triangles: %w", ErrChunk)
	}
	return nil
}

func shadowSources(document *world.Document) ([]worldlightmap.Surface, map[uint32]bool, string, string, error) {
	surfaces, err := worldlightmap.Surfaces(document)
	if err != nil {
		return nil, nil, "", "", err
	}
	geometry, err := GeometryDigest(document)
	if err != nil {
		return nil, nil, "", "", err
	}
	base, err := worldlightmap.OfflineSurfaceDigest(document)
	if err != nil {
		return nil, nil, "", "", err
	}
	excluded := make(map[uint32]bool)
	if document.Animations != nil {
		for _, binding := range document.Animations.Materials {
			for _, preset := range document.Animations.Presets {
				if preset.Name == binding.Preset && preset.Kind == "liquid" && preset.Opacity != nil && *preset.Opacity < 1 {
					excluded[binding.Material] = true
				}
			}
		}
	}
	materials := make([]uint32, 0, len(excluded))
	for material := range excluded {
		materials = append(materials, material)
	}
	slices.Sort(materials)
	w := binwire.NewWriter("KSI1", MaxShadowEncodedSize)
	w.String(ShadowSchema, 128)
	w.String(base, 64)
	w.Count(len(materials), false, world.MaxMaterialAnimations)
	for _, material := range materials {
		w.U32(material)
	}
	encoded, err := w.Bytes()
	if err != nil {
		return nil, nil, "", "", err
	}
	digest := sha256.Sum256(encoded)
	return surfaces, excluded, geometry, hex.EncodeToString(digest[:]), nil
}

func shadowCorner(surface worldlightmap.Surface, polygon []world.Vec3, corner int, material uint32) (ShadowTriangle, bool) {
	triangle := ShadowTriangle{
		Positions: [3][3]float32{packed(polygon[0]), packed(polygon[corner]), packed(polygon[corner+1])},
		Normal:    packed(surface.Normal), Material: material, Sector: -1, Solid: -1,
		Surface: surface.Binding.Kind, SurfaceEdge: surface.Binding.Edge,
	}
	if strings.HasPrefix(surface.Binding.Kind, "sector-") {
		triangle.Sector = surface.Binding.Index
	} else {
		triangle.Solid = surface.Binding.Index
	}
	a, b, c := unpacked(triangle.Positions[0]), unpacked(triangle.Positions[1]), unpacked(triangle.Positions[2])
	orientation := dot(cross(sub(b, a), sub(c, a)), unpacked(triangle.Normal))
	if orientation < 0 {
		triangle.Positions[1], triangle.Positions[2] = triangle.Positions[2], triangle.Positions[1]
	}
	return triangle, orientation != 0
}
