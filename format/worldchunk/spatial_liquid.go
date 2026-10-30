package worldchunk

import (
	"fmt"
	"math"
	"slices"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

// spatialLiquidTriangles preserves one canonical outer anchor set. Interior
// rings change geometry quality without introducing unshared outer edge points.
func spatialLiquidTriangles(
	polygon []world.Vec3,
	selection *worldlightmap.SurfaceSelection,
	document *world.Document,
	source Triangle,
) ([][3]world.Vec3, error) {
	step := selection.LiquidEdgeStep
	if step <= 0 {
		return nil, ErrChunk
	}
	plane := world.Plane{}
	switch source.Surface {
	case "sector-floor":
		plane = document.Sectors[source.Sector].Floor
	case "solid-top":
		plane = document.StaticSolids.Items[source.Solid].Top
	default:
		return nil, ErrChunk
	}
	anchors := make([]world.Vec3, 0, len(polygon))
	for i, a := range polygon {
		b := polygon[(i+1)%len(polygon)]
		parameters := []float64{0}
		for axis := range 2 {
			left, right := a.X, b.X
			if axis == 1 {
				left, right = a.Y, b.Y
			}
			if left == right {
				continue
			}
			lo, hi := int(math.Ceil(min(left, right)/step)), int(math.Floor(max(left, right)/step))
			if hi-lo > MaxTriangles {
				return nil, fmt.Errorf("spatial liquid edge budget: %w", ErrChunk)
			}
			for k := lo; k <= hi; k++ {
				t := (float64(k)*step - left) / (right - left)
				if t > 1e-10 && t < 1-1e-10 {
					parameters = append(parameters, t)
				}
			}
		}
		if len(parameters) > MaxTriangles {
			return nil, fmt.Errorf("spatial liquid anchor work budget: %w", ErrChunk)
		}
		slices.Sort(parameters)
		for _, t := range parameters {
			p := world.Vec3{X: a.X + t*(b.X-a.X), Y: a.Y + t*(b.Y-a.Y)}
			// Quantize grid intersections before final float32 storage. Shared axis
			// aligned boundaries then produce identical packed rest vertices.
			for axis := range 2 {
				v := &p.X
				if axis == 1 {
					v = &p.Y
				}
				q := *v / step
				if math.Abs(q-math.Round(q)) < 1e-8 {
					*v = math.Round(q) * step
				}
			}
			p.Z = plane.A*p.X + plane.B*p.Y + plane.C
			if len(anchors) > 0 && packed(anchors[len(anchors)-1]) == packed(p) {
				continue
			}
			if len(anchors) >= MaxTriangles {
				return nil, fmt.Errorf("spatial liquid anchor budget: %w", ErrChunk)
			}
			anchors = append(anchors, p)
		}
	}
	if len(anchors) > 1 && packed(anchors[0]) == packed(anchors[len(anchors)-1]) {
		anchors = anchors[:len(anchors)-1]
	}
	if len(anchors) < 3 {
		return nil, nil
	}
	rings := 5
	if selection.LiquidMaxSubdivisions > 0 {
		rings = min(rings, selection.LiquidMaxSubdivisions-1)
	}
	count := len(anchors) * (2*rings + 1)
	if count > MaxTriangles {
		return nil, fmt.Errorf("spatial liquid ring budget: %w", ErrChunk)
	}
	triangles := make([][3]world.Vec3, 0, count)
	centre := world.Vec3{}
	for _, p := range anchors {
		centre = add(centre, scale(p, 1/float64(len(anchors))))
	}
	centre.Z = plane.A*centre.X + plane.B*centre.Y + plane.C
	outer := anchors
	for ring := 1; ring <= rings; ring++ {
		fraction := 1 - float64(ring)/float64(rings+1)
		inner := make([]world.Vec3, len(anchors))
		for i, p := range anchors {
			inner[i] = add(centre, scale(sub(p, centre), fraction))
			inner[i].Z = plane.A*inner[i].X + plane.B*inner[i].Y + plane.C
		}
		for i, a := range outer {
			j := (i + 1) % len(outer)
			triangles = append(triangles, [3]world.Vec3{a, outer[j], inner[j]}, [3]world.Vec3{a, inner[j], inner[i]})
		}
		outer = inner
	}
	for i, a := range outer {
		triangles = append(triangles, [3]world.Vec3{centre, a, outer[(i+1)%len(outer)]})
	}
	return triangles, nil
}

func spatialLiquidSurface(document *world.Document, source Triangle) bool {
	if source.Normal[2] <= .999999 || document.Animations == nil {
		return false
	}
	for _, binding := range document.Animations.Materials {
		if binding.Material != source.Material {
			continue
		}
		for _, preset := range document.Animations.Presets {
			if preset.Name == binding.Preset {
				return preset.Kind == "liquid" && preset.SurfaceAmplitude > 0
			}
		}
	}
	return false
}
