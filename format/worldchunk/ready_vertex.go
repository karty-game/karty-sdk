package worldchunk

import "math"

func readyProjection(count uint8, weights [3]float32) (int, bool) {
	if count <= 1 {
		return 0, true
	}
	active := -1
	for index, weight := range weights {
		if weight == 0 {
			continue
		}
		if active >= 0 || float64(weight) <= .999999 || weight > 1 || index >= int(count) {
			return 0, false
		}
		active = index
	}
	return max(active, 0), active >= 0
}

func readyPackedBasis(normal [3]float32, basis Basis) ([4]float32, bool) {
	t, b := basis.Tangent, basis.Bitangent
	handed := float64(normal[0])*(float64(t[1])*float64(b[2])-float64(t[2])*float64(b[1])) +
		float64(normal[1])*(float64(t[2])*float64(b[0])-float64(t[0])*float64(b[2])) +
		float64(normal[2])*(float64(t[0])*float64(b[1])-float64(t[1])*float64(b[0]))
	if math.IsNaN(handed) || math.IsInf(handed, 0) || math.Abs(handed) < .000001 {
		return [4]float32{}, false
	}
	sign := float32(1)
	if handed < 0 {
		sign = -1
	}
	return [4]float32{t[0], t[1], t[2], sign}, true
}

func readyMaterialBasis(triangle Triangle, materials map[uint32]readyMaterial) ([8]float32, int, int, bool) {
	var result [8]float32
	primary, valid := readyProjection(triangle.MappingCount, triangle.Weights)
	if !valid || materials[triangle.Material].liquid {
		return result, 0, 0, false
	}
	secondary := 0
	if triangle.Secondary.Strength > 0 {
		secondary, valid = readyProjection(triangle.Secondary.MappingCount, triangle.Secondary.Weights)
		if !valid || materials[triangle.Secondary.Material].liquid {
			return result, 0, 0, false
		}
	}
	basis := triangle.ProjectionBases[primary]
	if triangle.MappingCount == 0 {
		basis = Basis{Tangent: triangle.Tangent, Bitangent: triangle.Bitangent}
	}
	packed, valid := readyPackedBasis(triangle.Normal, basis)
	if !valid {
		return result, 0, 0, false
	}
	copy(result[:4], packed[:])
	if triangle.Band.Coverage != 0 {
		if materials[triangle.Band.Material].liquid {
			return result, 0, 0, false
		}
		packed, valid = readyPackedBasis(triangle.Normal, triangle.Band.Basis)
		if !valid {
			return result, 0, 0, false
		}
		copy(result[4:], packed[:])
	}
	return result, primary, secondary, true
}

func readyVertex(vertex Vertex, triangle Triangle, primary, secondary int, static bool) [ReadyStride]float32 {
	result := [ReadyStride]float32{
		vertex.Position[0],
		vertex.Position[1],
		vertex.Position[2],
		triangle.Normal[0],
		triangle.Normal[1],
		triangle.Normal[2],
	}
	uv, detail := vertex.UV, vertex.SecondaryUV
	weights, detailWeights := triangle.Weights, triangle.Secondary.Weights
	if static {
		uv[0], detail[0] = uv[primary], detail[secondary]
	}
	if static || triangle.MappingCount <= 1 {
		weights = [3]float32{1, 0, 0}
	}
	if static || triangle.Secondary.MappingCount <= 1 {
		detailWeights = [3]float32{1, 0, 0}
	}
	for index := range 3 {
		copy(result[6+index*2:8+index*2], uv[index][:])
		copy(result[12+index*2:14+index*2], detail[index][:])
	}
	copy(result[18:21], weights[:])
	copy(result[21:24], detailWeights[:])
	copy(result[24:26], vertex.BandUV[:])
	return result
}

func readyWinding(mesh *Mesh, index int) int8 {
	var edges [2][3]float64
	for axis := range 3 {
		origin := mesh.Vertices[index*3].Position[axis]
		for edge := range 2 {
			edges[edge][axis] = float64(mesh.Vertices[index*3+edge+1].Position[axis]) - float64(origin)
		}
	}
	a, b := edges[0], edges[1]
	cross := [3]float64{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
	var dot, area, length float64
	for axis, value := range mesh.Triangles[index].Normal {
		normal := float64(value)
		dot += cross[axis] * normal
		area += cross[axis] * cross[axis]
		length += normal * normal
	}
	if area == 0 || length == 0 || dot*dot < area*length*(1-1e-12) {
		return 0
	}
	if dot < 0 {
		return -1
	}
	return 1
}
