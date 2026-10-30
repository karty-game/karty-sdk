package worldchunk

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

func validateSpatialCatalog(catalog *Catalog, document *world.Document) error {
	base := *catalog
	base.Schema = FixedSchema
	base.Spatial = nil
	base.Surfaces = nil
	base.TerrainSHA256 = ""
	base.Chunks = slices.Clone(catalog.Chunks)
	for i, c := range base.Chunks {
		if c.CollisionBytes == 0 || c.CollisionBytes > MaxTriangles*484 || c.CollisionBytes%484 != 0 {
			return fmt.Errorf("primary collision reservation: %w", ErrChunk)
		}
		if !spatialAtlasBytes(c.LightmapBytes) {
			return ErrChunk
		}
		base.Chunks[i].LightmapBytes = 512 * 512 * 12
		base.Chunks[i].CollisionBytes = 0
	}
	if err := validateLegacyCatalog(&base, document); err != nil {
		return err
	}
	if len(catalog.Spatial) == 0 || len(catalog.Spatial) > MaxSpatialChunks {
		return ErrChunk
	}
	surfaces, err := worldlightmap.Surfaces(document)
	if err != nil {
		return err
	}
	if len(surfaces) != len(catalog.Surfaces) {
		return fmt.Errorf("incomplete root surface ownership: %w", ErrChunk)
	}
	solidCount := 0
	if document.StaticSolids != nil {
		solidCount = len(document.StaticSolids.Items)
	}
	terrainReplacement := false
	owners := make(map[worldlightmap.Binding]SurfaceOwnership, len(surfaces))
	sourceFaces := make(map[worldlightmap.Binding]worldlightmap.Surface, len(surfaces))
	liquids := displacedLiquidMaterials(document)
	for i, s := range surfaces {
		o := catalog.Surfaces[i]
		b := s.Binding
		b.Chart = -1
		if o.Binding != b || o.Owner < 0 || o.Owner >= len(catalog.Chunks) {
			return ErrChunk
		}
		c := catalog.Chunks[o.Owner]
		indexes := c.Solids
		if strings.HasPrefix(b.Kind, "sector-") {
			indexes = c.Sectors
		}
		if _, ok := slices.BinarySearch(indexes, b.Index); !ok {
			return ErrChunk
		}
		if o.Kind != "primary" && o.Kind != "spatial" && o.Kind != "terrain" || o.Kind == "terrain" && b.Kind != "sector-floor" {
			return ErrChunk
		}
		if o.Kind == "terrain" {
			terrainReplacement = true
		}
		owners[b] = o
		sourceFaces[b] = s
	}
	if terrainReplacement && !hash(catalog.TerrainSHA256) || !terrainReplacement && catalog.TerrainSHA256 != "" {
		return ErrChunk
	}
	sharedLiquidEdgeStep := 0.0
	coverageWork := 0
	tiles := make(map[worldlightmap.Binding][][4]float64)
	for i, s := range catalog.Spatial {
		c := s.Chunk
		if c.ID != i || s.Parent < 0 || s.Parent >= len(catalog.Chunks) || !hash(c.ContentSHA256) || c.Size == 0 ||
			c.Size > MaxEncodedBytes ||
			c.MeshBytes == 0 ||
			c.MeshBytes > MaxTriangles*484 ||
			c.MeshBytes%484 != 0 ||
			c.LightingBytes == 0 ||
			c.LightingBytes > worldlightmap.MaxFixedLightingSize ||
			c.TerrainBytes != 0 ||
			c.CollisionBytes != 0 ||
			!validBounds(c.Bounds) ||
			!finite(s.MaxDistance) ||
			s.MaxDistance < 0 ||
			s.MaxDistance > world.MaxCoordinate*2 {
			return fmt.Errorf("spatial descriptor %d: %w", i, ErrChunk)
		}
		validPage := false
		for _, size := range []uint64{32, 64, 128, 256, 512, 1024} {
			if c.LightmapBytes == size*size*12 {
				validPage = true
			}
		}
		if !validPage && c.LightmapBytes != 0 || !indexes(c.Sectors, len(document.Sectors), true) || !indexes(c.Solids, solidCount, false) {
			return ErrChunk
		}
		selection := worldlightmap.SurfaceSelection{
			Bindings:              s.Bindings,
			LiquidTiles:           s.LiquidTiles,
			Clip:                  &s.Tile,
			LiquidMaxSubdivisions: s.LiquidMaxSubdivisions,
			LiquidEdgeStep:        s.LiquidEdgeStep,
		}
		if err := worldlightmap.ValidateSurfaceSelection(
			&selection,
			worldlightmap.LayoutAdmission{SectorCount: len(document.Sectors), SolidCount: solidCount},
		); err != nil {
			return err
		}
		if len(s.Bindings) == 0 {
			return ErrChunk
		}
		coverageWork += len(s.Bindings)
		if coverageWork > 65536 {
			return ErrChunk
		}
		for _, binding := range s.Bindings {
			o, ok := owners[binding]
			if !ok || o.Kind != "spatial" || o.Owner != s.Parent {
				return ErrChunk
			}
			list := c.Solids
			if strings.HasPrefix(binding.Kind, "sector-") {
				list = c.Sectors
			}
			if _, ok := slices.BinarySearch(list, binding.Index); !ok {
				return ErrChunk
			}
			surface := sourceFaces[binding]
			material, _, _ := surfaceMaterial(document, binding)
			padding := 0.0
			if horizontalLiquid(surface.Normal, material, liquids) {
				if s.LiquidEdgeStep <= 0 {
					return fmt.Errorf("spatial liquid needs a shared boundary grid: %w", ErrChunk)
				}
				if sharedLiquidEdgeStep == 0 {
					sharedLiquidEdgeStep = s.LiquidEdgeStep
				} else if sharedLiquidEdgeStep != s.LiquidEdgeStep {
					return fmt.Errorf("spatial liquid boundary grids differ: %w", ErrChunk)
				}
				padding = math.Abs(liquids[material].Amplitude)
			}
			for _, polygon := range renderPolygons(document, surface) {
				for _, point := range worldlightmap.ClipSurfacePolygon(polygon, s.Tile) {
					for axis, value := range [3]float64{point.X, point.Y, point.Z} {
						pad := 0.0
						if axis == 2 {
							pad = padding
						}
						if value-pad < c.Bounds[axis]-coordinateTolerance(value) ||
							value+pad > c.Bounds[axis+3]+coordinateTolerance(value) {
							return fmt.Errorf("spatial bounds omit source or wave displacement: %w", ErrChunk)
						}
					}
				}
			}
			tiles[binding] = append(tiles[binding], s.Tile)
		}
	}
	// Coverage proof compares the exact convex source clips and excludes any
	// pair with positive shared source area. Adjacent edge contact is permitted.
	coverageWork = 0
	for _, surface := range surfaces {
		binding := surface.Binding
		binding.Chart = -1
		o := owners[binding]
		if o.Kind != "spatial" {
			continue
		}
		rectangles := tiles[binding]
		if len(rectangles) == 0 {
			return fmt.Errorf("delegated face has no tiles: %w", ErrChunk)
		}
		polygons := renderPolygons(document, surface)
		expected, total := surfaceArea(polygons, surface.Normal), 0.0
		for i, tile := range rectangles {
			for _, p := range polygons {
				total += surfaceArea([][]world.Vec3{worldlightmap.ClipSurfacePolygon(p, tile)}, surface.Normal)
			}
			for _, other := range rectangles[:i] {
				coverageWork++
				if coverageWork > 4*1024*1024 {
					return fmt.Errorf("spatial proof work budget: %w", ErrChunk)
				}
				overlap := [4]float64{max(tile[0], other[0]), max(tile[1], other[1]), min(tile[2], other[2]), min(tile[3], other[3])}
				if overlap[0] > overlap[2] || overlap[1] > overlap[3] {
					continue
				}
				for _, p := range polygons {
					if surfaceArea([][]world.Vec3{worldlightmap.ClipSurfacePolygon(p, overlap)}, surface.Normal) > 1e-7 {
						return fmt.Errorf("overlapping spatial source coverage: %w", ErrChunk)
					}
				}
			}
		}
		if math.Abs(total-expected) > max(1, expected)*1e-7 {
			return fmt.Errorf("incomplete spatial source coverage %s %d/%d: %w", binding.Kind, binding.Index, binding.Edge, ErrChunk)
		}
	}
	return nil
}
func surfaceArea(polygons [][]world.Vec3, normal world.Vec3) float64 {
	area := 0.0
	for _, p := range polygons {
		if len(p) < 3 {
			continue
		}
		sum := world.Vec3{}
		for i, v := range p {
			sum = add(sum, cross(v, p[(i+1)%len(p)]))
		}
		area += math.Abs(dot(sum, normal)) * .5
	}
	return area
}

// ValidateStreamSelection binds selected face authority to an admitted root.
// It checks exact identities and rectangles without rediscovering geometry.
func ValidateStreamSelection(catalog *Catalog, id int, layout *worldlightmap.Layout) error {
	if catalog == nil || catalog.Schema != SpatialSchema || layout == nil || layout.Selection == nil {
		return ErrChunk
	}
	chunk, ok := catalog.StreamChunk(id)
	if !ok {
		return ErrChunk
	}
	if layout.Receivers == nil || !slices.Equal(layout.Receivers.Sectors, chunk.Sectors) ||
		!slices.Equal(layout.Receivers.Solids, chunk.Solids) {
		return ErrChunk
	}
	var expected worldlightmap.SurfaceSelection
	if id < len(catalog.Chunks) {
		for _, o := range catalog.Surfaces {
			if o.Kind == "primary" && o.Owner == id {
				expected.Bindings = append(expected.Bindings, o.Binding)
			}
		}
	} else {
		s := catalog.Spatial[id-len(catalog.Chunks)]
		expected.Bindings = s.Bindings
		expected.LiquidTiles = s.LiquidTiles
		expected.LiquidMaxSubdivisions = s.LiquidMaxSubdivisions
		expected.LiquidEdgeStep = s.LiquidEdgeStep
		expected.Clip = &s.Tile
	}
	// A planner may preserve a global allowed-face list. Canonical admission
	// accepts only this child's source ownership, never irrelevant faces.
	actual := layout.Selection
	if !worldlightmap.EqualSurfaceSelections(actual, &expected) {
		return ErrChunk
	}
	return nil
}

func spatialAtlasBytes(bytes uint64) bool {
	for _, size := range []uint64{32, 64, 128, 256, 512, 1024} {
		if bytes == size*size*12 {
			return true
		}
	}
	return false
}
