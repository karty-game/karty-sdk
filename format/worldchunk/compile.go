package worldchunk

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

func GeometryDigest(document *world.Document) (string, error) {
	return worldlightmap.GeometryDigest(document)
}

// CompileMesh triangulates immutable authored receiver polygons offline. Every
// boundary vertex, including collinear junctions, gets its own triangle edge.
// The runtime reads final arrays and never constructs or tessellates a face.
func CompileMesh(document world.Document, sectors, solids []int, layout worldlightmap.Layout) (Mesh, error) {
	if err := world.Validate(&document); err != nil {
		return Mesh{}, err
	}
	if layout.Schema != "" {
		if err := worldlightmap.Validate(&layout, &document); err != nil {
			return Mesh{}, err
		}
	} else {
		digest, err := GeometryDigest(&document)
		if err != nil {
			return Mesh{}, err
		}
		layout = worldlightmap.Layout{GeometrySHA256: digest, Receivers: &worldlightmap.ReceiverSelection{Sectors: sectors, Solids: solids}}
		surfaces, err := worldlightmap.Surfaces(&document)
		if err != nil {
			return Mesh{}, err
		}
		for _, surface := range surfaces {
			binding := surface.Binding
			binding.Chart = -1
			layout.Bindings = append(layout.Bindings, binding)
		}
	}
	if layout.Receivers == nil || !slices.Equal(sectors, layout.Receivers.Sectors) || !slices.Equal(solids, layout.Receivers.Solids) {
		return Mesh{}, ErrChunk
	}
	liquids := displacedLiquidMaterials(&document)
	surfaces, err := worldlightmap.Surfaces(&document)
	if err != nil {
		return Mesh{}, err
	}
	component, componentSolids, err := worldlightmap.ReceiverScope(&document, layout.Receivers)
	if err != nil {
		return Mesh{}, err
	}
	junctions := componentJunctions(&document, surfaces, component, componentSolids)
	surfaceDigest, err := meshSurfaceDigest(&document)
	if err != nil {
		return Mesh{}, err
	}
	mesh := Mesh{
		Schema:         MeshSchema,
		GeometrySHA256: layout.GeometrySHA256,
		SurfaceSHA256:  surfaceDigest,
		Sectors:        slices.Clone(sectors),
		Solids:         slices.Clone(solids),
	}
	mesh.Bounds = [6]float64{math.Inf(1), math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1), math.Inf(-1)}
	for si, surface := range surfaces {
		binding := surface.Binding
		owner := solids
		if strings.HasPrefix(binding.Kind, "sector-") {
			owner = sectors
		}
		material, _, _ := surfaceMaterial(&document, binding)
		if _, included := slices.BinarySearch(owner, binding.Index); !included {
			physical := componentSolids
			if strings.HasPrefix(binding.Kind, "sector-") {
				physical = component
			}
			// The complete physical patch defines subdivision factors, including
			// neighboring liquid faces that belong to another render chunk.
			if !physical[binding.Index] || !horizontalLiquid(surface.Normal, material, liquids) {
				continue
			}
		}
		if err := emitSurface(&mesh, &document, layout, junctions, surface, layout.Bindings[si].Chart); err != nil {
			return Mesh{}, err
		}
	}
	if len(mesh.Vertices) == 0 {
		return Mesh{}, fmt.Errorf("empty render chunk: %w", ErrChunk)
	}
	return tessellateLiquids(mesh, liquids)
}

// componentJunctions includes physical neighbors so separately emitted chunks
// retain identical authored boundary subdivisions.
func componentJunctions(document *world.Document, surfaces []worldlightmap.Surface, component, componentSolids []bool) pointIndex {
	points := make([]world.Vec3, 0)
	for _, surface := range surfaces {
		included := false
		if strings.HasPrefix(surface.Binding.Kind, "sector-") {
			included = component[surface.Binding.Index]
		} else {
			included = componentSolids[surface.Binding.Index]
		}
		if !included {
			continue
		}
		for _, polygon := range surface.Polygons {
			points = append(points, polygon...)
		}
	}
	for i, included := range component {
		if !included {
			continue
		}
		sector := document.Sectors[i]
		if sector.Sky {
			points = append(points, capPolygon(sector, true)...)
			for _, wall := range sector.Walls {
				if wall.Sky {
					points = append(
						points,
						at(wall.Start, sector.Floor),
						at(wall.End, sector.Floor),
						at(wall.Start, sector.Ceiling),
						at(wall.End, sector.Ceiling),
					)
				}
			}
		}
		for _, wall := range sector.Walls {
			for _, region := range wall.FrameRegions {
				for _, p := range region.Vertices {
					points = append(
						points,
						world.Vec3{
							X: wall.Start.X + (wall.End.X-wall.Start.X)*p.X,
							Y: wall.Start.Y + (wall.End.Y-wall.Start.Y)*p.X,
							Z: p.Y,
						},
					)
				}
			}
		}
	}
	return newPointIndex(points)
}

func emitSurface(
	mesh *Mesh,
	document *world.Document,
	layout worldlightmap.Layout,
	junctions pointIndex,
	surface worldlightmap.Surface,
	chart int,
) error {
	binding := surface.Binding
	material, mapping, secondary := surfaceMaterial(document, binding)
	template := Triangle{
		Sector:      -1,
		Solid:       -1,
		Surface:     binding.Kind,
		SurfaceEdge: binding.Edge,
		Normal:      packed(surface.Normal),
		Material:    material,
		Chart:       chart,
	}
	if strings.HasPrefix(binding.Kind, "sector-") {
		template.Sector = binding.Index
		template.Outdoor = document.Sectors[binding.Index].Sky
	} else {
		template.Solid = binding.Index
	}
	polygons := surface.Polygons
	if binding.Kind == "sector-ceiling" && document.Sectors[binding.Index].Sky {
		template.Sky = true
		polygons = [][]world.Vec3{capPolygon(document.Sectors[binding.Index], true)}
	}
	if binding.Kind == "sector-wall" {
		sector := document.Sectors[binding.Index]
		wall := sector.Walls[binding.Edge]
		if wall.Sky {
			template.Sky = true
			polygons = [][]world.Vec3{
				{
					at(wall.Start, sector.Floor),
					at(wall.End, sector.Floor),
					at(wall.End, sector.Ceiling),
					at(wall.Start, sector.Ceiling),
				},
			}
		}
		if len(wall.FrameRegions) > 0 {
			for _, region := range wall.FrameRegions {
				polygon := make([]world.Vec3, len(region.Vertices))
				for i, p := range region.Vertices {
					polygon[i] = world.Vec3{
						X: wall.Start.X + (wall.End.X-wall.Start.X)*p.X,
						Y: wall.Start.Y + (wall.End.Y-wall.Start.Y)*p.X,
						Z: p.Y,
					}
				}
				frame := template
				if region.Coverage != world.FrameCoverageMain {
					frame.Band = Band{Material: region.Material, Coverage: 1, RepeatU: region.RepeatU, RepeatV: region.RepeatV}
					if region.Coverage == world.FrameCoverageOpaque {
						frame.Band.Coverage = 2
					}
				}
				if err := emitPolygon(mesh, document, layout, junctions, polygon, frame, mapping, secondary, region.UV); err != nil {
					return err
				}
			}
			return nil
		}
	}
	for _, polygon := range polygons {
		if err := emitPolygon(mesh, document, layout, junctions, polygon, template, mapping, secondary, nil); err != nil {
			return err
		}
	}
	return nil
}

func emitPolygon(
	mesh *Mesh,
	document *world.Document,
	layout worldlightmap.Layout,
	junctions pointIndex,
	polygon []world.Vec3,
	template Triangle,
	mapping *world.SurfaceUV,
	secondary *world.SurfaceSecondary,
	band *world.SurfaceUV,
) error {
	polygon = junctions.split(polygon)
	if len(polygon) < 3 {
		return nil
	}
	if mapping == nil {
		mapping = legacyMapping(document, template)
	} else {
		template.MappingCount = uint8(len(mapping.Projections))
	}
	for i, weight := range mapping.Weights {
		template.Weights[i] = float32(weight)
	}
	for i, projection := range mapping.Projections {
		template.ProjectionBases[i] = projectionBasis(projection, unpacked(template.Normal))
	}
	template.Tangent, template.Bitangent = template.ProjectionBases[0].Tangent, template.ProjectionBases[0].Bitangent
	if secondary != nil && secondary.Strength > 0 {
		template.Secondary = Secondary{
			Material:     secondary.Material,
			Strength:     float32(secondary.Strength),
			MappingCount: uint8(len(secondary.UV.Projections)),
		}
		for i, weight := range secondary.UV.Weights {
			template.Secondary.Weights[i] = float32(weight)
		}
	}
	if band != nil {
		template.Band.Basis = projectionBasis(band.Projections[0], unpacked(template.Normal))
	}
	center := world.Vec3{}
	for _, p := range polygon {
		center = add(center, scale(p, 1/float64(len(polygon))))
	}
	for i, a := range polygon {
		b := polygon[(i+1)%len(polygon)]
		// Junctions discovered in float64 can differ while producing identical
		// float32 vertices. Winding and degeneracy must use the stored geometry.
		storedCenter, storedA, storedB := unpacked(packed(center)), unpacked(packed(a)), unpacked(packed(b))
		orientation := dot(cross(sub(storedA, storedCenter), sub(storedB, storedCenter)), unpacked(template.Normal))
		if orientation == 0 {
			continue
		}
		if orientation < 0 {
			a, b = b, a
		}
		if len(mesh.Triangles) >= MaxTriangles {
			return ErrChunk
		}
		triangle := template
		positions := [3]world.Vec3{center, a, b}
		for _, p := range positions {
			vertex := Vertex{Position: packed(p)}
			for j, projection := range mapping.Projections {
				vertex.UV[j] = uvAt(projection, p, polygon[0], true, true)
			}
			vertex.WaveUV = [2]float32{float32(evaluate(mapping.Projections[0].U, p)), float32(evaluate(mapping.Projections[0].V, p))}
			if template.Secondary.MappingCount > 0 {
				for j, projection := range secondary.UV.Projections {
					vertex.SecondaryUV[j] = uvAt(projection, p, polygon[0], true, true)
				}
			}
			if band != nil {
				vertex.BandUV = uvAt(band.Projections[0], p, polygon[0], template.Band.RepeatU, template.Band.RepeatV)
			}
			if triangle.Chart >= 0 {
				chart := layout.Charts[triangle.Chart]
				vertex.LightmapUV = [2]float32{float32(affine(chart.UPlane, p)), float32(affine(chart.VPlane, p))}
			}
			mesh.Indices = append(mesh.Indices, uint32(len(mesh.Vertices)))
			mesh.Vertices = append(mesh.Vertices, vertex)
			for axis, value := range vertex.Position {
				mesh.Bounds[axis] = math.Min(mesh.Bounds[axis], float64(value))
				mesh.Bounds[axis+3] = math.Max(mesh.Bounds[axis+3], float64(value))
			}
		}
		mesh.Triangles = append(mesh.Triangles, triangle)
	}
	return nil
}

func surfaceMaterial(document *world.Document, b worldlightmap.Binding) (uint32, *world.SurfaceUV, *world.SurfaceSecondary) {
	switch b.Kind {
	case "sector-floor":
		s := document.Sectors[b.Index]
		return s.FloorMaterial, s.FloorUV, s.FloorSecondary
	case "sector-ceiling":
		s := document.Sectors[b.Index]
		return s.CeilingMaterial, s.CeilingUV, s.CeilingSecondary
	case "sector-wall":
		w := document.Sectors[b.Index].Walls[b.Edge]
		return w.Material, w.UV, w.Secondary
	case "solid-top":
		s := document.StaticSolids.Items[b.Index]
		return s.TopMaterial, s.TopUV, nil
	case "solid-bottom":
		s := document.StaticSolids.Items[b.Index]
		return s.BottomMaterial, s.BottomUV, nil
	default:
		s := document.StaticSolids.Items[b.Index]
		return s.SideMaterial, s.SideUV, nil
	}
}

func legacyMapping(document *world.Document, t Triangle) *world.SurfaceUV {
	projection := world.UVProjection{U: world.UVPlane{X: 1}, V: world.UVPlane{Y: 1}}
	if t.Surface == "sector-wall" || t.Surface == "solid-side" {
		a, b := world.Vec2{}, world.Vec2{}
		if t.Sector >= 0 {
			wall := document.Sectors[t.Sector].Walls[t.SurfaceEdge]
			a, b = wall.Start, wall.End
		} else {
			solid := document.StaticSolids.Items[t.Solid]
			a, b = solid.Footprint[t.SurfaceEdge], solid.Footprint[(t.SurfaceEdge+1)%len(solid.Footprint)]
		}
		length := math.Hypot(b.X-a.X, b.Y-a.Y)
		x, y := (b.X-a.X)/length, (b.Y-a.Y)/length
		projection = world.UVProjection{U: world.UVPlane{X: x, Y: y, Offset: -x*a.X - y*a.Y}, V: world.UVPlane{Z: 1}}
	}
	return &world.SurfaceUV{Projections: []world.UVProjection{projection}, Weights: []float64{1}}
}

func projectionBasis(p world.UVProjection, n world.Vec3) Basis {
	u, v := world.Vec3{X: p.U.X, Y: p.U.Y, Z: p.U.Z}, world.Vec3{X: p.V.X, Y: p.V.Y, Z: p.V.Z}
	u, v = sub(u, scale(n, dot(u, n))), sub(v, scale(n, dot(v, n)))
	uu, vv, uv := dot(u, u), dot(v, v), dot(u, v)
	determinant := uu*vv - uv*uv
	if determinant <= 1e-20 {
		return Basis{}
	}
	t := unit(sub(scale(u, vv), scale(v, uv)))
	b := sub(scale(v, uu), scale(u, uv))
	b = unit(sub(b, scale(t, dot(t, b))))
	return Basis{Tangent: packed(t), Bitangent: packed(b)}
}

func uvAt(p world.UVProjection, v, origin world.Vec3, repeatU, repeatV bool) [2]float32 {
	u, w := evaluate(p.U, v), evaluate(p.V, v)
	if repeatU {
		u -= math.Floor(evaluate(p.U, origin))
	}
	if repeatV {
		w -= math.Floor(evaluate(p.V, origin))
	}
	return [2]float32{float32(u), float32(w)}
}
func evaluate(p world.UVPlane, v world.Vec3) float64 { return p.X*v.X + p.Y*v.Y + p.Z*v.Z + p.Offset }
func affine(p [4]float64, v world.Vec3) float64      { return p[0]*v.X + p[1]*v.Y + p[2]*v.Z + p[3] }
func capPolygon(s world.Sector, ceiling bool) []world.Vec3 {
	points := make([]world.Vec3, len(s.Walls))
	p := s.Floor
	if ceiling {
		p = s.Ceiling
	}
	for i, w := range s.Walls {
		points[i] = at(w.Start, p)
	}
	return points
}
func at(v world.Vec2, p world.Plane) world.Vec3 {
	return world.Vec3{X: v.X, Y: v.Y, Z: p.A*v.X + p.B*v.Y + p.C}
}
func packed(v world.Vec3) [3]float32 { return [3]float32{float32(v.X), float32(v.Y), float32(v.Z)} }
func unpacked(v [3]float32) world.Vec3 {
	return world.Vec3{X: float64(v[0]), Y: float64(v[1]), Z: float64(v[2])}
}
func sub(a, b world.Vec3) world.Vec3           { return world.Vec3{X: a.X - b.X, Y: a.Y - b.Y, Z: a.Z - b.Z} }
func add(a, b world.Vec3) world.Vec3           { return world.Vec3{X: a.X + b.X, Y: a.Y + b.Y, Z: a.Z + b.Z} }
func scale(a world.Vec3, s float64) world.Vec3 { return world.Vec3{X: a.X * s, Y: a.Y * s, Z: a.Z * s} }
func dot(a, b world.Vec3) float64              { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }
func cross(a, b world.Vec3) world.Vec3 {
	return world.Vec3{X: a.Y*b.Z - a.Z*b.Y, Y: a.Z*b.X - a.X*b.Z, Z: a.X*b.Y - a.Y*b.X}
}
func unit(v world.Vec3) world.Vec3 {
	length := math.Sqrt(dot(v, v))
	if length == 0 {
		return world.Vec3{}
	}
	return scale(v, 1/length)
}

type pointIndex [3][]world.Vec3

func coordinate(v world.Vec3, axis int) float64 {
	if axis == 0 {
		return v.X
	}
	if axis == 1 {
		return v.Y
	}
	return v.Z
}
func newPointIndex(points []world.Vec3) pointIndex {
	var result pointIndex
	for axis := range 3 {
		result[axis] = slices.Clone(points)
		slices.SortFunc(result[axis], func(a, b world.Vec3) int { return compare(coordinate(a, axis), coordinate(b, axis)) })
	}
	return result
}
func compare(a, b float64) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}
func (index pointIndex) split(polygon []world.Vec3) []world.Vec3 {
	result := make([]world.Vec3, 0, len(polygon))
	for i, a := range polygon {
		b := polygon[(i+1)%len(polygon)]
		delta := sub(b, a)
		length := dot(delta, delta)
		axis := 0
		for candidate := 1; candidate < 3; candidate++ {
			if math.Abs(coordinate(delta, candidate)) > math.Abs(coordinate(delta, axis)) {
				axis = candidate
			}
		}
		cuts := []float64{0}
		low, high := math.Min(coordinate(a, axis), coordinate(b, axis)), math.Max(coordinate(a, axis), coordinate(b, axis))
		start, _ := slices.BinarySearchFunc(index[axis], low, func(p world.Vec3, v float64) int { return compare(coordinate(p, axis), v) })
		for _, p := range index[axis][start:] {
			if coordinate(p, axis) > high {
				break
			}
			t := dot(sub(p, a), delta) / length
			if t <= 1e-9 || t >= 1-1e-9 {
				continue
			}
			difference := sub(p, add(a, scale(delta, t)))
			if dot(difference, difference) <= 1e-16 {
				cuts = append(cuts, t)
			}
		}
		slices.Sort(cuts)
		last := -1.0
		for _, t := range cuts {
			if t-last > 1e-9 {
				result = append(result, add(a, scale(delta, t)))
				last = t
			}
		}
	}
	return result
}
