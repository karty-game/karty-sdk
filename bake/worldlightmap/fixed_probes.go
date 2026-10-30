package worldlightmapbake

import (
	"context"
	"math"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
	"github.com/karty-game/karty-sdk/format/worldterrain"
)

var probeAxes = [6]vec{{X: 1}, {X: -1}, {Y: 1}, {Y: -1}, {Z: 1}, {Z: -1}}

func (s *scene) fixedProbes(
	ctx context.Context,
	document world.Document,
	layout worldlightmap.Layout,
	lights []worldlightmap.CompiledLight,
	options Options,
	counts *counters,
) ([]worldlightmap.LightingProbe, error) {
	sectors := make([]int, len(document.Sectors))
	for index := range sectors {
		sectors[index] = index
	}
	if layout.Receivers != nil {
		sectors = layout.Receivers.Sectors
	}
	// Five XY positions and three heights cap every possible world at 3840
	// probes before tracing (256 sectors), below the public 4096-probe bound.
	result := make([]worldlightmap.LightingProbe, 0, len(sectors)*15)
	for _, sectorID := range sectors {
		sector := document.Sectors[sectorID]
		centroid := vec{}
		low, high := vec{X: math.Inf(1), Y: math.Inf(1)}, vec{X: math.Inf(-1), Y: math.Inf(-1)}
		for _, wall := range sector.Walls {
			centroid.X += wall.Start.X
			centroid.Y += wall.Start.Y
			low.X = min(low.X, wall.Start.X)
			low.Y = min(low.Y, wall.Start.Y)
			high.X = max(high.X, wall.Start.X)
			high.Y = max(high.Y, wall.Start.Y)
		}
		centroid.X /= float64(len(sector.Walls))
		centroid.Y /= float64(len(sector.Walls))
		positions := []vec{centroid}
		for _, fx := range [2]float64{.25, .75} {
			for _, fy := range [2]float64{.25, .75} {
				candidate := vec{X: low.X + (high.X-low.X)*fx, Y: low.Y + (high.Y-low.Y)*fy}
				if probeInsideSector(sector, candidate) {
					positions = append(positions, candidate)
				}
			}
		}
		for _, center := range positions {
			floor := sector.Floor.A*center.X + sector.Floor.B*center.Y + sector.Floor.C
			if height, ok := worldterrain.HeightAt(options.Terrain, center.X, center.Y); ok {
				floor = height
			}
			ceiling := sector.Ceiling.A*center.X + sector.Ceiling.B*center.Y + sector.Ceiling.C
			for _, fraction := range [3]float64{.2, .5, .8} {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				position := center
				position.Z = floor + (ceiling-floor)*fraction
				if probeInsideSolid(document, position) {
					continue
				}
				probe := worldlightmap.LightingProbe{
					Sector:   sectorID,
					Position: position,
					Direct:   [2]worldlightmap.ProbeDirect{{Light: -1}, {Light: -1}},
				}
				strengths := [2]float64{}
				for index, light := range lights {
					direction, radiance, distance := fixedLightAt(light, position)
					strength := maximum(radiance)
					if strength <= 0 || !s.visible(position, direction, direction, distance, counts) {
						continue
					}
					if strength > strengths[0] {
						probe.Direct[1] = probe.Direct[0]
						strengths[1] = strengths[0]
						probe.Direct[0] = worldlightmap.ProbeDirect{Light: index, Visibility: 1}
						strengths[0] = strength
					} else if strength > strengths[1] {
						probe.Direct[1] = worldlightmap.ProbeDirect{Light: index, Visibility: 1}
						strengths[1] = strength
					}
				}
				for axis, normal := range probeAxes {
					frame := newSampleFrame(normal)
					chart := worldlightmap.Chart{Normal: normal, Tangent: frame.tangent, Bitangent: frame.bitangent}
					var coefficients [3]vec
					s.indirect(ctx, position, chart, frame, len(result)*6+axis, options, counts, &coefficients)
					ambient := scale(add(add(coefficients[0], coefficients[1]), coefficients[2]), 1.0/3)
					ambient = add(ambient, fixedFill(s.fixedFill, normal))
					for index, light := range lights {
						if probe.Direct[0].Light == index || probe.Direct[1].Light == index {
							continue
						}
						direction, radiance, distance := fixedLightAt(light, position)
						cosine := dot(normal, direction)
						if cosine > 0 && maximum(radiance) > 0 && s.visible(position, direction, direction, distance, counts) {
							ambient = add(ambient, scale(radiance, halfLambert(cosine)))
						}
					}
					probe.Ambient[axis] = ambient
				}
				result = append(result, probe)
			}
		}
	}
	return result, nil
}

func probeInsideSector(sector world.Sector, p vec) bool {
	for _, wall := range sector.Walls {
		if (wall.End.X-wall.Start.X)*(p.Y-wall.Start.Y)-(wall.End.Y-wall.Start.Y)*(p.X-wall.Start.X) < -1e-8 {
			return false
		}
	}
	return true
}

func probeInsideSolid(document world.Document, p vec) bool {
	if document.StaticSolids == nil {
		return false
	}
	for _, solid := range document.StaticSolids.Items {
		if p.Z <= solid.Bottom.A*p.X+solid.Bottom.B*p.Y+solid.Bottom.C || p.Z >= solid.Top.A*p.X+solid.Top.B*p.Y+solid.Top.C {
			continue
		}
		inside := true
		for index, a := range solid.Footprint {
			b := solid.Footprint[(index+1)%len(solid.Footprint)]
			if (b.X-a.X)*(p.Y-a.Y)-(b.Y-a.Y)*(p.X-a.X) < -1e-8 {
				inside = false
				break
			}
		}
		if inside {
			return true
		}
	}
	return false
}

func fixedLightAt(light worldlightmap.CompiledLight, position vec) (vec, vec, float64) {
	if light.Kind == "sun" {
		return light.Direction, light.Color, math.Inf(1)
	}
	delta := sub(light.Position, position)
	distance := math.Sqrt(dot(delta, delta))
	if distance <= epsilon(position) || distance >= light.Radius {
		return vec{}, vec{}, distance
	}
	return scale(delta, 1/distance), scale(light.Color, square(1-distance*distance/(light.Radius*light.Radius))), distance
}
