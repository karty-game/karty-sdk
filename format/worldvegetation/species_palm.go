//nolint:mnd // Immutable normalized palm template and fixed vertex layout.
package worldvegetation

import "math"

const palmTriangles = 432 // Eight six-sided trunk segments, fourteen six-section folded fronds.

// Palms have one curved leader and connected arched fronds. This shared mesh is
// prepared once; roots, scale, yaw and wind still come from the GPU layer seed.
func builtInPalmVertices() []float32 {
	mesh := shrubMeshBuilder{vertices: make([]float32, 0, palmTriangles*3*shrubVertexFloats)}

	const segments = 8
	for segment := range segments {
		fraction, next := float32(segment)/segments, float32(segment+1)/segments
		start, end := palmLeader(fraction), palmLeader(next)
		first := len(mesh.vertices)
		mesh.branch(start, end, .026-.014*fraction, .026-.014*next, segment)
		// Continuous cylindrical bark, rather than one constant texel per face.
		for side := range 6 {
			uv := [6][2]float32{{float32(side) / 6, .94}, {float32(side+1) / 6, .94}, {float32(side+1) / 6, .56},
				{float32(side) / 6, .94}, {float32(side+1) / 6, .56}, {float32(side) / 6, .56}}
			for corner, coordinate := range uv {
				at := first + (side*6+corner)*shrubVertexFloats
				mesh.vertices[at+6], mesh.vertices[at+7] = coordinate[0], coordinate[1]
			}
		}
	}

	for frond := range 14 {
		yaw := float64(frond)*2.39996323 + float64(speciesVariation(frond, 4))*.25
		reach := .36 + .045*speciesVariation(frond, 5)
		rise := .10 + .065*speciesVariation(frond, 6)

		droop := .12 + .085*speciesVariation(frond, 7)
		if frond >= 10 {
			reach, rise, droop = .20+.05*speciesVariation(frond, 8), .18, -.035
		}

		mesh.palmFrond(yaw, reach, rise, droop, frond)
	}

	return mesh.vertices
}

func palmLeader(fraction float32) [3]float32 {
	return [3]float32{.067 * fraction * fraction, -.022 * float32(math.Sin(float64(fraction)*math.Pi)), .006 + .785*fraction}
}

func (mesh *shrubMeshBuilder) palmFrond(yaw float64, reach, rise, droop float32, index int) {
	base := palmLeader(1)
	radial := [3]float32{float32(math.Cos(yaw)), float32(math.Sin(yaw)), 0}
	right := [3]float32{-radial[1], radial[0], 0}

	const sections = 6
	for section := range sections {
		var vertices [6][shrubVertexFloats]float32

		for end := range 2 {
			fraction := float32(section+end) / sections
			arch := float32(math.Sin(float64(fraction) * math.Pi))
			derivativeZ := rise*float32(math.Cos(float64(fraction)*math.Pi))*math.Pi - 2*droop*fraction
			length := float32(math.Hypot(float64(reach), float64(derivativeZ)))

			for across := range 3 {
				horizontal := float32(across) / 2

				fold := float32(0)
				if across == 1 {
					fold = .013 * arch
				}

				width := .15 + .015*speciesVariation(index, 9)
				position := [3]float32{base[0] + radial[0]*reach*fraction + right[0]*(horizontal-.5)*width,
					base[1] + radial[1]*reach*fraction + right[1]*(horizontal-.5)*width,
					base[2] + rise*arch - droop*fraction*fraction + fold}
				vertex := &vertices[end*3+across]
				*vertex = [shrubVertexFloats]float32{position[0], position[1], position[2],
					-radial[0] * derivativeZ / length, -radial[1] * derivativeZ / length, reach / length,
					.01 + .98*horizontal, .495 - .49*fraction,
					right[0], right[1], 0, -radial[0] * reach / length, -radial[1] * reach / length, -derivativeZ / length,
					.73 + .23*fraction}
			}
		}

		for _, corner := range [...]int{0, 1, 4, 0, 4, 3, 1, 2, 5, 1, 5, 4} {
			mesh.vertices = append(mesh.vertices, vertices[corner][:]...)
		}
	}
}
