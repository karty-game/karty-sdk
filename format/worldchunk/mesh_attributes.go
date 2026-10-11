package worldchunk

import (
	"math"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

type meshAttributeDomain struct {
	polygon []world.Vec3
	band    Band
	bandUV  *world.SurfaceUV
}

func newMeshCoverage(document *world.Document, surface worldlightmap.Surface) *meshCoverage {
	coverage := &meshCoverage{surface: surface, polygons: renderPolygons(document, surface)}
	binding := surface.Binding
	_, coverage.mapping, coverage.secondary = surfaceMaterial(document, binding)
	if coverage.mapping == nil {
		triangle := Triangle{Sector: -1, Solid: -1, Surface: binding.Kind, SurfaceEdge: binding.Edge}
		if surfaceCode(binding.Kind) <= 3 {
			triangle.Sector = binding.Index
		} else {
			triangle.Solid = binding.Index
		}
		coverage.mapping = legacyMapping(document, triangle)
	}
	for index, projection := range coverage.mapping.Projections {
		coverage.bases[index] = projectionBasis(projection, unpacked(packed(surface.Normal)))
	}
	if binding.Kind == "sector-wall" {
		wall := document.Sectors[binding.Index].Walls[binding.Edge]
		if len(wall.FrameRegions) > 0 {
			for _, region := range wall.FrameRegions {
				domain := meshAttributeDomain{polygon: make([]world.Vec3, len(region.Vertices)), bandUV: region.UV}
				for index, point := range region.Vertices {
					domain.polygon[index] = world.Vec3{X: wall.Start.X + (wall.End.X-wall.Start.X)*point.X,
						Y: wall.Start.Y + (wall.End.Y-wall.Start.Y)*point.X, Z: point.Y}
				}
				if region.Coverage != world.FrameCoverageMain {
					domain.band = Band{Material: region.Material, Coverage: 1, RepeatU: region.RepeatU, RepeatV: region.RepeatV}
					if region.Coverage == world.FrameCoverageOpaque {
						domain.band.Coverage = 2
					}
				}
				if domain.bandUV != nil {
					domain.band.Basis = projectionBasis(domain.bandUV.Projections[0], unpacked(packed(surface.Normal)))
				}
				coverage.attributes = append(coverage.attributes, domain)
			}
			return coverage
		}
	}
	for _, polygon := range coverage.polygons {
		coverage.attributes = append(coverage.attributes, meshAttributeDomain{polygon: polygon})
	}
	return coverage
}

func affineAttributeMatches(actual float32, plane world.UVPlane, position world.Vec3, offset float64) bool {
	want := evaluate(plane, position) - offset
	// Stored positions and interpolated UVs each have a bounded float32 error.
	value := float32(want)
	uvError := 4 * math.Abs(float64(math.Nextafter32(value, float32(math.Inf(1))))-float64(value))
	positionError := projectedTolerance(position, world.Vec3{X: plane.X, Y: plane.Y, Z: plane.Z})
	return math.Abs(float64(actual)-want) <= max(1e-7, uvError+positionError)
}

func uvAttributesMatch(actual [2]float32, projection world.UVProjection, position, origin world.Vec3, repeatU, repeatV bool) bool {
	u, v := 0.0, 0.0
	if repeatU {
		u = math.Floor(evaluate(projection.U, origin))
	}
	if repeatV {
		v = math.Floor(evaluate(projection.V, origin))
	}
	return affineAttributeMatches(actual[0], projection.U, position, u) && affineAttributeMatches(actual[1], projection.V, position, v)
}

func (coverage *meshCoverage) validateAttributes(triangle Triangle, vertices []Vertex) error {
	if triangle.Normal != packed(coverage.surface.Normal) || triangle.ProjectionBases != coverage.bases ||
		triangle.Tangent != coverage.bases[0].Tangent || triangle.Bitangent != coverage.bases[0].Bitangent {
		return ErrChunk
	}
	weights := [3]float32{}
	if coverage.secondary != nil && coverage.secondary.Strength > 0 {
		for index, weight := range coverage.secondary.UV.Weights {
			weights[index] = float32(weight)
		}
	}
	if triangle.Secondary.Weights != weights {
		return ErrChunk
	}
	for _, domain := range coverage.attributes {
		if triangle.Band != domain.band {
			continue
		}
		matched := true
		for _, vertex := range vertices {
			position := unpacked(vertex.Position)
			if !insideRenderPolygon(position, domain.polygon, coverage.surface.Normal) {
				matched = false
				break
			}
			origin := domain.polygon[0]
			for index := range 3 {
				if index < len(coverage.mapping.Projections) {
					matched = matched &&
						uvAttributesMatch(vertex.UV[index], coverage.mapping.Projections[index], position, origin, true, true)
				} else {
					matched = matched && vertex.UV[index] == [2]float32{}
				}
				if triangle.Secondary.MappingCount > 0 && index < len(coverage.secondary.UV.Projections) {
					matched = matched &&
						uvAttributesMatch(vertex.SecondaryUV[index], coverage.secondary.UV.Projections[index], position, origin, true, true)
				} else {
					matched = matched && vertex.SecondaryUV[index] == [2]float32{}
				}
			}
			if domain.bandUV != nil {
				matched = matched &&
					uvAttributesMatch(
						vertex.BandUV,
						domain.bandUV.Projections[0],
						position,
						origin,
						triangle.Band.RepeatU,
						triangle.Band.RepeatV,
					)
			} else {
				matched = matched && vertex.BandUV == [2]float32{}
			}
			projection := coverage.mapping.Projections[0]
			matched = matched && affineAttributeMatches(vertex.WaveUV[0], projection.U, position, 0) &&
				affineAttributeMatches(vertex.WaveUV[1], projection.V, position, 0)
			if triangle.Chart == -1 {
				matched = matched && vertex.LightmapUV == [2]float32{}
			}
		}
		if matched {
			return nil
		}
	}
	return ErrChunk
}
