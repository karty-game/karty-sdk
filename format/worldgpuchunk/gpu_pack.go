package worldgpuchunk

import (
	"encoding/binary"
	"math"
)

// Vertex matches the versioned shader storage ABI, without host struct padding.
// Build tools fill these fields; runtime cell loaders upload the encoded bytes.
type Vertex struct {
	Position, Normal           [3]float32
	Tangent                    [4]float32
	UV, LightUV                [2]float32
	BakeTangent, BakeBitangent [3]float32
	Motion                     [4]float32
}

// StaticRecord contains the material-dependent values in an immutable cell
// record. Owner and surface metadata are carried separately by RecordRef.
type StaticRecord struct {
	Layer, AlphaMode, Probe uint32
	RGBMRange, Emission     float32
	Water                   bool
	Opacity, Exponent       float32
	Specular                [3]float32
	UVTransform             [4]float32
}

// AppendVertex writes exactly one little-endian, 96-byte source vertex.
func AppendVertex(destination []byte, vertex Vertex) []byte {
	appendFloats := func(values []float32) {
		for _, value := range values {
			destination = binary.LittleEndian.AppendUint32(destination, math.Float32bits(value))
		}
	}
	appendFloats(vertex.Position[:])
	appendFloats(vertex.Normal[:])
	appendFloats(vertex.Tangent[:])
	appendFloats(vertex.UV[:])
	appendFloats(vertex.LightUV[:])
	appendFloats(vertex.BakeTangent[:])
	appendFloats(vertex.BakeBitangent[:])
	appendFloats(vertex.Motion[:])
	return destination
}

// AppendStaticRecord writes an identity-transform RNM record. Its local RNM
// base layer is always zero; a cell owns precisely its three prepared planes.
func AppendStaticRecord(destination []byte, record StaticRecord) ([]byte, error) {
	var words [RecordStride / 4]uint32
	for _, index := range [...]int{0, 5, 10, 15, 16, 19, 21, 26, 28, 29, 30, 31} {
		words[index] = math.Float32bits(1)
	}
	words[59] = math.Float32bits(record.RGBMRange)
	for _, index := range [...]int{64, 65, 66} {
		words[index] = math.Float32bits(.04)
	}
	words[67] = math.Float32bits(32)
	words[69] = 4 // Prepared world RNM detail profile.
	words[70], words[71] = record.Layer, record.Layer
	words[73] = 1 | 0x20000000 | 0x40000000 | record.Layer<<1
	if record.Water {
		words[31] = math.Float32bits(record.Opacity)
		for axis, value := range record.Specular {
			words[64+axis] = math.Float32bits(value)
		}
		words[67] = math.Float32bits(record.Exponent)
		words[73] &= ^uint32(0x40000000)
		words[83] = math.Float32bits(1)
	}
	words[74], words[77] = record.AlphaMode, record.Probe
	words[87] = math.Float32bits(record.Emission)
	for index, value := range record.UVTransform {
		words[88+index] = math.Float32bits(value)
	}
	start := len(destination)
	for _, value := range words {
		destination = binary.LittleEndian.AppendUint32(destination, value)
	}
	if err := ValidateRecord(destination[start:], record.Layer, float64(record.RGBMRange), 511); err != nil {
		return nil, err
	}
	return destination, nil
}
