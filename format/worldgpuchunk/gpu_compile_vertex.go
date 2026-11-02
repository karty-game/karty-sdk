package worldgpuchunk

import (
	"math"

	"github.com/karty-game/karty-sdk/format/worldchunk"
)

func compileVertex(batch worldchunk.ReadyBatch, vertexID int, band bool, tangent [4]float32) (Vertex, error) {
	if vertexID < 0 || vertexID >= len(batch.Vertices)/worldchunk.ReadyStride {
		return Vertex{}, ErrChunk
	}
	source := batch.Vertices[vertexID*worldchunk.ReadyStride : (vertexID+1)*worldchunk.ReadyStride]
	if !batch.StaticMaterial && (source[18] != 1 || source[19] != 0 || source[20] != 0) {
		return Vertex{}, ErrChunk
	}
	vertex := Vertex{
		Position: [3]float32{source[0], source[1], source[2]},
		Normal:   [3]float32{source[3], source[4], source[5]},
		UV:       [2]float32{source[6], source[7]},
		Tangent:  tangent,
	}
	if band {
		vertex.UV = [2]float32{source[24], source[25]}
	}
	if len(batch.MaterialBasis) > 0 {
		offset := vertexID * 8
		if band {
			offset += 4
		}
		if offset+4 > len(batch.MaterialBasis) {
			return Vertex{}, ErrChunk
		}
		copy(vertex.Tangent[:], batch.MaterialBasis[offset:offset+4])
	}
	if len(batch.LightmapUV) > 0 {
		if vertexID*2+2 > len(batch.LightmapUV) || vertexID*6+6 > len(batch.LightmapBasis) {
			return Vertex{}, ErrChunk
		}
		copy(vertex.LightUV[:], batch.LightmapUV[vertexID*2:vertexID*2+2])
		copy(vertex.BakeTangent[:], batch.LightmapBasis[vertexID*6:vertexID*6+3])
		copy(vertex.BakeBitangent[:], batch.LightmapBasis[vertexID*6+3:vertexID*6+6])
	}
	return vertex, nil
}

func compileTangent(batch worldchunk.ReadyBatch, first int, band bool) ([4]float32, error) {
	if len(batch.MaterialBasis) > 0 {
		return [4]float32{}, nil
	}
	var positions [3][3]float64
	var coordinates [3][2]float64
	var normal [3]float64
	for corner := range 3 {
		id := batch.Indices[first+corner]
		if uint64(id) >= uint64(len(batch.Vertices)/worldchunk.ReadyStride) {
			return [4]float32{}, ErrChunk
		}
		source := batch.Vertices[int(id)*worldchunk.ReadyStride : (int(id)+1)*worldchunk.ReadyStride]
		positions[corner] = [3]float64{float64(source[0]), float64(source[1]), float64(source[2])}
		normal = [3]float64{float64(source[3]), float64(source[4]), float64(source[5])}
		offset := 6
		if band {
			offset = 24
		}
		coordinates[corner] = [2]float64{float64(source[offset]), float64(source[offset+1])}
	}
	edge1, edge2 := gpuSubtract(positions[1], positions[0]), gpuSubtract(positions[2], positions[0])
	du1, dv1 := coordinates[1][0]-coordinates[0][0], coordinates[1][1]-coordinates[0][1]
	du2, dv2 := coordinates[2][0]-coordinates[0][0], coordinates[2][1]-coordinates[0][1]
	determinant := du1*dv2 - du2*dv1
	axis := [3]float64{0, 0, 1}
	if math.Abs(normal[2]) > .9 {
		axis = [3]float64{0, 1, 0}
	}
	tangent := gpuUnit(gpuCross(axis, normal))
	handedness := float32(1)
	if math.Abs(determinant) > 1e-12 {
		for component := range 3 {
			tangent[component] = (edge1[component]*dv2 - edge2[component]*dv1) / determinant
		}
		tangent = gpuUnit(tangent)
		bitangent := [3]float64{}
		for component := range 3 {
			bitangent[component] = (edge2[component]*du1 - edge1[component]*du2) / determinant
		}
		cross := gpuCross(normal, tangent)
		if cross[0]*bitangent[0]+cross[1]*bitangent[1]+cross[2]*bitangent[2] < 0 {
			handedness = -1
		}
	}
	return [4]float32{float32(tangent[0]), float32(tangent[1]), float32(tangent[2]), handedness}, nil
}

func gpuSubtract(a, b [3]float64) [3]float64 {
	return [3]float64{a[0] - b[0], a[1] - b[1], a[2] - b[2]}
}
func gpuCross(a, b [3]float64) [3]float64 {
	return [3]float64{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}
func gpuUnit(value [3]float64) [3]float64 {
	length := math.Sqrt(value[0]*value[0] + value[1]*value[1] + value[2]*value[2])
	if length < 1e-12 {
		return [3]float64{}
	}
	return [3]float64{value[0] / length, value[1] / length, value[2] / length}
}
