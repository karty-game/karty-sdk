package worldchunk

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"slices"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
	"github.com/karty-game/karty-sdk/internal/binwire"
)

// These constants match the authored two-wave vertex displacement. The rest
// mesh remains the collision and bake receiver; only its sampling is refined.
const (
	liquidInterpolationError   = .005
	liquidMaxSubdivisions      = 32
	liquidPatchTriangleTarget  = 256
	liquidPatchTriangleMaximum = 1024
	liquidSmallWaveAmplitude   = .05
	liquidFirstWaveV           = .37
	liquidSecondWaveU          = -.53
	liquidSecondWaveWeight     = .5
	liquidInterpolationDivisor = 8
	liquidTessellationIdentity = "karty.world-chunk-liquid-tessellation@1"
)

type liquidMaterial struct {
	Material  uint32
	Frequency float64
	Amplitude float64
}

func displacedLiquidMaterials(document *world.Document) map[uint32]liquidMaterial {
	materials := make(map[uint32]liquidMaterial)
	if document.Animations == nil {
		return materials
	}
	for _, binding := range document.Animations.Materials {
		for _, preset := range document.Animations.Presets {
			if preset.Name == binding.Preset && preset.Kind == "liquid" && preset.SurfaceAmplitude > 0 {
				materials[binding.Material] = liquidMaterial{binding.Material, preset.Frequency, preset.SurfaceAmplitude}
			}
		}
	}
	return materials
}

// Preserve the ordinary offline surface digest when no displacement is bound.
// The chunk-specific domain also binds every effective tessellation input;
// timing, shading waves and opacity can change without invalidating rest data.
func meshSurfaceDigest(document *world.Document) (string, error) {
	base, err := worldlightmap.OfflineSurfaceDigest(document)
	if err != nil {
		return "", err
	}
	materials := displacedLiquidMaterials(document)
	if len(materials) == 0 {
		return base, nil
	}
	inputs := make([]liquidMaterial, 0, len(materials))
	for _, material := range materials {
		inputs = append(inputs, material)
	}
	slices.SortFunc(inputs, func(a, b liquidMaterial) int { return compare(float64(a.Material), float64(b.Material)) })
	w := binwire.NewWriter("KLQ1", MaxEncodedSize)
	w.String(liquidTessellationIdentity, 128)
	w.String(base, 64)
	w.Count(len(inputs), false, world.MaxMaterialAnimations)
	for _, input := range inputs {
		w.U32(input.Material)
		w.F64(input.Frequency)
		w.F64(input.Amplitude)
	}
	encoded, err := w.Bytes()
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func horizontalLiquid(normal world.Vec3, material uint32, liquids map[uint32]liquidMaterial) bool {
	_, bound := liquids[material]
	return bound && math.Abs(normal.X) < 1e-9 && math.Abs(normal.Y) < 1e-9 && normal.Z > .999999
}

// Validate the final rest samples directly against the authored planes. This
// needs no receiver triangulation or host-side subdivision reconstruction.
func validateLiquidVertices(document *world.Document, triangle Triangle, vertices []Vertex, mapping *world.SurfaceUV) error {
	plane := world.Plane{}
	switch triangle.Surface {
	case "sector-floor":
		plane = document.Sectors[triangle.Sector].Floor
	case "solid-top":
		plane = document.StaticSolids.Items[triangle.Solid].Top
	default:
		return ErrChunk
	}
	if mapping == nil {
		mapping = legacyMapping(document, triangle)
	}
	projection := mapping.Projections[0]
	for _, vertex := range vertices {
		position := unpacked(vertex.Position)
		height := plane.A*position.X + plane.B*position.Y + plane.C
		if math.Abs(position.Z-height) > max(1, math.Abs(height))*2e-6 {
			return ErrChunk
		}
		want := [2]float64{evaluate(projection.U, position), evaluate(projection.V, position)}
		for axis := range 2 {
			if math.Abs(float64(vertex.WaveUV[axis])-want[axis]) > max(1, math.Abs(want[axis]))*2e-6 {
				return ErrChunk
			}
		}
	}
	return nil
}

type liquidEdge [2][3]float32

type liquidPatch struct {
	parent, factor, budget, count int
}

func liquidPositionBefore(a, b [3]float32) bool {
	return a[0] < b[0] || a[0] == b[0] && (a[1] < b[1] || a[1] == b[1] && a[2] < b[2])
}

func liquidRoot(patches []liquidPatch, index int) int {
	for patches[index].parent != index {
		parent := patches[index].parent
		patches[index].parent = patches[parent].parent
		index = patches[index].parent
	}
	return index
}

func liquidTriangleFactor(vertices []Vertex, material liquidMaterial) int {
	firstSpan, secondSpan := 0.0, 0.0
	for corner, vertex := range vertices {
		other := vertices[(corner+1)%3].WaveUV
		u, v := float64(vertex.WaveUV[0])-float64(other[0]), float64(vertex.WaveUV[1])-float64(other[1])
		first, second := u+liquidFirstWaveV*v, liquidSecondWaveU*u+v
		firstSpan = max(firstSpan, first*first)
		secondSpan = max(secondSpan, second*second)
	}
	phase := 2 * math.Pi * material.Frequency
	errorBound := material.Amplitude * phase * phase * (firstSpan + liquidSecondWaveWeight*secondSpan) /
		((1 + liquidSecondWaveWeight) * liquidInterpolationDivisor)
	factor := math.Sqrt(errorBound / liquidInterpolationError)
	return max(1, int(math.Ceil(min(liquidMaxSubdivisions, factor))))
}

func liquidPatchBudget(material liquidMaterial) int {
	if material.Amplitude <= liquidSmallWaveAmplitude {
		return liquidPatchTriangleTarget
	}
	budget := liquidPatchTriangleTarget * material.Amplitude / liquidSmallWaveAmplitude * math.Sqrt(max(1, material.Frequency))
	return int(math.Ceil(min(liquidPatchTriangleMaximum, max(liquidPatchTriangleTarget, budget))))
}

func liquidFactors(mesh *Mesh, liquids map[uint32]liquidMaterial) []int {
	patches := make([]liquidPatch, len(mesh.Triangles))
	edges := make(map[liquidEdge]int)
	for index, triangle := range mesh.Triangles {
		patches[index] = liquidPatch{parent: index, factor: 1}
		if !horizontalLiquid(unpacked(triangle.Normal), triangle.Material, liquids) {
			continue
		}
		material := liquids[triangle.Material]
		vertices := mesh.Vertices[index*3 : index*3+3]
		patches[index].factor = liquidTriangleFactor(vertices, material)
		patches[index].budget = liquidPatchBudget(material)
		for corner, vertex := range vertices {
			a, b := vertex.Position, vertices[(corner+1)%3].Position
			if liquidPositionBefore(b, a) {
				a, b = b, a
			}
			edge := liquidEdge{a, b}
			if previous, found := edges[edge]; found {
				left, right := liquidRoot(patches, previous), liquidRoot(patches, index)
				patches[right].parent = left
				patches[left].factor = max(patches[left].factor, patches[right].factor)
				patches[left].budget = max(patches[left].budget, patches[right].budget)
			} else {
				edges[edge] = index
			}
		}
	}
	for index := range patches {
		root := liquidRoot(patches, index)
		patches[root].count++
	}
	for index := range patches {
		patch := &patches[index]
		if patch.parent == index && patch.budget > 0 {
			factor := max(1, int(math.Sqrt(float64(patch.budget)/float64(patch.count))))
			patch.factor = min(patch.factor, factor)
		}
	}
	factors := make([]int, len(patches))
	for index := range patches {
		factors[index] = patches[liquidRoot(patches, index)].factor
	}
	return factors
}

func meshOwnsTriangle(mesh *Mesh, triangle Triangle) bool {
	owners, owner := mesh.Sectors, triangle.Sector
	if triangle.Solid >= 0 {
		owners, owner = mesh.Solids, triangle.Solid
	}
	_, found := slices.BinarySearch(owners, owner)
	return found
}

func tessellateLiquids(source Mesh, liquids map[uint32]liquidMaterial) (Mesh, error) {
	if len(liquids) == 0 {
		return source, nil
	}
	factors := liquidFactors(&source, liquids)
	return tessellateLiquidFactors(source, factors)
}

// Factors can be prepared once from the complete physical patch and reused
// while emitting independent owners. The public compiler uses the same path.
func tessellateLiquidFactors(source Mesh, factors []int) (Mesh, error) {
	count := 0
	for index, triangle := range source.Triangles {
		if meshOwnsTriangle(&source, triangle) {
			count += factors[index] * factors[index]
			if count > MaxTriangles {
				return Mesh{}, fmt.Errorf("offline liquid triangle budget: %w", ErrChunk)
			}
		}
	}
	result := source
	result.Vertices = make([]Vertex, 0, count*3)
	result.Indices = make([]uint32, 0, count*3)
	result.Triangles = make([]Triangle, 0, count)
	result.Bounds = [6]float64{math.Inf(1), math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1), math.Inf(-1)}
	for index, triangle := range source.Triangles {
		if !meshOwnsTriangle(&source, triangle) {
			continue
		}
		vertices := source.Vertices[index*3 : index*3+3]
		factor := factors[index]
		if factor == 1 {
			appendLiquidTriangle(&result, triangle, [3]Vertex{vertices[0], vertices[1], vertices[2]})
			continue
		}
		for u := range factor {
			for v := range factor - u {
				emitLiquidTriangle(&result, triangle, vertices, factor, [3][2]int{{u, v}, {u + 1, v}, {u, v + 1}})
				if u+v+1 < factor {
					emitLiquidTriangle(&result, triangle, vertices, factor, [3][2]int{{u + 1, v}, {u + 1, v + 1}, {u, v + 1}})
				}
			}
		}
	}
	return result, nil
}

func emitLiquidTriangle(mesh *Mesh, triangle Triangle, source []Vertex, factor int, coordinates [3][2]int) {
	var vertices [3]Vertex
	for corner, coordinate := range coordinates {
		weights := [3]int{factor - coordinate[0] - coordinate[1], coordinate[0], coordinate[1]}
		vertex := &vertices[corner]
		for axis := range vertex.Position {
			vertex.Position[axis] = liquidScalar(
				[3]float32{source[0].Position[axis], source[1].Position[axis], source[2].Position[axis]},
				weights,
				factor,
			)
		}
		for axis := range 2 {
			for projection := range 3 {
				vertex.UV[projection][axis] = liquidScalar(
					[3]float32{source[0].UV[projection][axis], source[1].UV[projection][axis], source[2].UV[projection][axis]},
					weights,
					factor,
				)
				vertex.SecondaryUV[projection][axis] = liquidScalar(
					[3]float32{
						source[0].SecondaryUV[projection][axis],
						source[1].SecondaryUV[projection][axis],
						source[2].SecondaryUV[projection][axis],
					},
					weights,
					factor,
				)
			}
			vertex.BandUV[axis] = liquidScalar(
				[3]float32{source[0].BandUV[axis], source[1].BandUV[axis], source[2].BandUV[axis]},
				weights,
				factor,
			)
			vertex.LightmapUV[axis] = liquidScalar(
				[3]float32{source[0].LightmapUV[axis], source[1].LightmapUV[axis], source[2].LightmapUV[axis]},
				weights,
				factor,
			)
			vertex.WaveUV[axis] = liquidScalar(
				[3]float32{source[0].WaveUV[axis], source[1].WaveUV[axis], source[2].WaveUV[axis]},
				weights,
				factor,
			)
		}
		// Opposite triangle windings must evaluate a shared edge in the same
		// endpoint order, rather than rounding complementary barycentric sums.
		for zero, weight := range weights {
			if weight != 0 {
				continue
			}
			left, right := (zero+1)%3, (zero+2)%3
			if liquidPositionBefore(source[right].Position, source[left].Position) {
				left, right = right, left
			}
			amount := float64(weights[right]) / float64(factor)
			*vertex = liquidEdgeVertex(source[left], source[right], amount)
			break
		}
	}
	appendLiquidTriangle(mesh, triangle, vertices)
}

func liquidEdgeVertex(left, right Vertex, amount float64) Vertex {
	result := Vertex{}
	interpolate := func(a, b float32) float32 { return float32(float64(a)*(1-amount) + float64(b)*amount) }
	for axis := range result.Position {
		result.Position[axis] = interpolate(left.Position[axis], right.Position[axis])
	}
	for axis := range 2 {
		for projection := range 3 {
			result.UV[projection][axis] = interpolate(left.UV[projection][axis], right.UV[projection][axis])
			result.SecondaryUV[projection][axis] = interpolate(left.SecondaryUV[projection][axis], right.SecondaryUV[projection][axis])
		}
		result.BandUV[axis] = interpolate(left.BandUV[axis], right.BandUV[axis])
		result.LightmapUV[axis] = interpolate(left.LightmapUV[axis], right.LightmapUV[axis])
		result.WaveUV[axis] = interpolate(left.WaveUV[axis], right.WaveUV[axis])
	}
	return result
}

func liquidScalar(values [3]float32, weights [3]int, factor int) float32 {
	value := 0.0
	for corner, weight := range weights {
		value += float64(values[corner]) * (float64(weight) / float64(factor))
	}
	return float32(value)
}

func appendLiquidTriangle(mesh *Mesh, triangle Triangle, vertices [3]Vertex) {
	a, b, c := unpacked(vertices[0].Position), unpacked(vertices[1].Position), unpacked(vertices[2].Position)
	orientation := dot(cross(sub(b, a), sub(c, a)), unpacked(triangle.Normal))
	if orientation == 0 {
		return
	}
	if orientation < 0 {
		vertices[1], vertices[2] = vertices[2], vertices[1]
	}
	for _, vertex := range vertices {
		mesh.Indices = append(mesh.Indices, uint32(len(mesh.Vertices)))
		mesh.Vertices = append(mesh.Vertices, vertex)
		for axis, value := range vertex.Position {
			mesh.Bounds[axis] = math.Min(mesh.Bounds[axis], float64(value))
			mesh.Bounds[axis+3] = math.Max(mesh.Bounds[axis+3], float64(value))
		}
	}
	mesh.Triangles = append(mesh.Triangles, triangle)
}
