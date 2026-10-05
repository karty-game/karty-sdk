package worldlightmap

import (
	"fmt"
	"math"

	"github.com/karty-game/karty-sdk/format/world"
)

// Surface describes actual receiver polygons before triangulation. Empty
// Polygons explicitly describes a fully open portal wall.
type Surface struct {
	Binding  Binding
	Normal   world.Vec3
	Polygons [][]world.Vec3
}

// Surfaces discovers semantic receivers in stable sector-floor/ceiling/wall,
// then solid-top/bottom/side order. It preserves separate solid edge identities.
func Surfaces(document *world.Document) ([]Surface, error) {
	if err := world.Validate(document); err != nil {
		return nil, err
	}
	result := make([]Surface, 0, len(document.Sectors)*6)
	for i, s := range document.Sectors {
		points := make([]world.Vec2, len(s.Walls))
		for j, w := range s.Walls {
			points[j] = w.Start
		}
		result = append(result, capSurface("sector-floor", i, s.Floor, points, false), capSurface("sector-ceiling", i, s.Ceiling, points, true))
		for edge, w := range s.Walls {
			length := math.Hypot(w.End.X-w.Start.X, w.End.Y-w.Start.Y)
			surface := Surface{Binding: Binding{Kind: "sector-wall", Index: i, Edge: edge, Chart: -1}, Normal: world.Vec3{X: (w.Start.Y - w.End.Y) / length, Y: (w.End.X - w.Start.X) / length}}
			heights := [4][2]float64{{elevation(s.Floor, w.Start), elevation(s.Floor, w.End)}, {elevation(s.Ceiling, w.Start), elevation(s.Ceiling, w.End)}}
			heights[2], heights[3] = heights[0], heights[1]
			if w.Portal >= 0 {
				target, ok := destination(document, i, w)
				if !ok {
					return nil, fmt.Errorf("portal wall: %w", ErrLayout)
				}
				neighbor := document.Sectors[w.Portal]
				tz := 0.0
				if !coincident(w, target) {
					a := world.Vec2{X: (w.Start.X + w.End.X) * .5, Y: (w.Start.Y + w.End.Y) * .5}
					b := world.Vec2{X: (target.Start.X + target.End.X) * .5, Y: (target.Start.Y + target.End.Y) * .5}
					tz = elevation(s.Floor, a) - elevation(neighbor.Floor, b)
				}
				heights[2] = [2]float64{elevation(neighbor.Floor, target.End) + tz, elevation(neighbor.Floor, target.Start) + tz}
				heights[3] = [2]float64{elevation(neighbor.Ceiling, target.End) + tz, elevation(neighbor.Ceiling, target.Start) + tz}
			}
			profile, err := world.NewWallProfile(heights, 1e-9)
			if err != nil {
				return nil, err
			}
			for cut := 0; cut < profile.Count-1; cut++ {
				a, b := profile.Cuts[cut], profile.Cuts[cut+1]
				middle := (a + b) * .5
				low, high := profile.Controls(middle)
				spans := [][2]int{{0, 1}}
				if w.Portal >= 0 && profile.Height(high, middle) > profile.Height(low, middle)+1e-9 {
					spans = [][2]int{{0, low}, {high, 1}}
				}
				for _, span := range spans {
					if profile.Height(span[1], middle) <= profile.Height(span[0], middle)+1e-9 {
						continue
					}
					pa := world.Vec2{X: w.Start.X + (w.End.X-w.Start.X)*a, Y: w.Start.Y + (w.End.Y-w.Start.Y)*a}
					pb := world.Vec2{X: w.Start.X + (w.End.X-w.Start.X)*b, Y: w.Start.Y + (w.End.Y-w.Start.Y)*b}
					surface.Polygons = append(surface.Polygons, []world.Vec3{point(pa, profile.Height(span[0], a)), point(pb, profile.Height(span[0], b)), point(pb, profile.Height(span[1], b)), point(pa, profile.Height(span[1], a))})
				}
			}
			result = append(result, surface)
		}
	}
	if document.StaticSolids != nil {
		for i, s := range document.StaticSolids.Items {
			result = append(result, capSurface("solid-top", i, s.Top, s.Footprint, false), capSurface("solid-bottom", i, s.Bottom, s.Footprint, true))
			for edge, a := range s.Footprint {
				b := s.Footprint[(edge+1)%len(s.Footprint)]
				length := math.Hypot(b.X-a.X, b.Y-a.Y)
				result = append(result, Surface{Binding: Binding{Kind: "solid-side", Index: i, Edge: edge, Chart: -1}, Normal: world.Vec3{X: (b.Y - a.Y) / length, Y: (a.X - b.X) / length}, Polygons: [][]world.Vec3{{point(a, elevation(s.Bottom, a)), point(b, elevation(s.Bottom, b)), point(b, elevation(s.Top, b)), point(a, elevation(s.Top, a))}}})
			}
		}
	}
	return result, nil
}

func capSurface(kind string, index int, plane world.Plane, footprint []world.Vec2, reverse bool) Surface {
	normal := unit(world.Vec3{X: -plane.A, Y: -plane.B, Z: 1})
	if reverse {
		normal = scale(normal, -1)
	}
	polygon := make([]world.Vec3, len(footprint))
	for i, v := range footprint {
		polygon[i] = point(v, elevation(plane, v))
	}
	return Surface{Binding: Binding{Kind: kind, Index: index, Edge: -1, Chart: -1}, Normal: normal, Polygons: [][]world.Vec3{polygon}}
}

func destination(document *world.Document, sector int, wall world.Wall) (world.Wall, bool) {
	neighbor := document.Sectors[wall.Portal]
	if wall.PortalWall > 0 {
		return neighbor.Walls[int(wall.PortalWall)-1], true
	}
	for _, other := range neighbor.Walls {
		if other.Portal == int32(sector) && coincident(wall, other) {
			return other, true
		}
	}
	return world.Wall{}, false
}

func coincident(a, b world.Wall) bool {
	return math.Hypot(a.Start.X-b.End.X, a.Start.Y-b.End.Y) <= 1e-9 && math.Hypot(a.End.X-b.Start.X, a.End.Y-b.Start.Y) <= 1e-9
}

func ordinaryPortal(document *world.Document, sector, edge int, wall world.Wall) bool {
	if wall.Portal < 0 {
		return false
	}
	target, ok := destination(document, sector, wall)
	if !ok || target.Portal != int32(sector) || !coincident(wall, target) {
		return false
	}
	if document.Version >= world.Version {
		return int(target.PortalWall) == edge+1
	}
	return true
}
