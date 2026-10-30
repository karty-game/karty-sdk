package worldchunk

import (
	"encoding/hex"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
	"github.com/karty-game/karty-sdk/format/worldterrain"
)

func EncodeCatalog(catalog Catalog, document *world.Document) ([]byte, error) {
	if err := ValidateCatalog(&catalog, document); err != nil {
		return nil, err
	}
	return encodeCatalogBinary(catalog)
}

func DecodeCatalog(encoded []byte, document *world.Document) (Catalog, error) {
	var catalog Catalog
	if err := decodeCatalogBinary(encoded, &catalog); err != nil {
		return Catalog{}, err
	}
	if err := ValidateCatalog(&catalog, document); err != nil {
		return Catalog{}, err
	}
	return catalog, nil
}

func EncodeMesh(mesh Mesh, document *world.Document) ([]byte, error) {
	if err := ValidateMesh(&mesh, document); err != nil {
		return nil, err
	}
	return encodeMeshBinary(mesh)
}

func DecodeMesh(encoded []byte, document *world.Document) (Mesh, error) {
	var mesh Mesh
	if err := decodeMeshBinary(encoded, &mesh); err != nil {
		return Mesh{}, err
	}
	if err := ValidateMesh(&mesh, document); err != nil {
		return Mesh{}, err
	}
	return mesh, nil
}

func ValidateCatalog(catalog *Catalog, document *world.Document) error {
	if catalog != nil && catalog.Schema == SpatialSchema {
		return validateSpatialCatalog(catalog, document)
	}
	if catalog != nil {
		if len(catalog.Spatial) != 0 || len(catalog.Surfaces) != 0 || catalog.TerrainSHA256 != "" {
			return ErrChunk
		}
		for _, chunk := range catalog.Chunks {
			if chunk.CollisionBytes != 0 {
				return ErrChunk
			}
		}
	}
	return validateLegacyCatalog(catalog, document)
}

func validateLegacyCatalog(catalog *Catalog, document *world.Document) error {
	if err := world.Validate(document); err != nil {
		return err
	}
	digest, err := GeometryDigest(document)
	if err != nil {
		return err
	}
	if catalog == nil || (catalog.Schema != Schema && catalog.Schema != FixedSchema) || catalog.GeometrySHA256 != digest ||
		len(catalog.Chunks) == 0 ||
		len(catalog.Chunks) > len(document.Sectors) ||
		len(catalog.SectorChunks) != len(document.Sectors) ||
		len(catalog.Candidates) != len(document.Sectors) {
		return ErrChunk
	}
	if err := validateCatalogBakedLights(catalog, document); err != nil {
		return err
	}
	sectorOwners := make([]int, len(document.Sectors))
	for i := range sectorOwners {
		sectorOwners[i] = -1
	}
	solidCount := 0
	if document.StaticSolids != nil {
		solidCount = len(document.StaticSolids.Items)
	}
	solidSeen := make([]bool, solidCount)
	renderTriangles := uint64(world.SolidRendererTriangles(document.SolidRenderers))
	for i, chunk := range catalog.Chunks {
		if catalog.Schema == FixedSchema &&
			(chunk.LightingBytes == 0 || chunk.LightingBytes > worldlightmap.MaxFixedLightingSize || chunk.LightmapBytes == 0) ||
			catalog.Schema == Schema && (chunk.LightingBytes != 0 || chunk.TerrainBytes != 0) ||
			chunk.TerrainBytes > worldterrain.MaxDecodedBytes+worldterrain.MaxVertices*worldterrain.MaxPatches*24+worldterrain.MaxTriangles*12 {
			return ErrChunk
		}
		if chunk.ID != i || !hash(chunk.ContentSHA256) || chunk.Size == 0 || chunk.Size > MaxEncodedBytes || chunk.MeshBytes == 0 ||
			chunk.MeshBytes > 64*1024*1024 ||
			chunk.MeshBytes%484 != 0 ||
			(chunk.LightmapBytes != 0 && chunk.LightmapBytes != 512*512*12 && chunk.LightmapBytes != 1024*1024*12) ||
			!indexes(chunk.Sectors, len(document.Sectors), true) ||
			!indexes(chunk.Solids, solidCount, false) ||
			!validBounds(chunk.Bounds) ||
			!connected(document, chunk.Sectors) {
			return fmt.Errorf("catalog chunk %d: %w", i, ErrChunk)
		}
		if document.SolidRenderers != nil {
			renderTriangles += chunk.MeshBytes / 484
			if renderTriangles > MaxTriangles {
				return ErrChunk
			}
		}
		if _, _, err := worldlightmap.ReceiverScope(
			document,
			&worldlightmap.ReceiverSelection{Sectors: chunk.Sectors, Solids: chunk.Solids},
		); err != nil {
			return err
		}
		for _, sector := range chunk.Sectors {
			if sectorOwners[sector] >= 0 || catalog.SectorChunks[sector] != i {
				return ErrChunk
			}
			sectorOwners[sector] = i
		}
		for _, solid := range chunk.Solids {
			if solidSeen[solid] {
				return ErrChunk
			}
			solidSeen[solid] = true
		}
	}
	for _, owner := range sectorOwners {
		if owner < 0 {
			return ErrChunk
		}
	}
	for _, seen := range solidSeen {
		if !seen {
			return ErrChunk
		}
	}
	expected := reachableCandidates(document, sectorOwners, len(catalog.Chunks))
	for i := range expected {
		if !slices.Equal(expected[i], catalog.Candidates[i]) {
			return fmt.Errorf("incomplete conservative candidates: %w", ErrChunk)
		}
	}
	return nil
}

func ValidateMesh(mesh *Mesh, document *world.Document) error {
	var work cooperativeWork
	if err := world.Validate(document); err != nil {
		return err
	}
	digest, err := GeometryDigest(document)
	if err != nil {
		return err
	}
	surfaceDigest, err := meshSurfaceDigest(document)
	if err != nil {
		return err
	}
	solidCount := 0
	if document.StaticSolids != nil {
		solidCount = len(document.StaticSolids.Items)
	}
	if mesh == nil || !validMeshVersion(mesh) || mesh.GeometrySHA256 != digest || mesh.SurfaceSHA256 != surfaceDigest ||
		!indexes(mesh.Sectors, len(document.Sectors), true) ||
		!indexes(mesh.Solids, solidCount, false) ||
		!connected(document, mesh.Sectors) ||
		len(mesh.Vertices) == 0 ||
		len(mesh.Vertices) > MaxVertices ||
		len(mesh.Triangles) > MaxTriangles ||
		len(mesh.Vertices) != 3*len(mesh.Triangles) ||
		len(mesh.Indices) != len(mesh.Vertices) ||
		!validBounds(mesh.Bounds) {
		return ErrChunk
	}
	if _, _, err := worldlightmap.ReceiverScope(
		document,
		&worldlightmap.ReceiverSelection{Sectors: mesh.Sectors, Solids: mesh.Solids},
	); err != nil {
		return err
	}
	bounds := [6]float64{math.Inf(1), math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1), math.Inf(-1)}
	for i, v := range mesh.Vertices {
		work.checkpoint(1024)
		if mesh.Indices[i] != uint32(i) || !finiteVertex(v) {
			return fmt.Errorf("vertex %d: %w", i, ErrChunk)
		}
		for axis, value := range v.Position {
			if math.Abs(float64(value)) > world.MaxCoordinate {
				return ErrChunk
			}
			bounds[axis] = math.Min(bounds[axis], float64(value))
			bounds[axis+3] = math.Max(bounds[axis+3], float64(value))
		}
	}
	if bounds != mesh.Bounds {
		return fmt.Errorf("mesh bounds: %w", ErrChunk)
	}
	areas := make(map[worldlightmap.Binding]float64)
	surfaces, err := worldlightmap.Surfaces(document)
	if err != nil {
		return err
	}
	if mesh.Selection != nil {
		if err := worldlightmap.ValidateSurfaceSelection(
			mesh.Selection,
			worldlightmap.LayoutAdmission{SectorCount: len(document.Sectors), SolidCount: solidCount},
		); err != nil {
			return err
		}
	}
	coverage := make(map[worldlightmap.Binding]*meshCoverage)
	for _, surface := range surfaces {
		binding := surface.Binding
		if mesh.Selection != nil && !worldlightmap.SurfaceSelected(binding, mesh.Selection) {
			continue
		}
		owners := mesh.Solids
		if strings.HasPrefix(binding.Kind, "sector-") {
			owners = mesh.Sectors
		}
		if _, included := slices.BinarySearch(owners, binding.Index); included {
			receiver := newMeshCoverage(document, surface)
			if mesh.Selection != nil && mesh.Selection.Clip != nil {
				clipped := make([][]world.Vec3, 0, len(receiver.polygons))
				for _, polygon := range receiver.polygons {
					if p := worldlightmap.ClipSurfacePolygon(polygon, *mesh.Selection.Clip); len(p) >= 3 {
						clipped = append(clipped, p)
					}
				}
				receiver.polygons = clipped
			}
			coverage[binding] = receiver
		}
	}
	liquids := displacedLiquidMaterials(document)
	for i, t := range mesh.Triangles {
		work.checkpoint(1024)
		binding, err := triangleBinding(t, mesh, document, mesh.Vertices[i*3:i*3+3])
		if err != nil {
			return fmt.Errorf("triangle %d: %w", i, err)
		}
		material, mapping, secondary := surfaceMaterial(document, binding)
		mappingCount := uint8(0)
		if mapping != nil {
			mappingCount = uint8(len(mapping.Projections))
		}
		if t.Material != material || t.MappingCount != mappingCount || t.Chart < -1 || t.Chart >= worldlightmap.MaxCharts ||
			!finiteTriangle(t) ||
			t.Sky && t.Chart != -1 ||
			math.Abs(dot(unpacked(t.Normal), unpacked(t.Normal))-1) > 1e-5 ||
			t.Band.Coverage > 2 ||
			t.Band.Coverage > 0 && t.Band.Material == 0 {
			return ErrChunk
		}
		if dot(unpacked(t.Normal), semanticNormal(document, binding)) < 1-1e-5 {
			return ErrChunk
		}
		weights := [3]float32{1, 0, 0}
		if mapping != nil {
			for i, weight := range mapping.Weights {
				weights[i] = float32(weight)
			}
		}
		if t.Weights != weights {
			return ErrChunk
		}
		if t.Band.Coverage > 0 {
			if binding.Kind != "sector-wall" {
				return ErrChunk
			}
			found := false
			for _, region := range document.Sectors[binding.Index].Walls[binding.Edge].FrameRegions {
				found = found || region.Material == t.Band.Material && region.Coverage != world.FrameCoverageMain
			}
			if !found {
				return ErrChunk
			}
		}
		if secondary == nil || secondary.Strength == 0 {
			if t.Secondary != (Secondary{}) {
				return ErrChunk
			}
		} else {
			if t.Secondary.Material != secondary.Material || t.Secondary.Strength != float32(secondary.Strength) ||
				t.Secondary.MappingCount != uint8(len(secondary.UV.Projections)) {
				return ErrChunk
			}
		}
		vertices := mesh.Vertices[i*3 : i*3+3]
		receiver := coverage[binding]
		if receiver == nil {
			return ErrChunk
		}
		if err := receiver.addTriangle(vertices); err != nil {
			return fmt.Errorf("triangle %d: %w", i, err)
		}
		if err := receiver.validateAttributes(t, vertices); err != nil {
			return fmt.Errorf("triangle %d authored attributes: %w", i, err)
		}
		if horizontalLiquid(unpacked(t.Normal), t.Material, liquids) {
			if err := validateLiquidVertices(document, t, vertices, mapping); err != nil {
				return fmt.Errorf("liquid triangle %d: %w", i, err)
			}
		}
		a, b, c := unpacked(vertices[0].Position), unpacked(vertices[1].Position), unpacked(vertices[2].Position)
		area := dot(cross(sub(b, a), sub(c, a)), unpacked(t.Normal)) * .5
		if area <= 0 {
			return fmt.Errorf(
				"triangle %d (%s %d edge %d) is degenerate or reversed at %v: %w",
				i,
				binding.Kind,
				binding.Index,
				binding.Edge,
				[3]world.Vec3{a, b, c},
				ErrChunk,
			)
		}
		areas[binding] += area
	}
	// Compare complete receiver coverage without triangulating anything. This
	// catches truncated final batches and missing surfaces before GPU allocation.
	for _, surface := range surfaces {
		work.checkpoint(1024)
		binding := surface.Binding
		binding.Chart = -1
		if mesh.Selection != nil && !worldlightmap.SurfaceSelected(binding, mesh.Selection) {
			continue
		}
		owners := mesh.Solids
		if strings.HasPrefix(binding.Kind, "sector-") {
			owners = mesh.Sectors
		}
		if _, included := slices.BinarySearch(owners, binding.Index); !included {
			continue
		}
		polygons := coverage[binding].polygons
		expected := 0.0
		for _, polygon := range polygons {
			sum := world.Vec3{}
			for i, p := range polygon {
				sum = add(sum, cross(p, polygon[(i+1)%len(polygon)]))
			}
			expected += math.Abs(dot(sum, surface.Normal)) * .5
		}
		if math.Abs(areas[binding]-expected) > max(1, expected)*1e-4 {
			return fmt.Errorf("incomplete %s %d/%d: %w", binding.Kind, binding.Index, binding.Edge, ErrChunk)
		}
		if err := coverage[binding].validateBoundary(&work); err != nil {
			return fmt.Errorf("receiver %s %d/%d: %w", binding.Kind, binding.Index, binding.Edge, err)
		}
	}
	return nil
}

func finiteVertex(v Vertex) bool {
	for _, x := range v.Position {
		if !finite(float64(x)) {
			return false
		}
	}
	for _, p := range v.UV {
		for _, x := range p {
			if !finite(float64(x)) {
				return false
			}
		}
	}
	for _, p := range v.SecondaryUV {
		for _, x := range p {
			if !finite(float64(x)) {
				return false
			}
		}
	}
	for _, p := range [3][2]float32{v.BandUV, v.LightmapUV, v.WaveUV} {
		for _, x := range p {
			if !finite(float64(x)) {
				return false
			}
		}
	}
	return true
}

func finiteTriangle(t Triangle) bool {
	for _, p := range [3][3]float32{t.Normal, t.Tangent, t.Bitangent} {
		for _, x := range p {
			if !finite(float64(x)) {
				return false
			}
		}
	}
	for _, basis := range append(t.ProjectionBases[:], t.Band.Basis) {
		for _, p := range [2][3]float32{basis.Tangent, basis.Bitangent} {
			for _, x := range p {
				if !finite(float64(x)) {
					return false
				}
			}
		}
	}
	for _, weights := range [2][3]float32{t.Weights, t.Secondary.Weights} {
		for _, x := range weights {
			if !finite(float64(x)) || x < 0 || x > 1 {
				return false
			}
		}
	}
	return finite(float64(t.Secondary.Strength)) && t.Secondary.Strength >= 0 && t.Secondary.Strength <= 1
}

func finite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }

func indexes(values []int, count int, nonempty bool) bool {
	if nonempty && len(values) == 0 {
		return false
	}
	for i, v := range values {
		if v < 0 || v >= count || i > 0 && v <= values[i-1] {
			return false
		}
	}
	return true
}

func hash(s string) bool {
	decoded, err := hex.DecodeString(s)
	return err == nil && len(decoded) == 32 && hex.EncodeToString(decoded) == s
}

func validBounds(bounds [6]float64) bool {
	for axis := range 3 {
		if !finite(bounds[axis]) || !finite(bounds[axis+3]) || bounds[axis] > bounds[axis+3] ||
			math.Abs(bounds[axis]) > world.MaxCoordinate ||
			math.Abs(bounds[axis+3]) > world.MaxCoordinate {
			return false
		}
	}
	return true
}

func connected(document *world.Document, sectors []int) bool {
	if len(sectors) == 0 {
		return false
	}
	// A receiver page may hold nonadjacent sectors whose intervening ordinary
	// sectors belong to another page. Its selected sectors must still share the
	// same complete physical component; transformed links never join it.
	seen := make([]bool, len(document.Sectors))
	seen[sectors[0]] = true
	queue := make([]int, 1, len(document.Sectors))
	queue[0] = sectors[0]
	for next := 0; next < len(queue); next++ {
		sector := queue[next]
		for edge, wall := range document.Sectors[sector].Walls {
			if wall.Portal < 0 || seen[int(wall.Portal)] || !ordinary(document, sector, edge) {
				continue
			}
			seen[int(wall.Portal)] = true
			queue = append(queue, int(wall.Portal))
		}
	}
	for _, sector := range sectors {
		if !seen[sector] {
			return false
		}
	}
	return true
}
