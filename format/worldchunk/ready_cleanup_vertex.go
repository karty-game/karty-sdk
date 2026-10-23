package worldchunk

import (
	"math"

	"github.com/karty-game/karty-sdk/format/world"
)

func cleanupInterpolate(source []Vertex, point world.Vec3, plane cleanupPlane) Vertex {
	position := packed(point)
	for _, vertex := range source {
		if vertex.Position == position {
			return vertex
		}
	}
	// Use the two well-conditioned coordinates of the original rendered triangle.
	// Packed positions and every UV stream interpolate from that same triangle;
	// wrapping origins, RNM padding and material/chart ownership stay unchanged.
	normal := [3]float64{plane.normal.X, plane.normal.Y, plane.normal.Z}
	drop := 0
	for axis := 1; axis < 3; axis++ {
		if math.Abs(normal[axis]) > math.Abs(normal[drop]) {
			drop = axis
		}
	}
	first, second := (drop+1)%3, (drop+2)%3
	a, b, c := source[0].Position, source[1].Position, source[2].Position
	x, y := float64(position[first]), float64(position[second])
	ax, ay, bx, by, cx, cy := float64(
		a[first],
	), float64(
		a[second],
	), float64(
		b[first],
	), float64(
		b[second],
	), float64(
		c[first],
	), float64(
		c[second],
	)
	denominator := (by-cy)*(ax-cx) + (cx-bx)*(ay-cy)
	weights := [3]float64{((by-cy)*(x-cx) + (cx-bx)*(y-cy)) / denominator, ((cy-ay)*(x-cx) + (ax-cx)*(y-cy)) / denominator, 0}
	weights[2] = 1 - weights[0] - weights[1]
	// The other two coordinates have already been quantized. Reconstruct the
	// dominant coordinate on the original rendered triangle at those stored
	// coordinates; packing all three independently would tilt a sloping cut.
	position[drop] = float32(weights[0]*float64(a[drop]) + weights[1]*float64(b[drop]) + weights[2]*float64(c[drop]))
	result := Vertex{Position: position}
	for index := range 3 {
		result.UV[index] = cleanupUV(source[0].UV[index], source[1].UV[index], source[2].UV[index], weights)
		result.SecondaryUV[index] = cleanupUV(
			source[0].SecondaryUV[index],
			source[1].SecondaryUV[index],
			source[2].SecondaryUV[index],
			weights,
		)
	}
	result.BandUV = cleanupUV(source[0].BandUV, source[1].BandUV, source[2].BandUV, weights)
	result.LightmapUV = cleanupUV(source[0].LightmapUV, source[1].LightmapUV, source[2].LightmapUV, weights)
	result.WaveUV = cleanupUV(source[0].WaveUV, source[1].WaveUV, source[2].WaveUV, weights)
	return result
}

func cleanupUV(a, b, c [2]float32, weights [3]float64) [2]float32 {
	return [2]float32{
		float32(weights[0]*float64(a[0]) + weights[1]*float64(b[0]) + weights[2]*float64(c[0])),
		float32(weights[0]*float64(a[1]) + weights[1]*float64(b[1]) + weights[2]*float64(c[1])),
	}
}
