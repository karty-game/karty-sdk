package character

import (
	"math"
	"unicode/utf8"

	"github.com/karty-game/karty-sdk/codec/qoi"
)

func name(value string) bool {
	return len(value) > 0 && len(value) <= MaxName && utf8.ValidString(value)
}
func finite(value float32) bool {
	return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0) && math.Abs(float64(value)) <= 1e6
}
func numbers(values []float32) bool {
	for _, value := range values {
		if !finite(value) {
			return false
		}
	}
	return true
}

func (document Document) Validate() error {
	if len(document.Models) == 0 || len(document.Models) > MaxModels || len(document.Banks) == 0 || len(document.Banks) > MaxBanks {
		return ErrInvalid
	}
	for index, bank := range document.Banks {
		if !name(bank.ID) || len(bank.Clips) == 0 || len(bank.Clips) > MaxClips {
			return ErrInvalid
		}
		for previous := range index {
			if document.Banks[previous].ID == bank.ID {
				return ErrInvalid
			}
		}
		if err := validateBank(bank); err != nil {
			return err
		}
	}
	for index := range document.Models {
		model := &document.Models[index]
		if !name(model.ID) || model.Bank < 0 || model.Bank >= len(document.Banks) {
			return ErrInvalid
		}
		for previous := range index {
			if document.Models[previous].ID == model.ID {
				return ErrInvalid
			}
		}
		if err := validateModel(model, document.Banks[model.Bank]); err != nil {
			return err
		}
	}
	if encodedSize(document) > MaxBytes {
		return ErrInvalid
	}
	return nil
}

func validateBank(bank Bank) error {
	keys := 0
	for index, clip := range bank.Clips {
		if !name(clip.Name) || !finite(clip.Duration) || clip.Duration <= 0 || clip.Duration > 600 || len(clip.Tracks) > MaxTracks {
			return ErrInvalid
		}
		for previous := range index {
			if bank.Clips[previous].Name == clip.Name {
				return ErrInvalid
			}
		}
		seen := make(map[string]uint8, len(clip.Tracks))
		for _, track := range clip.Tracks {
			dimension := 3
			if track.Path == Rotation {
				dimension = 4
			}
			if !name(track.Node) || track.Path > Scale || len(track.Times) == 0 || len(track.Times) > MaxKeys-keys ||
				len(track.Values) != len(track.Times)*dimension ||
				!numbers(track.Times) ||
				!numbers(track.Values) {
				return ErrInvalid
			}
			mask := uint8(1) << track.Path
			if seen[track.Node]&mask != 0 {
				return ErrInvalid
			}
			seen[track.Node] |= mask
			for key, value := range track.Times {
				if value < 0 || value > clip.Duration+1e-4 || key > 0 && value <= track.Times[key-1] {
					return ErrInvalid
				}
				values := track.Values[key*dimension : (key+1)*dimension]
				if track.Path == Rotation && !unitQuaternion(values) {
					return ErrInvalid
				}
				if track.Path == Scale {
					for _, scale := range values {
						if scale <= 0 {
							return ErrInvalid
						}
					}
				}
			}
			keys += len(track.Times)
		}
	}
	return nil
}

func unitQuaternion(values []float32) bool {
	sum := float32(0)
	for _, value := range values {
		sum += value * value
	}
	return math.Abs(float64(sum-1)) < .005
}

func validateModel(model *Model, bank Bank) error {
	if len(model.Nodes) == 0 || len(model.Nodes) > MaxNodes || model.MeshNode < 0 || model.MeshNode >= len(model.Nodes) ||
		len(model.Joints) == 0 ||
		len(model.Joints) > MaxJoints ||
		len(model.InverseBind) != len(model.Joints)*16 ||
		!numbers(model.InverseBind) {
		return ErrInvalid
	}
	if model.Width < 1 || model.Height < 1 || model.Width > MaxTextureSize || model.Height > MaxTextureSize ||
		len(model.AlbedoQOI) > MaxImageBytes {
		return ErrInvalid
	}
	info, err := qoi.Inspect(model.AlbedoQOI)
	if err != nil || int(info.Width) != model.Width || int(info.Height) != model.Height || info.Colorspace != qoi.ColorspaceSRGB {
		return ErrInvalid
	}
	if _, err = qoi.Validate(model.AlbedoQOI); err != nil {
		return ErrInvalid
	}
	names := make(map[string]int, len(model.Nodes))
	for index, node := range model.Nodes {
		if !name(node.Name) || node.Parent < -1 || node.Parent >= len(model.Nodes) || !numbers(node.Translation[:]) ||
			!numbers(node.Rotation[:]) ||
			!numbers(node.Scale[:]) ||
			!unitQuaternion(node.Rotation[:]) {
			return ErrInvalid
		}
		if _, exists := names[node.Name]; exists {
			return ErrInvalid
		}
		names[node.Name] = index
		for _, scale := range node.Scale {
			if math.Abs(float64(scale)) < 1e-8 {
				return ErrInvalid
			}
		}
		current := index
		for depth := 0; current >= 0; depth++ {
			if depth >= len(model.Nodes) || current >= len(model.Nodes) {
				return ErrInvalid
			}
			current = model.Nodes[current].Parent
		}
	}
	for _, clip := range bank.Clips {
		for _, track := range clip.Tracks {
			if _, exists := names[track.Node]; !exists {
				return ErrInvalid
			}
		}
	}
	for index, joint := range model.Joints {
		if int(joint) >= len(model.Nodes) {
			return ErrInvalid
		}
		for previous := range index {
			if model.Joints[previous] == joint {
				return ErrInvalid
			}
		}
		matrix := model.InverseBind[index*16 : (index+1)*16]
		if math.Abs(float64(matrix[3])) > 1e-5 || math.Abs(float64(matrix[7])) > 1e-5 || math.Abs(float64(matrix[11])) > 1e-5 ||
			math.Abs(float64(matrix[15]-1)) > 1e-5 {
			return ErrInvalid
		}
		determinant := float64(
			matrix[0],
		)*(float64(matrix[5])*float64(matrix[10])-float64(matrix[9])*float64(matrix[6])) - float64(
			matrix[4],
		)*(float64(matrix[1])*float64(matrix[10])-float64(matrix[9])*float64(matrix[2])) + float64(
			matrix[8],
		)*(float64(matrix[1])*float64(matrix[6])-float64(matrix[5])*float64(matrix[2]))
		if math.Abs(determinant) < 1e-12 {
			return ErrInvalid
		}
	}
	if len(model.Vertices) == 0 || len(model.Vertices)%VertexStride != 0 || len(model.Vertices)/VertexStride > MaxVertices ||
		len(model.Indices) == 0 ||
		len(model.Indices) > MaxIndices ||
		len(model.Indices)%3 != 0 ||
		!numbers(model.Vertices) {
		return ErrInvalid
	}
	for at := 0; at < len(model.Vertices); at += VertexStride {
		vertex := model.Vertices[at : at+VertexStride]
		if vertex[3]*vertex[3]+vertex[4]*vertex[4]+vertex[5]*vertex[5] < 1e-10 {
			return ErrInvalid
		}
		sum := float32(0)
		for influence := range 4 {
			joint, weight := vertex[8+influence], vertex[12+influence]
			if joint < 0 || joint >= float32(len(model.Joints)) || joint != float32(int(joint)) || weight < 0 || weight > 1 {
				return ErrInvalid
			}
			sum += weight
		}
		if math.Abs(float64(sum-1)) > .001 {
			return ErrInvalid
		}
	}
	for _, index := range model.Indices {
		if uint64(index) >= uint64(len(model.Vertices)/VertexStride) {
			return ErrInvalid
		}
	}
	return validateHierarchyEnvelope(model, bank, names)
}
