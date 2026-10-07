package world

import (
	"fmt"
	"math"
)

func validateFramePartition(d *Document, si, wi int) error {
	w := &d.Sectors[si].Walls[wi]
	p, err := ProfileForWall(d, si, wi)
	if err != nil {
		return err
	}
	spans := spansForProfile(p, w.Portal >= 0)
	expected := 0.0
	for _, span := range spans {
		ha := affineHeight(span.top, span.a) - affineHeight(span.bottom, span.a)
		hb := affineHeight(span.top, span.b) - affineHeight(span.bottom, span.b)
		expected += (ha + hb) * (span.b - span.a) * .5
	}
	total := 0.0
	for i, r := range w.FrameRegions {
		if len(r.Vertices) < 3 || len(r.Vertices) > MaxFrameRegionVertices {
			return ErrBounds
		}
		if r.Coverage == FrameCoverageMain {
			if r.Material != w.Material || r.UV != nil || r.RepeatU || r.RepeatV {
				return ErrMaterialMapping
			}
		} else {
			if r.Coverage != FrameCoverageOpaque && r.Coverage != FrameCoverageMasked || ValidateSurfaceUV(r.UV) != nil ||
				len(r.UV.Projections) != 1 {
				return ErrMaterialMapping
			}
			if r.Coverage == FrameCoverageOpaque && !frameDomainCovered(*w, r) {
				return ErrMaterialMapping
			}
		}
		for j, v := range r.Vertices {
			if !validVec2(v) || v.X < 0 || v.X > 1 {
				return fmt.Errorf("region %d vertex %d invalid: %w", i, j, ErrGeometry)
			}
			a, b := r.Vertices[j], r.Vertices[(j+1)%len(r.Vertices)]
			for k, other := range r.Vertices {
				if k != j && k != (j+1)%len(r.Vertices) && orientedDistance(a, b, other) <= 0 {
					return fmt.Errorf("region %d not convex: %w", i, ErrGeometry)
				}
			}
		}
		area := frameArea(r.Vertices)
		if area <= 0 {
			return ErrGeometry
		}
		contained := false
		for _, span := range spans {
			inside := true
			for _, v := range r.Vertices {
				if v.X < span.a-geometryEpsilon || v.X > span.b+geometryEpsilon || v.Y < affineHeight(span.bottom, v.X)-geometryEpsilon ||
					v.Y > affineHeight(span.top, v.X)+geometryEpsilon {
					inside = false
					break
				}
			}
			if inside {
				contained = true
				break
			}
		}
		if !contained {
			return fmt.Errorf("region %d outside solid span: %w", i, ErrGeometry)
		}
		for j := 0; j < i; j++ {
			if framePolygonsOverlap(r.Vertices, w.FrameRegions[j].Vertices) {
				return fmt.Errorf("region %d overlaps region %d: %w", i, j, ErrGeometry)
			}
		}
		total += area
	}
	if math.Abs(total-expected) > 1e-8*math.Max(1, expected) {
		return fmt.Errorf("partition area %g expected %g: %w", total, expected, ErrGeometry)
	}
	return nil
}
func frameArea(v []Vec2) float64 {
	sum := 0.0
	// Translate around the first point to avoid cancellation at large elevations.
	origin := v[0]
	for i := 1; i < len(v)-1; i++ {
		sum += (v[i].X-origin.X)*(v[i+1].Y-origin.Y) - (v[i].Y-origin.Y)*(v[i+1].X-origin.X)
	}
	return sum * .5
}
func framePolygonsOverlap(a, b []Vec2) bool {
	// Separating axes are edge normals. Touching boundaries have zero area and
	// are valid; tolerance handles independently evaluated affine intersections.
	separate := func(edges, other []Vec2) bool {
		for i, p := range edges {
			q := edges[(i+1)%len(edges)]
			inside := false
			for _, v := range other {
				signed := (q.X-p.X)*(v.Y-p.Y) - (q.Y-p.Y)*(v.X-p.X)
				if signed > geometryEpsilon {
					inside = true
					break
				}
			}
			if !inside {
				return true
			}
		}
		return false
	}
	return !separate(a, b) && !separate(b, a)
}
