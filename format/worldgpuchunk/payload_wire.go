package worldgpuchunk

import "github.com/karty-game/karty-sdk/internal/binwire"

// EncodePayload writes metadata and the final GPU byte streams without changing
// any vertex, address, index or record bytes. ValidatePayload admits root refs.
func EncodePayload(payload Payload) ([]byte, error) {
	if err := validatePayloadStructure(payload); err != nil {
		return nil, err
	}
	w := binwire.NewWriter("KGP2", MaxEncodedSize)
	w.U32(payload.CellID)
	w.U32(2)
	w.Raw(payload.MaterialLayoutSHA256[:])
	for _, value := range payload.Bounds {
		w.F64(value)
	}
	w.F64(payload.RGBMRange)
	w.Count(len(payload.RecordRefs), false, MaxRecords)
	for _, ref := range payload.RecordRefs {
		w.U32(ref.MaterialID)
		w.U32(ref.OwnerKind)
		w.U32(ref.OwnerID)
		w.U32(ref.SurfaceKind)
	}
	w.Count(len(payload.Draws), false, MaxDraws)
	for _, draw := range payload.Draws {
		w.U32(draw.FirstIndex)
		w.U32(draw.IndexCount)
		w.U32(draw.Record)
		w.U32(draw.AlphaMode)
		for _, value := range draw.Bounds {
			w.F64(value)
		}
		writeIDs(w, draw.Sectors)
	}
	for _, data := range [][]byte{payload.Vertices, payload.Addresses, payload.Indices, payload.Records} {
		w.Count(len(data), false, MaxEncodedSize)
		w.Raw(data)
	}
	return w.Bytes()
}

// DecodePayload borrows immutable GPU bytes and validates all local references.
// It neither converts vertices nor constructs draw addresses or image pools.
func DecodePayload(encoded []byte) (Payload, error) { return DecodePayloadWithCheckpoint(encoded, nil) }

// DecodePayloadWithCheckpoint performs the same complete admission while
// letting a host cancel or cooperatively yield during bounded scan batches. The
// callback must not mutate encoded bytes; failure never returns a partial cell.
func DecodePayloadWithCheckpoint(encoded []byte, checkpoint func() error) (Payload, error) {
	if err := runPayloadCheckpoint(checkpoint); err != nil {
		return Payload{}, err
	}
	r := binwire.NewReader(encoded, "KGP2", MaxEncodedSize)
	result := Payload{CellID: r.U32()}
	if r.U32() != 2 {
		return Payload{}, ErrChunk
	}
	copy(result.MaterialLayoutSHA256[:], r.Raw(32))
	for i := range result.Bounds {
		result.Bounds[i] = r.F64()
	}
	result.RGBMRange = r.F64()
	count, _ := r.Count(MaxRecords, 16, 16)
	result.RecordRefs = make([]RecordRef, count)
	for i := range result.RecordRefs {
		if i%256 == 0 {
			if err := runPayloadCheckpoint(checkpoint); err != nil {
				return Payload{}, err
			}
		}
		result.RecordRefs[i] = RecordRef{r.U32(), r.U32(), r.U32(), r.U32()}
	}
	count, _ = r.Count(MaxDraws, 68, 96)
	result.Draws = make([]Draw, count)
	for i := range result.Draws {
		if i%64 == 0 {
			if err := runPayloadCheckpoint(checkpoint); err != nil {
				return Payload{}, err
			}
		}
		draw := &result.Draws[i]
		draw.FirstIndex = r.U32()
		draw.IndexCount = r.U32()
		draw.Record = r.U32()
		draw.AlphaMode = r.U32()
		for j := range draw.Bounds {
			draw.Bounds[j] = r.F64()
		}
		draw.Sectors = readIDs(r)
	}
	for _, data := range []*[]byte{&result.Vertices, &result.Addresses, &result.Indices, &result.Records} {
		size, _ := r.Count(MaxEncodedSize, 1, 0)
		*data = r.Raw(size)
	}
	if r.Done() != nil {
		return Payload{}, ErrChunk
	}
	if err := validatePayloadStructureWithCheckpoint(result, checkpoint); err != nil {
		return Payload{}, err
	}
	return result, nil
}
