package worldgpuchunk

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"os"
	"testing"
)

func payloadFixture() Payload {
	p := Payload{CellID: 0, Bounds: [6]float64{0, 0, 0, 1, 1, 0}, MaterialLayoutSHA256: [32]byte{2}, RGBMRange: 1}
	p.Vertices = make([]byte, 3*VertexStride)
	for i, position := range [][3]float32{{0, 0, 0}, {1, 0, 0}, {0, 1, 0}} {
		v := p.Vertices[i*VertexStride:]
		for axis, value := range position {
			fixturePutF32(v, axis*4, value)
		}
		for _, offset := range []int{20, 24, 36, 56, 72} {
			fixturePutF32(v, offset, 1)
		}
		p.Addresses = binary.LittleEndian.AppendUint32(p.Addresses, uint32(i))
		p.Addresses = binary.LittleEndian.AppendUint32(p.Addresses, 0)
		p.Indices = binary.LittleEndian.AppendUint32(p.Indices, uint32(i))
	}
	p.Records = make([]byte, RecordStride)
	for _, offset := range []int{0, 20, 40, 60, 64, 76, 84, 104, 112, 116, 120, 124, 236} {
		fixturePutF32(p.Records, offset, 1)
	}
	binary.LittleEndian.PutUint32(p.Records[276:], 4)
	binary.LittleEndian.PutUint32(p.Records[292:], 1|0x20000000)
	p.RecordRefs = []RecordRef{{MaterialID: 99, OwnerKind: OwnerSector, OwnerID: 0}}
	p.Draws = []Draw{{IndexCount: 3, Bounds: p.Bounds, Sectors: []uint32{0}}}
	return p
}
func fixturePutF32(data []byte, offset int, value float32) {
	binary.LittleEndian.PutUint32(data[offset:], math.Float32bits(value))
}

func TestPayloadCanonicalBorrowedGPUBytesAndRootReferences(t *testing.T) {
	t.Parallel()
	p := payloadFixture()
	if err := ValidatePayload(p, []uint32{99}, 1, 0); err != nil {
		t.Fatal(err)
	}
	encoded, err := EncodePayload(p)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodePayload(encoded)
	if err != nil {
		t.Fatal(err)
	}
	again, err := EncodePayload(decoded)
	if err != nil || !bytes.Equal(encoded, again) {
		t.Fatal("noncanonical payload", err)
	}
	if !bytes.Equal(decoded.Vertices, p.Vertices) || !bytes.Equal(decoded.Addresses, p.Addresses) ||
		!bytes.Equal(decoded.Records, p.Records) {
		t.Fatal("GPU bytes transformed")
	}
	decoded.Vertices[0] = 23
	if !bytes.Contains(encoded, decoded.Vertices) {
		t.Fatal("decoder copied immutable GPU blob")
	}
	if err := ValidatePayload(p, []uint32{100}, 1, 0); err == nil {
		t.Fatal("material ID replaced by array layer")
	}
}

func TestPayloadRejectsMalformedFinalBuffers(t *testing.T) {
	t.Parallel()
	cases := map[string]func(*Payload){
		"vertex stride":      func(p *Payload) { p.Vertices = p.Vertices[:len(p.Vertices)-1] },
		"vertex NaN":         func(p *Payload) { fixturePutF32(p.Vertices, 12, float32(math.NaN())) },
		"vertex bounds":      func(p *Payload) { fixturePutF32(p.Vertices, 0, 2) },
		"foreign vertex":     func(p *Payload) { binary.LittleEndian.PutUint32(p.Addresses, 3) },
		"foreign record":     func(p *Payload) { binary.LittleEndian.PutUint32(p.Addresses[4:], 1) },
		"foreign address":    func(p *Payload) { binary.LittleEndian.PutUint32(p.Indices, 3) },
		"draw gap":           func(p *Payload) { p.Draws[0].FirstIndex = 1 },
		"blended draw":       func(p *Payload) { p.Draws[0].AlphaMode = 3 },
		"singular transform": func(p *Payload) { fixturePutF32(p.Records, 0, 0) },
		"bad handedness":     func(p *Payload) { fixturePutF32(p.Records, 76, 0) },
		"clip state":         func(p *Payload) { binary.LittleEndian.PutUint32(p.Records[316:], 1) },
		"portal state":       func(p *Payload) { binary.LittleEndian.PutUint32(p.Records[308:], 512) },
		"array layer":        func(p *Payload) { binary.LittleEndian.PutUint32(p.Records[280:], 99) },
		"foreign owner":      func(p *Payload) { p.RecordRefs[0].OwnerID = 1 },
		"surface kind":       func(p *Payload) { p.RecordRefs[0].SurfaceKind = 3 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p := payloadFixture()
			mutate(&p)
			if ValidatePayload(p, []uint32{99}, 1, 0) == nil {
				t.Fatal("malformed GPU data admitted")
			}
		})
	}
}

func TestPayloadRejectsTruncationTrailingAndHostileCount(t *testing.T) {
	t.Parallel()
	encoded, err := EncodePayload(payloadFixture())
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []int{0, 15, 100, len(encoded) - 1} {
		if _, err := DecodePayload(encoded[:size]); err == nil {
			t.Fatal("truncation admitted", size)
		}
	}
	if _, err := DecodePayload(append(bytes.Clone(encoded), 0)); err == nil {
		t.Fatal("trailing bytes admitted")
	}
	binary.LittleEndian.PutUint32(encoded[112:], math.MaxUint32)
	if _, err := DecodePayload(encoded); err == nil {
		t.Fatal("unbounded typed allocation admitted")
	}
}

func TestCellRNMIsIndependentThreeLayerPreparedImage(t *testing.T) {
	t.Parallel()
	p := payloadFixture()
	image, err := os.ReadFile("../../codec/ktx2/testdata/linear-rgba8-array.ktx2")
	if err != nil {
		t.Fatal(err)
	}
	c := catalogFixture()
	if err := c.ValidateCell(0, p, image); err != nil {
		t.Fatal(err)
	}
	c.Cells[0].RNMBytes++
	if err := c.ValidateCell(0, p, image); err == nil {
		t.Fatal("incorrect decoded texture budget admitted")
	}
}

func TestRecordDeltaValidationNeedsOnlyOwnStride(t *testing.T) {
	t.Parallel()
	record := payloadFixture().Records
	if err := ValidateRecord(record, 0, 1, 0); err != nil {
		t.Fatal(err)
	}
	binary.LittleEndian.PutUint32(record[308:], 1)
	if ValidateRecord(record, 0, 1, 0) == nil {
		t.Fatal("foreign probe admitted")
	}
	if ValidateRecord(record, 0, 1, 1) != nil {
		t.Fatal("admitted fixed probe rejected")
	}
}

func TestChunkDrawBatchesIndependentOpaqueAndCutoutRecordsWithoutReindexing(t *testing.T) {
	t.Parallel()
	p := payloadFixture()
	p.Records = append(p.Records, bytes.Clone(p.Records)...)
	binary.LittleEndian.PutUint32(p.Records[RecordStride+296:], 1)
	p.RecordRefs = append(p.RecordRefs, RecordRef{MaterialID: 99, OwnerKind: OwnerSector, OwnerID: 0, SurfaceKind: 1})
	for vertex := range 3 {
		p.Addresses = binary.LittleEndian.AppendUint32(p.Addresses, uint32(vertex))
		p.Addresses = binary.LittleEndian.AppendUint32(p.Addresses, 1)
		p.Indices = binary.LittleEndian.AppendUint32(p.Indices, uint32(vertex+3))
	}
	p.Draws[0].IndexCount = 6
	if err := ValidatePayload(p, []uint32{99}, 2, 0); err != nil {
		t.Fatal("same-owner static batch rejected", err)
	}
	encoded, err := EncodePayload(p)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodePayload(encoded)
	if err != nil || !bytes.Equal(decoded.Addresses, p.Addresses) {
		t.Fatal("owner batch bytes changed", err)
	}
	p.RecordRefs[1].OwnerID = 1
	if err := ValidatePayload(p, []uint32{99}, 2, 0); err != nil {
		t.Fatal("opaque chunk batch rejected independent per-address owner", err)
	}
	binary.LittleEndian.PutUint32(p.Records[RecordStride+308:], 2)
	if err := ValidatePayload(p, []uint32{99}, 2, 0); err != nil {
		t.Fatal("opaque chunk batch rejected independent per-record probe", err)
	}
	p.RecordRefs[1].OwnerID = 2
	if ValidatePayload(p, []uint32{99}, 2, 0) == nil {
		t.Fatal("chunk batching admitted an owner outside the root table")
	}
	p.RecordRefs[1].OwnerID = 1
	p.Draws[0].AlphaMode = 2
	for index := range 2 {
		binary.LittleEndian.PutUint32(p.Records[index*RecordStride+296:], 2)
	}
	if ValidatePayload(p, []uint32{99}, 2, 0) == nil {
		t.Fatal("blended chunk batch crossed independently ordered owners")
	}
	p.RecordRefs[1].OwnerID = 0
	if err := ValidatePayload(p, []uint32{99}, 2, 0); err != nil {
		t.Fatal("single-owner blended batch was rejected", err)
	}
}

func TestPayloadCheckpointCancellationNeverReturnsPartialBuffers(t *testing.T) {
	t.Parallel()
	encoded, err := EncodePayload(payloadFixture())
	if err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("cancel admission")
	calls := 0
	payload, err := DecodePayloadWithCheckpoint(encoded, func() error {
		calls++
		if calls == 3 {
			return sentinel
		}
		return nil
	})
	if !errors.Is(err, sentinel) || len(payload.Vertices) != 0 || calls != 3 {
		t.Fatal("checkpoint cancellation lost atomicity", calls, err)
	}
}

func TestPayloadStaticVertexFramesMatchExistingRendererAdmission(t *testing.T) {
	t.Parallel()
	for _, mutate := range []func([]byte){func(v []byte) { fixturePutF32(v, 20, 2) }, func(v []byte) { fixturePutF32(v, 24, 2) }, func(v []byte) { fixturePutF32(v, 32, 1) }, func(v []byte) { fixturePutF32(v, 36, 0) }, func(v []byte) { fixturePutF32(v, 80, 3) }} {
		payload := payloadFixture()
		mutate(payload.Vertices)
		if _, err := EncodePayload(payload); err == nil {
			t.Fatal("invalid static vertex frame admitted")
		}
	}
}

func TestPreparedLiquidRecordsPreserveStrictBlendAndWaveProfile(t *testing.T) {
	t.Parallel()
	p := payloadFixture()
	binary.LittleEndian.PutUint32(p.Records[296:], 2)
	fixturePutF32(p.Records, 332, 1)
	fixturePutF32(p.Vertices, 80, 2)
	p.Draws[0].AlphaMode = 2
	if err := ValidatePayload(p, []uint32{99}, 1, 0); err != nil {
		t.Fatal("prepared liquid rejected", err)
	}
	p.Draws[0].AlphaMode = 0
	if ValidatePayload(p, []uint32{99}, 1, 0) == nil {
		t.Fatal("blended liquid entered opaque owner batch")
	}
	p.Draws[0].AlphaMode = 2
	fixturePutF32(p.Records, 332, 2)
	if ValidatePayload(p, []uint32{99}, 1, 0) == nil {
		t.Fatal("unknown liquid flag admitted")
	}
}
