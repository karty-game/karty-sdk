package worldlightmapbake

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
	"github.com/karty-game/karty-sdk/format/worldterrain"
)

func terrainInputDigest(terrain *worldterrain.Document) (string, error) {
	if terrain == nil {
		return "", nil
	}
	encoded, err := worldterrain.Encode(*terrain)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:]), nil
}

func prepareFixedTerrain(document world.Document, options Options, surfaces []worldlightmap.Surface) error {
	if options.Terrain == nil {
		if options.TerrainReceivers != nil {
			return fmt.Errorf("terrain receivers require complete transport")
		}
		return nil
	}
	if !options.fixed {
		return fmt.Errorf("terrain transport requires fixed producer")
	}
	if err := worldterrain.ValidateForWorld(options.Terrain, &document); err != nil {
		return err
	}
	if options.TerrainReceivers != nil {
		if err := worldterrain.ValidateForWorld(options.TerrainReceivers, &document); err != nil {
			return err
		}
	}
	for index, surface := range surfaces {
		if surface.Binding.Kind != "sector-floor" {
			continue
		}
		for _, sector := range options.Terrain.ExteriorSectors {
			if surface.Binding.Index == int(sector) {
				surfaces[index].Polygons = nil
			}
		}
	}
	return nil
}

// appendTerrain adds the actual immutable heightfield triangles to the same
// transport accelerator as rooms/solids. V1 uses far-colour diffuse reflectance
// for terrain bounces; normal/detail textures remain material response only.
func (p *preparedTransport) appendTerrain(ctx context.Context, options Options) error {
	if options.Terrain == nil {
		return nil
	}
	meshes, err := worldterrain.CompilePatches(options.Terrain)
	if err != nil {
		return err
	}
	for _, mesh := range meshes {
		for index := 0; index < len(mesh.Indices); index += 3 {
			if index%1024 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			var points [3]vec
			for corner := range points {
				point := mesh.Vertices[mesh.Indices[index+corner]].Position
				points[corner] = vec{X: float64(point[0]), Y: float64(point[1]), Z: float64(point[2])}
			}
			normal := unit(cross(sub(points[1], points[0]), sub(points[2], points[0])))
			surface := len(p.surfaces)
			p.surfaces = append(p.surfaces, worldlightmap.Surface{Normal: normal})
			p.surfaceFrames = append(p.surfaceFrames, newSampleFrame(normal))
			p.materials = append(p.materials, reflectance{terrain: options.Terrain})
			triangle := triangle{a: points[0], b: points[1], c: points[2], surface: surface}
			triangle.bounds = emptyBox().point(points[0]).point(points[1]).point(points[2])
			p.accel.triangles = append(p.accel.triangles, triangle)
		}
	}
	return nil
}

func terrainReflectance(terrain *worldterrain.Document, p vec) vec {
	u := (p.X - float64(terrain.Origin[0])) / (float64(terrain.Columns-1) * float64(terrain.CellSize))
	v := (p.Y - float64(terrain.Origin[1])) / (float64(terrain.Rows-1) * float64(terrain.CellSize))
	image := terrain.FarColour
	x := min(int(image.Width)-1, max(0, int(math.Round(u*float64(image.Width-1)))))
	y := min(int(image.Height)-1, max(0, int(math.Round(v*float64(image.Height-1)))))
	at := (y*int(image.Width) + x) * 4
	return vec{X: diffuseBytes[image.Pixels[at]], Y: diffuseBytes[image.Pixels[at+1]], Z: diffuseBytes[image.Pixels[at+2]]}
}

func (s *scene) fixedTerrain(ctx context.Context, options Options, counts *counters) ([]worldlightmap.TerrainPatchLighting, error) {
	if options.TerrainReceivers == nil {
		return nil, nil
	}
	count := 0
	for _, patch := range options.TerrainReceivers.Patches {
		count += (int(patch.Columns) + 1) * (int(patch.Rows) + 1) * 9
	}
	if count > worldlightmap.MaxFixedLightingSize/4 {
		return nil, fmt.Errorf("terrain coefficients exceed fixed chunk bound")
	}
	meshes, err := worldterrain.CompilePatches(options.TerrainReceivers)
	if err != nil {
		return nil, err
	}
	result := make([]worldlightmap.TerrainPatchLighting, len(meshes))
	for patch, mesh := range meshes {
		result[patch] = worldlightmap.TerrainPatchLighting{Patch: patch, Coefficients: make([]float32, len(mesh.Vertices)*9)}
		for index, vertex := range mesh.Vertices {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			p := vec{X: float64(vertex.Position[0]), Y: float64(vertex.Position[1]), Z: float64(vertex.Position[2])}
			n := vec{X: float64(vertex.Normal[0]), Y: float64(vertex.Normal[1]), Z: float64(vertex.Normal[2])}
			n = unit(n)
			tangent, bitangent := worldlightmap.TerrainLightingBasis(n)
			chart := worldlightmap.Chart{Normal: n, Tangent: tangent, Bitangent: bitangent}
			coefficients := s.direct(p, chart, counts)
			s.indirect(ctx, p, chart, newSampleFrame(n), terrainSampleIndex(p, n), options, counts, &coefficients)
			for basis, color := range coefficients {
				at := index*9 + basis*3
				result[patch].Coefficients[at], result[patch].Coefficients[at+1], result[patch].Coefficients[at+2] = float32(
					color.X,
				), float32(
					color.Y,
				), float32(
					color.Z,
				)
			}
		}
	}
	return result, nil
}

// Shared vertices must keep their sample rotation across patch/crop layouts.
// Fold exact physical coordinates and the normalized shading normal into a
// nonnegative 31-bit index, preserving the existing user-seed combination on
// both 32- and 64-bit hosts. Signed zero denotes the same physical component.
func terrainSampleIndex(position, normal vec) int {
	hash := uint64(0x7465727261696e33)
	for _, value := range [6]float64{position.X, position.Y, position.Z, normal.X, normal.Y, normal.Z} {
		bits := uint64(0)
		if value != 0 {
			bits = math.Float64bits(value)
		}
		hash ^= bits + 0x9e3779b97f4a7c15
		hash = (hash ^ (hash >> 30)) * 0xbf58476d1ce4e5b9
		hash = (hash ^ (hash >> 27)) * 0x94d049bb133111eb
		hash ^= hash >> 31
	}
	return int(hash & 0x7fffffff)
}
