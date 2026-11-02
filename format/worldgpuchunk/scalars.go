package worldgpuchunk

import "math"

// The byte ABI contains integer words among floats. Explicit ranges avoid
// treating valid flags as NaNs or overlooking hostile floating point inputs.
func validateGPUScalars(payload Payload, checkpoint func() error) error {
	for at := 0; at < len(payload.Vertices); at += VertexStride {
		if at%(VertexStride*1024) == 0 {
			if err := runPayloadCheckpoint(checkpoint); err != nil {
				return err
			}
		}
		vertex := payload.Vertices[at : at+VertexStride]
		for offset := 0; offset < VertexStride; offset += 4 {
			if !finite(float64(readF32(vertex, offset))) {
				return ErrChunk
			}
		}
		if !validStaticVertexFrame(vertex) {
			return ErrChunk
		}
		for axis := range 3 {
			position := float64(readF32(vertex, axis*4))
			if position < payload.Bounds[axis] || position > payload.Bounds[axis+3] {
				return ErrChunk
			}
		}
	}
	for at := 0; at < len(payload.Records); at += RecordStride {
		if at%(RecordStride*64) == 0 {
			if err := runPayloadCheckpoint(checkpoint); err != nil {
				return err
			}
		}
		record := payload.Records[at : at+RecordStride]
		if err := ValidateRecord(record, readU32(record, 280), payload.RGBMRange, 511); err != nil {
			return err
		}
	}

	return nil
}

// ValidateRecord admits one complete changed metadata record before any writes.
// MaterialLayer is the canonical root array index, not an authored material ID.
// ProbeCount is the admitted immutable root probe table length (at most 511).
func ValidateRecord(record []byte, materialLayer uint32, rgbmRange float64, probeCount uint32) error {
	if len(record) != RecordStride || materialLayer >= 256 || probeCount > 511 || !finite(rgbmRange) || rgbmRange < 1 ||
		rgbmRange > math.MaxFloat32 {
		return ErrChunk
	}
	for offset := 0; offset < RecordStride; offset += 4 {
		if offset == 108 || offset == 252 || offset >= 272 && offset < 320 {
			continue
		}
		if !finite(float64(readF32(record, offset))) {
			return ErrChunk
		}
	}
	for _, offset := range []int{108, 252, 288, 300, 304, 312, 316} {
		if readU32(record, offset) != 0 {
			return ErrChunk
		}
	}
	if !validRecordTransform(record) {
		return ErrChunk
	}
	water := readF32(record, 332)
	if water != 0 && water != 1 || water == 1 && readU32(record, 296) != 2 {
		return ErrChunk
	}
	flags := readU32(record, 292)
	expected := uint32(1|0x20000000) | (materialLayer << 1)
	if flags & ^uint32(1<<30) != expected || readU32(record, 272) != 0 || readU32(record, 276) != 4 ||
		readU32(record, 280) != materialLayer || readU32(record, 284) != materialLayer || readU32(record, 296) > 2 ||
		readU32(record, 308) > probeCount || float64(readF32(record, 236)) != float64(float32(rgbmRange)) {
		return ErrChunk
	}
	return nil
}

// Validate the supplied transform, without generating its already packed normal
// matrix or modifying a byte. This mirrors the existing shader admission bound.
func validRecordTransform(record []byte) bool {
	for _, offset := range []int{12, 28, 44} {
		if readF32(record, offset) != 0 {
			return false
		}
	}
	if readF32(record, 60) != 1 {
		return false
	}
	a, b, c := float64(readF32(record, 0)), float64(readF32(record, 16)), float64(readF32(record, 32))
	d, e, f := float64(readF32(record, 4)), float64(readF32(record, 20)), float64(readF32(record, 36))
	g, h, i := float64(readF32(record, 8)), float64(readF32(record, 24)), float64(readF32(record, 40))
	determinant := a*(e*i-f*h) - b*(d*i-f*g) + c*(d*h-e*g)
	if math.Abs(determinant) < 1e-12 {
		return false
	}
	sign := float32(1)
	if determinant < 0 {
		sign = -1
	}
	return readF32(record, 76) == sign
}
