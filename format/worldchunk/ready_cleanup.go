package worldchunk

import (
	"fmt"
	"math"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

// ErrReadyCapacity identifies derived renderer geometry exceeding the existing
// triangle/scalar limits. Builders may repartition a multi-sector child.
var ErrReadyCapacity = fmt.Errorf("prepared geometry capacity exceeded: %w", ErrChunk)

type cleanupPlane struct {
	normal   world.Vec3
	offset   float64
	anchor   world.Vec3
	anchored bool
}
type cleanupVolume struct {
	solid    int
	planes   []cleanupPlane
	semantic []cleanupPlane
	bounds   [6]float64
}
type cleanupContact struct {
	plane   cleanupPlane
	stored  cleanupPlane
	polygon []world.Vec3
	bounds  [6]float64
}
type readyCleanup struct {
	volumes  []cleanupVolume
	contacts map[[4]float64][]cleanupContact
	unsafe   map[uint32]bool
}

// The canonical mesh remains the complete authored proof. This derived mesh
// belongs only to this Ready build; source ownership, materials and chart IDs
// survive every cut, and the oracle deterministically derives it again.
func cleanupReadyMesh(source *Mesh, document *world.Document, materials map[uint32]readyMaterial) (Mesh, error) {
	scope, solidScope, err := worldlightmap.ReceiverScope(
		document,
		&worldlightmap.ReceiverSelection{Sectors: source.Sectors, Solids: source.Solids},
	)
	if err != nil {
		// Source validation already succeeded. Ambiguous or disconnected physical
		// scopes remain supported; retaining their original surfaces is safe.
		return *source, nil //nolint:nilerr // Optional ambiguous cleanup retains fully validated canonical geometry.
	}
	cleanup := readyCleanup{unsafe: make(map[uint32]bool), contacts: make(map[[4]float64][]cleanupContact)}
	for id, material := range materials {
		cleanup.unsafe[id] = material.masked || material.liquid || material.transparent
	}
	if document.Animations != nil {
		for _, binding := range document.Animations.Materials {
			cleanup.unsafe[binding.Material] = true
		}
	}
	solidTriangles := make(map[int][]int)
	for index, triangle := range source.Triangles {
		if triangle.Solid >= 0 {
			solidTriangles[triangle.Solid] = append(solidTriangles[triangle.Solid], index)
		}
	}
	for _, index := range source.Solids {
		solid := document.StaticSolids.Items[index]
		if solid.Collision && solidScope[index] && !cleanup.unsafe[solid.TopMaterial] && !cleanup.unsafe[solid.BottomMaterial] &&
			!cleanup.unsafe[solid.SideMaterial] {
			cleanup.volumes = append(cleanup.volumes, cleanupSolidVolume(index, solid, source, solidTriangles[index]))
		}
	}
	for index, triangle := range source.Triangles {
		if triangle.Sector < 0 || !scope[triangle.Sector] || !cleanup.triangleSafe(triangle) {
			continue
		}
		if triangle.Surface == "sector-wall" {
			wall := document.Sectors[triangle.Sector].Walls[triangle.SurfaceEdge]
			if wall.FrameCompiled || len(wall.FrameRegions) != 0 {
				continue
			}
		}
		plane := cleanupTrianglePlane(document, triangle)
		polygon := make([]world.Vec3, 3)
		for corner := range 3 {
			polygon[corner] = unpacked(source.Vertices[index*3+corner].Position)
		}
		key := plane.canonical()
		cleanup.contacts[key] = append(
			cleanup.contacts[key],
			cleanupContact{
				plane:   plane,
				stored:  cleanupStoredPlane(polygon[0], polygon[1], polygon[2]),
				polygon: polygon,
				bounds:  cleanupBounds(polygon),
			},
		)
	}
	result := *source
	result.Vertices, result.Indices, result.Triangles = nil, nil, nil
	result.Bounds = [6]float64{math.Inf(1), math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1), math.Inf(-1)}
	for index, triangle := range source.Triangles {
		vertices := source.Vertices[index*3 : index*3+3]
		if !cleanup.triangleSafe(triangle) ||
			triangle.Solid >= 0 && (!solidScope[triangle.Solid] || cleanup.unsafeSolid(document, triangle.Solid)) {
			if err := cleanupAppend(&result, triangle, [3]Vertex{vertices[0], vertices[1], vertices[2]}); err != nil {
				return Mesh{}, err
			}
			continue
		}
		plane := cleanupTrianglePlane(document, triangle)
		polygon := make([]world.Vec3, 3)
		for corner := range 3 {
			polygon[corner] = unpacked(vertices[corner].Position)
		}
		pieces := [][]world.Vec3{polygon}
		changed := false
		for _, volume := range cleanup.volumes {
			if triangle.Solid == volume.solid || !cleanupBoundsTouch(cleanupBounds(polygon), volume.bounds) {
				continue
			}
			sameFacingWinner := false
			for _, boundary := range volume.semantic {
				if plane.same(boundary) && dot(plane.normal, boundary.normal) > 0 && (triangle.Solid < 0 || triangle.Solid < volume.solid) {
					sameFacingWinner = true
					break
				}
			}
			if sameFacingWinner {
				continue
			}
			var removed bool
			pieces, removed, err = cleanupSubtract(pieces, volume.planes, plane)
			if err != nil {
				return Mesh{}, err
			}
			changed = changed || removed
			if len(pieces) > MaxTriangles {
				return Mesh{}, ErrReadyCapacity
			}
		}
		if triangle.Solid >= 0 {
			for _, contact := range cleanup.contacts[plane.canonical()] {
				if !plane.same(contact.plane) || dot(plane.normal, contact.plane.normal) <= 0 ||
					!cleanupCoplanar(polygon, contact.stored) ||
					!cleanupBoundsTouch(cleanupBounds(polygon), contact.bounds) {
					continue
				}
				var removed bool
				pieces, removed, err = cleanupSubtract(pieces, cleanupPolygonPlanes(contact.polygon, contact.stored.normal), plane)
				if err != nil {
					return Mesh{}, err
				}
				changed = changed || removed
				if len(pieces) > MaxTriangles {
					return Mesh{}, ErrReadyCapacity
				}
			}
		}
		if !changed {
			if err := cleanupAppend(&result, triangle, [3]Vertex{vertices[0], vertices[1], vertices[2]}); err != nil {
				return Mesh{}, err
			}
			continue
		}
		startVertices, startTriangles := len(result.Vertices), len(result.Triangles)
		conservativeFallback := false
		for _, piece := range pieces {
			triangles, ok := cleanupTriangulate(piece, unpacked(triangle.Normal))
			if !ok {
				conservativeFallback = true
				break
			}
			for _, positions := range triangles {
				derived := [3]Vertex{}
				for corner, position := range positions {
					derived[corner] = cleanupInterpolate(vertices, position, plane)
				}
				if err := cleanupAppend(&result, triangle, derived); err != nil {
					return Mesh{}, err
				}
			}
		}
		if conservativeFallback {
			result.Vertices = result.Vertices[:startVertices]
			result.Indices = result.Indices[:startVertices]
			result.Triangles = result.Triangles[:startTriangles]
			if err := cleanupAppend(&result, triangle, [3]Vertex{vertices[0], vertices[1], vertices[2]}); err != nil {
				return Mesh{}, err
			}
		}
	}
	// The established Ready wire has no empty-child representation. Keeping the
	// original hidden geometry is safe and preserves support for filled cells.
	if len(result.Triangles) == 0 {
		return *source, nil
	}
	result.Bounds = [6]float64{math.Inf(1), math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1), math.Inf(-1)}
	for _, vertex := range result.Vertices {
		for axis, value := range vertex.Position {
			result.Bounds[axis] = min(result.Bounds[axis], float64(value))
			result.Bounds[axis+3] = max(result.Bounds[axis+3], float64(value))
		}
	}
	return result, nil
}

func (cleanup *readyCleanup) unsafeSolid(document *world.Document, index int) bool {
	solid := document.StaticSolids.Items[index]
	return cleanup.unsafe[solid.TopMaterial] || cleanup.unsafe[solid.BottomMaterial] || cleanup.unsafe[solid.SideMaterial]
}

func (cleanup *readyCleanup) triangleSafe(triangle Triangle) bool {
	return !triangle.Sky && !cleanup.unsafe[triangle.Material] &&
		(triangle.Secondary.Strength <= 0 || !cleanup.unsafe[triangle.Secondary.Material]) &&
		(triangle.Band.Coverage == 0 || !cleanup.unsafe[triangle.Band.Material])
}

func cleanupAppend(mesh *Mesh, triangle Triangle, vertices [3]Vertex) error {
	normal := unpacked(triangle.Normal)
	a, b, c := unpacked(vertices[0].Position), unpacked(vertices[1].Position), unpacked(vertices[2].Position)
	area := dot(cross(sub(b, a), sub(c, a)), normal)
	if area == 0 {
		return nil
	}
	if area < 0 {
		return fmt.Errorf("reversed cleanup triangle: %w", ErrChunk)
	}
	if len(mesh.Triangles) >= MaxTriangles {
		return ErrReadyCapacity
	}
	for _, vertex := range vertices {
		if !finiteVertex(vertex) {
			return ErrChunk
		}
		mesh.Indices = append(mesh.Indices, uint32(len(mesh.Vertices)))
		mesh.Vertices = append(mesh.Vertices, vertex)
		for axis, value := range vertex.Position {
			mesh.Bounds[axis] = min(mesh.Bounds[axis], float64(value))
			mesh.Bounds[axis+3] = max(mesh.Bounds[axis+3], float64(value))
		}
	}
	mesh.Triangles = append(mesh.Triangles, triangle)
	return nil
}

func cleanupBounds(polygon []world.Vec3) [6]float64 {
	bounds := [6]float64{math.Inf(1), math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1), math.Inf(-1)}
	for _, point := range polygon {
		for axis, value := range [3]float64{point.X, point.Y, point.Z} {
			bounds[axis] = min(bounds[axis], value)
			bounds[axis+3] = max(bounds[axis+3], value)
		}
	}
	return bounds
}

func cleanupBoundsTouch(left, right [6]float64) bool {
	for axis := range 3 {
		if left[axis] > right[axis+3] || left[axis+3] < right[axis] {
			return false
		}
	}
	return true
}

func cleanupSolidVolume(index int, solid world.Solid, mesh *Mesh, triangles []int) cleanupVolume {
	volume := cleanupVolume{solid: index}
	volume.semantic = append(volume.semantic, cleanupCapPlane(solid.Top, false), cleanupCapPlane(solid.Bottom, true))
	for edge, a := range solid.Footprint {
		b := solid.Footprint[(edge+1)%len(solid.Footprint)]
		normal := world.Vec3{X: b.Y - a.Y, Y: a.X - b.X}
		volume.semantic = append(volume.semantic, cleanupPlane{normal: normal, offset: -normal.X*a.X - normal.Y*a.Y})
	}
	points := make([]world.Vec3, 0, len(triangles)*3)
	// Intersect every actual packed outward facet's interior. Quantized sloping
	// caps need not remain planar; this conservative convex core cannot extend
	// beyond their rendered shell. Authored planes only decide duplicate priority.
	for _, triangle := range triangles {
		a, b, c := unpacked(
			mesh.Vertices[triangle*3].Position,
		), unpacked(
			mesh.Vertices[triangle*3+1].Position,
		), unpacked(
			mesh.Vertices[triangle*3+2].Position,
		)
		volume.planes = append(volume.planes, cleanupStoredPlane(a, b, c))
		points = append(points, a, b, c)
	}
	volume.bounds = cleanupBounds(points)
	return volume
}

func cleanupStoredPlane(a, b, c world.Vec3) cleanupPlane {
	normal := cross(sub(b, a), sub(c, a))
	return cleanupPlane{normal: normal, offset: -dot(normal, a), anchor: a, anchored: true}
}

func (plane cleanupPlane) distance(point world.Vec3) float64 {
	if plane.anchored {
		return dot(plane.normal, sub(point, plane.anchor))
	}
	return dot(plane.normal, point) + plane.offset
}

func cleanupCoplanar(points []world.Vec3, plane cleanupPlane) bool {
	for _, point := range points {
		if plane.distance(point) != 0 {
			return false
		}
	}
	return true
}

func cleanupTrianglePlane(document *world.Document, triangle Triangle) cleanupPlane {
	switch triangle.Surface {
	case "sector-floor":
		return cleanupCapPlane(document.Sectors[triangle.Sector].Floor, false)
	case "sector-ceiling":
		return cleanupCapPlane(document.Sectors[triangle.Sector].Ceiling, true)
	case "solid-top":
		return cleanupCapPlane(document.StaticSolids.Items[triangle.Solid].Top, false)
	case "solid-bottom":
		return cleanupCapPlane(document.StaticSolids.Items[triangle.Solid].Bottom, true)
	case "sector-wall":
		wall := document.Sectors[triangle.Sector].Walls[triangle.SurfaceEdge]
		normal := world.Vec3{X: wall.Start.Y - wall.End.Y, Y: wall.End.X - wall.Start.X}
		return cleanupPlane{normal: normal, offset: -normal.X*wall.Start.X - normal.Y*wall.Start.Y}
	default:
		solid := document.StaticSolids.Items[triangle.Solid]
		a, b := solid.Footprint[triangle.SurfaceEdge], solid.Footprint[(triangle.SurfaceEdge+1)%len(solid.Footprint)]
		normal := world.Vec3{X: b.Y - a.Y, Y: a.X - b.X}
		return cleanupPlane{normal: normal, offset: -normal.X*a.X - normal.Y*a.Y}
	}
}

func cleanupCapPlane(plane world.Plane, reverse bool) cleanupPlane {
	result := cleanupPlane{normal: world.Vec3{X: -plane.A, Y: -plane.B, Z: 1}, offset: -plane.C}
	if reverse {
		result.normal = scale(result.normal, -1)
		result.offset = -result.offset
	}
	return result
}

func (plane cleanupPlane) canonical() [4]float64 {
	values := [4]float64{plane.normal.X, plane.normal.Y, plane.normal.Z, plane.offset}
	axis := 0
	for index := 1; index < 3; index++ {
		if math.Abs(values[index]) > math.Abs(values[axis]) {
			axis = index
		}
	}
	divisor := values[axis]
	for index := range values {
		values[index] /= divisor
	}
	return values
}

func (plane cleanupPlane) same(other cleanupPlane) bool {
	return plane.canonical() == other.canonical()
}

func cleanupPolygonPlanes(polygon []world.Vec3, normal world.Vec3) []cleanupPlane {
	planes := make([]cleanupPlane, 0, len(polygon))
	for index, a := range polygon {
		b := polygon[(index+1)%len(polygon)]
		outward := cross(sub(b, a), normal)
		planes = append(planes, cleanupPlane{normal: outward, offset: -dot(outward, a), anchor: a, anchored: true})
	}
	return planes
}
