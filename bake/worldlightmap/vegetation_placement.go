package worldlightmapbake

import (
	"fmt"
	"math"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldterrain"
	"github.com/karty-game/karty-sdk/format/worldvegetation"
)

type vegetationBakeLayer struct {
	preset     worldvegetation.Preset
	bounds     [4]float32
	planes     [][4]float32
	plane      [3]float32
	mean       float32
	candidates int
	seed       uint32
	ceiling    bool
	terrain    *worldterrain.Document
}

func prepareVegetationLayers(
	document *world.Document,
	input *Vegetation,
	terrain *worldterrain.Document,
) ([]vegetationBakeLayer, int, error) {
	if err := worldvegetation.ValidateForWorld(&input.Document, document); err != nil {
		return nil, 0, err
	}
	presets := make(map[string]worldvegetation.Preset, len(input.Document.Presets))
	for _, preset := range input.Document.Presets {
		presets[preset.Name] = preset
	}
	layers := make([]vegetationBakeLayer, 0, len(input.Document.Layers))
	triangles := 0
	for _, authored := range input.Document.Layers {
		preset := presets[authored.Preset]
		sector := document.Sectors[authored.Sector]
		layer := vegetationBakeLayer{
			preset:  preset,
			ceiling: authored.Surface == worldvegetation.SurfaceCeiling,
			bounds: [4]float32{
				math.MaxFloat32,
				math.MaxFloat32,
				-math.MaxFloat32,
				-math.MaxFloat32,
			},
			planes: make([][4]float32, len(sector.Walls)),
		}
		plane := sector.Floor
		if layer.ceiling {
			plane = sector.Ceiling
		}
		layer.plane = [3]float32{float32(plane.A), float32(plane.B), float32(plane.C)}
		area := 0.0
		for _, wall := range sector.Walls {
			area += wall.Start.X*wall.End.Y - wall.End.X*wall.Start.Y
		}
		sign := 1.0
		if area < 0 {
			sign = -1
		}
		for index, wall := range sector.Walls {
			dx, dy := wall.End.X-wall.Start.X, wall.End.Y-wall.Start.Y
			length := math.Hypot(dx, dy)
			nx, ny := -dy/length*sign, dx/length*sign
			margin := preset.Width.Max/2 + preset.WindAmplitude
			if ordinaryVegetationEdge(document, int(authored.Sector), wall) {
				margin = 0
			}
			layer.planes[index] = [4]float32{float32(nx), float32(ny), float32(-nx*wall.Start.X - ny*wall.Start.Y), float32(margin)}
			layer.bounds[0] = min(layer.bounds[0], float32(wall.Start.X))
			layer.bounds[1] = min(layer.bounds[1], float32(wall.Start.Y))
			layer.bounds[2] = max(layer.bounds[2], float32(wall.Start.X))
			layer.bounds[3] = max(layer.bounds[3], float32(wall.Start.Y))
		}
		mean := (float64(layer.bounds[2]) - float64(layer.bounds[0])) * (float64(layer.bounds[3]) - float64(layer.bounds[1])) * float64(
			float32(authored.Density),
		)
		count := math.Ceil(mean)
		perPlant := []int{4, 300, 588, 432}[preset.Geometry]
		if count > float64((MaxVegetationTriangles-triangles)/perPlant) {
			return nil, 0, fmt.Errorf("vegetation bake exceeds %d triangles", MaxVegetationTriangles)
		}
		layer.mean, layer.candidates = float32(mean), int(count)
		triangles += layer.candidates * perPlant
		// The visual shader splits seed words but reconstructs the same uint32.
		layer.seed = authored.Seed
		if !layer.ceiling && terrain != nil {
			for _, sectorID := range terrain.ExteriorSectors {
				if uint32(sectorID) == authored.Sector {
					layer.terrain = terrain
					break
				}
			}
		}
		layers = append(layers, layer)
	}
	return layers, triangles, nil
}

func ordinaryVegetationEdge(document *world.Document, source int, wall world.Wall) bool {
	if wall.Portal < 0 || int(wall.Portal) >= len(document.Sectors) {
		return false
	}
	origin, target := document.Sectors[source], document.Sectors[wall.Portal]
	if origin.Sky != target.Sky {
		return false
	}
	for _, point := range [2]world.Vec2{wall.Start, wall.End} {
		for _, planes := range [][2]world.Plane{{origin.Floor, target.Floor}, {origin.Ceiling, target.Ceiling}} {
			a, b := planes[0], planes[1]
			if math.Abs((a.A-b.A)*point.X+(a.B-b.B)*point.Y+a.C-b.C) > 1e-6 {
				return false
			}
		}
	}
	for index, other := range target.Walls {
		if wall.PortalWall > 0 && int(wall.PortalWall)-1 != index {
			continue
		}
		if int(other.Portal) == source && wall.Start == other.End && wall.End == other.Start {
			return true
		}
	}
	return false
}

func hashVegetation(value uint32) uint32 {
	value ^= value >> 16
	value *= 2246822519
	value ^= value >> 13
	value *= 3266489917
	return value ^ (value >> 16)
}
func randomVegetation(identity, salt uint32) float32 {
	return float32(hashVegetation(identity^salt)>>8) * (1.0 / 16777216.0)
}
func vegetationMix(a, b, t float32) float32 { return a*(1-t) + b*t }

func vegetationTerrainHeight(source *worldterrain.Document, xy [2]float32) (float32, bool) {
	x, y := (xy[0]-source.Origin[0])/source.CellSize, (xy[1]-source.Origin[1])/source.CellSize
	if x < 0 || y < 0 || x > float32(source.Columns-1) || y > float32(source.Rows-1) {
		return 0, false
	}
	column, row := min(int(math.Floor(float64(x))), int(source.Columns)-2), min(int(math.Floor(float64(y))), int(source.Rows)-2)
	if source.Holes[row*(int(source.Columns)-1)+column] != 0 {
		return 0, false
	}
	index := row*int(source.Columns) + column
	a, b, c := source.Heights[index], source.Heights[index+1], source.Heights[index+int(source.Columns)]
	fx, fy := x-float32(column), y-float32(row)
	if fx+fy <= 1 {
		return a*(1-fx-fy) + b*fx + c*fy, true
	}
	d := source.Heights[index+int(source.Columns)+1]
	return b*(1-fy) + d*(fx+fy-1) + c*(1-fx), true
}
