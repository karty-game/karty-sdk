package worldchunk

import (
	"math"
	"slices"
	"strings"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

// ValidateMeshBinding checks every final RNM coordinate against its semantic
// chart. Geometry validation is separate and never regenerates triangles.
func ValidateMeshBinding(mesh *Mesh, layout *worldlightmap.Layout) error {
	var work cooperativeWork
	if mesh == nil || len(mesh.Triangles) > MaxTriangles || len(mesh.Vertices) != 3*len(mesh.Triangles) {
		return ErrChunk
	}
	if layout == nil {
		for _, triangle := range mesh.Triangles {
			work.checkpoint(1024)
			if triangle.Chart != -1 {
				return ErrChunk
			}
		}
		return nil
	}
	if !worldlightmap.EqualSurfaceSelections(mesh.Selection, layout.Selection) || layout.Receivers == nil ||
		!slices.Equal(mesh.Sectors, layout.Receivers.Sectors) ||
		!slices.Equal(mesh.Solids, layout.Receivers.Solids) ||
		layout.GeometrySHA256 != mesh.GeometrySHA256 {
		return ErrChunk
	}
	bindings := make(map[worldlightmap.Binding]int, len(layout.Bindings))
	for _, binding := range layout.Bindings {
		work.checkpoint(1024)
		chart := binding.Chart
		binding.Chart = -1
		bindings[binding] = chart
	}
	for i, t := range mesh.Triangles {
		work.checkpoint(1024)
		index := t.Sector
		if index < 0 {
			index = t.Solid
		}
		binding := worldlightmap.Binding{Kind: t.Surface, Index: index, Edge: t.SurfaceEdge, Chart: -1}
		chart, found := bindings[binding]
		if t.Sky {
			chart = -1
		}
		if !found || chart != t.Chart {
			return ErrChunk
		}
		if chart >= 0 {
			if chart >= len(layout.Charts) {
				return ErrChunk
			}
			for _, v := range mesh.Vertices[i*3 : i*3+3] {
				position := unpacked(v.Position)
				c := layout.Charts[chart]
				if !finite(float64(v.LightmapUV[0])) || !finite(float64(v.LightmapUV[1])) ||
					!finite(float64(v.Position[0])) || !finite(float64(v.Position[1])) || !finite(float64(v.Position[2])) ||
					math.Abs(float64(v.LightmapUV[0])-affine(c.UPlane, position)) > 2e-5 ||
					math.Abs(float64(v.LightmapUV[1])-affine(c.VPlane, position)) > 2e-5 {
					return ErrChunk
				}
			}
		}
	}
	return nil
}

func triangleBinding(t Triangle, mesh *Mesh, document *world.Document, vertices []Vertex) (worldlightmap.Binding, error) {
	b := worldlightmap.Binding{Kind: t.Surface, Edge: t.SurfaceEdge, Chart: -1}
	if strings.HasPrefix(t.Surface, "sector-") {
		if t.Solid != -1 {
			return b, ErrChunk
		}
		if _, found := slices.BinarySearch(mesh.Sectors, t.Sector); !found {
			return b, ErrChunk
		}
		b.Index = t.Sector
		s := document.Sectors[t.Sector]
		switch t.Surface {
		case "sector-floor":
			if t.SurfaceEdge != -1 || t.Sky {
				return b, ErrChunk
			}
		case "sector-ceiling":
			if t.SurfaceEdge != -1 || t.Sky != s.Sky {
				return b, ErrChunk
			}
		case "sector-wall":
			if t.SurfaceEdge < 0 || t.SurfaceEdge >= len(s.Walls) || t.Sky != skyWallTriangle(document, t, vertices) {
				return b, ErrChunk
			}
		default:
			return b, ErrChunk
		}
		if t.Outdoor != s.Sky {
			return b, ErrChunk
		}
	} else {
		if t.Sector != -1 || t.Sky || t.Outdoor {
			return b, ErrChunk
		}
		if _, found := slices.BinarySearch(mesh.Solids, t.Solid); !found {
			return b, ErrChunk
		}
		b.Index = t.Solid
		s := document.StaticSolids.Items[t.Solid]
		switch t.Surface {
		case "solid-top", "solid-bottom":
			if t.SurfaceEdge != -1 {
				return b, ErrChunk
			}
		case "solid-side":
			if t.SurfaceEdge < 0 || t.SurfaceEdge >= len(s.Footprint) {
				return b, ErrChunk
			}
		default:
			return b, ErrChunk
		}
	}
	return b, nil
}

func semanticNormal(document *world.Document, binding worldlightmap.Binding) world.Vec3 {
	var plane world.Plane
	switch binding.Kind {
	case "sector-floor":
		plane = document.Sectors[binding.Index].Floor
	case "sector-ceiling":
		plane = document.Sectors[binding.Index].Ceiling
	case "solid-top":
		plane = document.StaticSolids.Items[binding.Index].Top
	case "solid-bottom":
		plane = document.StaticSolids.Items[binding.Index].Bottom
	case "sector-wall":
		wall := document.Sectors[binding.Index].Walls[binding.Edge]
		return unit(world.Vec3{X: wall.Start.Y - wall.End.Y, Y: wall.End.X - wall.Start.X})
	case "solid-side":
		solid := document.StaticSolids.Items[binding.Index]
		a, b := solid.Footprint[binding.Edge], solid.Footprint[(binding.Edge+1)%len(solid.Footprint)]
		return unit(world.Vec3{X: b.Y - a.Y, Y: a.X - b.X})
	}
	normal := unit(world.Vec3{X: -plane.A, Y: -plane.B, Z: 1})
	if binding.Kind == "sector-ceiling" || binding.Kind == "solid-bottom" {
		normal = scale(normal, -1)
	}
	return normal
}
