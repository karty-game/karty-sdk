package worldchunk

import (
	"math"

	"github.com/karty-game/karty-sdk/format/world"
)

// Transport surfaces omit these finite upper spans. Rendering still needs
// their depth/sky masks, independently of physical receiver and caster data.
func upperSkyWallPolygons(document *world.Document, sector, edge int) [][]world.Vec3 {
	profile, ok := skyWallProfile(document, sector, edge)
	if !ok {
		return nil
	}
	wall := document.Sectors[sector].Walls[edge]
	var polygons [][]world.Vec3
	for cut := 0; cut < profile.Count-1; cut++ {
		a, b := profile.Cuts[cut], profile.Cuts[cut+1]
		middle := (a + b) * .5
		low, high := profile.Controls(middle)
		if profile.Height(high, middle) <= profile.Height(low, middle)+1e-9 ||
			profile.Height(1, middle) <= profile.Height(high, middle)+1e-9 {
			continue
		}
		pa := world.Vec2{X: wall.Start.X + (wall.End.X-wall.Start.X)*a, Y: wall.Start.Y + (wall.End.Y-wall.Start.Y)*a}
		pb := world.Vec2{X: wall.Start.X + (wall.End.X-wall.Start.X)*b, Y: wall.Start.Y + (wall.End.Y-wall.Start.Y)*b}
		polygons = append(polygons, []world.Vec3{
			{X: pa.X, Y: pa.Y, Z: profile.Height(high, a)},
			{X: pb.X, Y: pb.Y, Z: profile.Height(high, b)},
			{X: pb.X, Y: pb.Y, Z: profile.Height(1, b)},
			{X: pa.X, Y: pa.Y, Z: profile.Height(1, a)},
		})
	}
	return polygons
}

func skyWallProfile(document *world.Document, sector, edge int) (world.WallProfile, bool) {
	s := document.Sectors[sector]
	w := s.Walls[edge]
	if !s.Sky || w.Portal < 0 || !document.Sectors[w.Portal].Sky {
		return world.WallProfile{}, false
	}
	// All callers have already completely validated the world.
	profile, err := world.ProfileForWall(document, sector, edge)
	return profile, err == nil
}

// Compiled frame regions already lie in one solid span. Match the retained
// compiler's upper-face classification without changing those exact regions.
func upperSkyWallFrame(document *world.Document, sector, edge int, polygon []world.Vec3, packedPositions bool) bool {
	profile, ok := skyWallProfile(document, sector, edge)
	if !ok {
		return false
	}
	wall := document.Sectors[sector].Walls[edge]
	dx, dy := wall.End.X-wall.Start.X, wall.End.Y-wall.Start.Y
	lengthSquared := dx*dx + dy*dy
	for _, position := range polygon {
		t := ((position.X-wall.Start.X)*dx + (position.Y-wall.Start.Y)*dy) / lengthSquared
		low, high := profile.Controls(t)
		tolerance := 1e-9
		if packedPositions {
			fractionError := (math.Abs(dx)*coordinateTolerance(position.X) + math.Abs(dy)*coordinateTolerance(position.Y)) / lengthSquared
			tolerance += coordinateTolerance(position.Z) + math.Abs(profile.Heights[high][1]-profile.Heights[high][0])*fractionError
		}
		if profile.Height(high, t) <= profile.Height(low, t)+1e-9 || position.Z < profile.Height(high, t)-tolerance {
			return false
		}
	}
	return len(polygon) > 0
}

func skyWallTriangle(document *world.Document, triangle Triangle, vertices []Vertex) bool {
	wall := document.Sectors[triangle.Sector].Walls[triangle.SurfaceEdge]
	if wall.Sky {
		return true
	}
	positions := [3]world.Vec3{}
	for index, vertex := range vertices {
		positions[index] = unpacked(vertex.Position)
	}
	if len(wall.FrameRegions) > 0 {
		return upperSkyWallFrame(document, triangle.Sector, triangle.SurfaceEdge, positions[:], true)
	}
	for _, polygon := range upperSkyWallPolygons(document, triangle.Sector, triangle.SurfaceEdge) {
		inside := true
		for _, position := range positions {
			inside = inside && insideRenderPolygon(position, polygon, unpacked(triangle.Normal))
		}
		if inside {
			return true
		}
	}
	return false
}
