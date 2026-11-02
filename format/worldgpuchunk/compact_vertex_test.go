package worldgpuchunk

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

func TestCompactVertexExactFloatFieldsAndMotionOffsets(t *testing.T) {
	t.Parallel()
	vertex := Vertex{
		Position: [3]float32{1, 2, 3}, Normal: [3]float32{4, 5, 6},
		Tangent: [4]float32{7, 8, 9, 10}, UV: [2]float32{11, 12}, LightUV: [2]float32{13, 14},
		BakeTangent: [3]float32{15, 16, 17}, BakeBitangent: [3]float32{18, 19, 20},
		Motion: [4]float32{21, 22, 23, 24},
	}

	encoded := AppendVertex(nil, vertex)
	if VertexStride != 96 || len(encoded) != VertexStride {
		t.Fatal("compact vertex stride", VertexStride, len(encoded))
	}

	for word := range 24 {
		if binary.LittleEndian.Uint32(encoded[word*4:]) != math.Float32bits(float32(word+1)) {
			t.Fatal("vertex field precision/order changed", word)
		}
	}

	if ABI != "karty.webgpu-cell@2" || Feature != "world/gpu-chunks@2" || Schema != "karty.world-gpu-chunks@2" {
		t.Fatal("compact vertex ABI is not independently versioned")
	}
}

func TestCompactWireRejectsLegacyAndUnknownVersions(t *testing.T) {
	t.Parallel()
	payload, err := EncodePayload(payloadFixture())
	if err != nil {
		t.Fatal(err)
	}

	if string(payload[:4]) != "KGP2" || binary.LittleEndian.Uint32(payload[20:24]) != 2 {
		t.Fatal("wrong compact payload header")
	}

	for _, magic := range []string{"KGP1", "KGP3"} {
		encoded := bytes.Clone(payload)
		copy(encoded[:4], magic)
		if _, err := DecodePayload(encoded); err == nil {
			t.Fatal("wrong payload version accepted", magic)
		}
	}

	for _, abi := range []uint32{0, 1, 3} {
		encoded := bytes.Clone(payload)
		binary.LittleEndian.PutUint32(encoded[20:24], abi)
		if _, err := DecodePayload(encoded); err == nil {
			t.Fatal("wrong payload ABI accepted", abi)
		}
	}

	catalog, err := EncodeCatalog(catalogFixture())
	if err != nil {
		t.Fatal(err)
	}

	if string(catalog[:4]) != "KGC2" {
		t.Fatal("wrong compact catalog header")
	}

	for _, magic := range []string{"KGC1", "KGC3"} {
		encoded := bytes.Clone(catalog)
		copy(encoded[:4], magic)
		if _, err := DecodeCatalog(encoded); err == nil {
			t.Fatal("wrong catalog version accepted", magic)
		}
	}
}

func TestCompactAdmissionChecksEveryFloatAndReturnsNoPartialCell(t *testing.T) {
	t.Parallel()
	for offset := 0; offset < VertexStride; offset += 4 {
		payload := payloadFixture()
		// The final vertex must receive the same admission as the first.
		fixturePutF32(payload.Vertices, 2*VertexStride+offset, float32(math.NaN()))
		if _, err := EncodePayload(payload); err == nil {
			t.Fatal("nonfinite final vertex admitted", offset)
		}
	}

	encoded, err := EncodePayload(payloadFixture())
	if err != nil {
		t.Fatal(err)
	}

	// Mutate the borrowed vertex stream after encoding to exercise decoder
	// admission rather than relying only on the producer's validation.
	decoded, err := DecodePayload(encoded)
	if err != nil {
		t.Fatal(err)
	}

	fixturePutF32(decoded.Vertices, len(decoded.Vertices)-4, float32(math.Inf(1)))
	rejected, err := DecodePayload(encoded)
	if err == nil || len(rejected.Vertices) != 0 || len(rejected.Draws) != 0 {
		t.Fatal("malformed final motion scalar returned a partial cell", err)
	}

	for _, length := range []int{164 * 3, VertexStride*3 - 1} {
		payload := payloadFixture()
		payload.Vertices = make([]byte, length)
		if _, err := EncodePayload(payload); err == nil {
			t.Fatal("legacy/misaligned vertex bytes accepted", length)
		}
	}

	if MaxVertices != 262144 || MaxIndices != 1572864 || MaxRecords != 8192 || MaxDraws != 1024 || MaxEncodedSize != 8<<20 {
		t.Fatal("compact ABI changed admission bounds")
	}
}
