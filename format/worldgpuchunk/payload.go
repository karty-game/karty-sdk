package worldgpuchunk

import (
	"encoding/binary"
	"math"
	"slices"

	"github.com/karty-game/karty-sdk/codec/ktx2"
)

func validatePayloadStructure(payload Payload) error {
	return validatePayloadStructureWithCheckpoint(payload, nil)
}

func validatePayloadStructureWithCheckpoint(payload Payload, checkpoint func() error) error {
	if payload.CellID >= MaxCells || payload.MaterialLayoutSHA256 == [32]byte{} || !validBounds(payload.Bounds) ||
		!finite(payload.RGBMRange) || payload.RGBMRange < 1 || payload.RGBMRange > math.MaxFloat32 ||
		len(payload.Vertices) == 0 || len(payload.Vertices)%VertexStride != 0 || len(payload.Vertices)/VertexStride > MaxVertices ||
		len(payload.Addresses) == 0 || len(payload.Addresses)%AddressStride != 0 || len(payload.Addresses)/AddressStride > MaxIndices ||
		len(payload.Indices) == 0 || len(payload.Indices)%IndexStride != 0 || len(payload.Indices)/IndexStride > MaxIndices ||
		len(
			payload.Records,
		) == 0 || len(payload.Records)%RecordStride != 0 || len(payload.RecordRefs) != len(payload.Records)/RecordStride ||
		len(payload.RecordRefs) > MaxRecords || len(payload.Draws) == 0 || len(payload.Draws) > MaxDraws {
		return ErrChunk
	}
	for _, ref := range payload.RecordRefs {
		if ref.OwnerKind > OwnerSolid || ref.SurfaceKind > 2 {
			return ErrChunk
		}
	}
	total := uint64(len(payload.Vertices)) + uint64(len(payload.Addresses)) + uint64(len(payload.Indices)) + uint64(len(payload.Records))
	if total > MaxEncodedSize {
		return ErrChunk
	}
	if err := validateGPUScalars(payload, checkpoint); err != nil {
		return err
	}
	return validateDrawReferences(payload, checkpoint)
}

// ValidatePayload additionally binds owners and canonical shared material layers
// to admitted root metadata. The root layout order is the material layer order.
func ValidatePayload(payload Payload, materialIDs []uint32, sectorCount, solidCount int) error {
	if err := validatePayloadStructure(payload); err != nil {
		return err
	}
	if len(materialIDs) == 0 || len(materialIDs) > 256 || sectorCount < 1 || solidCount < 0 {
		return ErrChunk
	}
	for index, ref := range payload.RecordRefs {
		layer := slices.Index(materialIDs, ref.MaterialID)
		limit := sectorCount
		if ref.OwnerKind == OwnerSolid {
			limit = solidCount
		} else if ref.OwnerKind != OwnerSector {
			return ErrChunk
		}
		if layer < 0 || ref.SurfaceKind > 2 || uint64(ref.OwnerID) >= uint64(limit) {
			return ErrChunk
		}
		record := payload.Records[index*RecordStride : (index+1)*RecordStride]
		if err := ValidateRecord(record, uint32(layer), payload.RGBMRange, 511); err != nil {
			return err
		}
	}
	for _, draw := range payload.Draws {
		for _, sector := range draw.Sectors {
			if uint64(sector) >= uint64(sectorCount) {
				return ErrChunk
			}
		}
		ref := payload.RecordRefs[draw.Record]
		if ref.OwnerKind == OwnerSector && !slices.Contains(draw.Sectors, ref.OwnerID) {
			return ErrChunk
		}
	}
	return nil
}

// ValidateCell binds the complete payload and its independently prepared RNM
// image to one content-addressed root descriptor. Module hashing stays with the
// level loader because the descriptor identifies the entire KLD, not its image.
func (catalog Catalog) ValidateCell(id uint32, payload Payload, rnm []byte) error {
	if len(rnm) > MaxEncodedSize || uint64(id) >= uint64(len(catalog.Cells)) {
		return ErrChunk
	}
	cell := catalog.Cells[id]
	if cell.ID != id || payload.CellID != id || payload.MaterialLayoutSHA256 != catalog.MaterialLayoutSHA256 ||
		payload.Bounds != cell.Bounds ||
		validatePayloadStructure(payload) != nil {
		return ErrChunk
	}
	total := uint64(len(payload.Vertices)) + uint64(len(payload.Addresses)) + uint64(len(payload.Indices)) + uint64(len(payload.Records))
	if total != cell.GPUBytes {
		return ErrChunk
	}
	info, err := ktx2.InspectRGBA8(rnm)
	if err != nil || info.Layers != 3 || info.Width > MaxDimension || info.Height > MaxDimension || info.RGBABytes != cell.RNMBytes {
		return ErrChunk
	}
	for _, draw := range payload.Draws {
		for _, sector := range draw.Sectors {
			if !slices.Contains(cell.Sectors, sector) {
				return ErrChunk
			}
		}
	}
	return nil
}

func validateDrawReferences(payload Payload, checkpoint func() error) error {
	vertices, addresses, records := len(
		payload.Vertices,
	)/VertexStride, len(
		payload.Addresses,
	)/AddressStride, len(
		payload.Records,
	)/RecordStride
	for offset := 0; offset < len(payload.Addresses); offset += AddressStride {
		if offset%(AddressStride*8192) == 0 {
			if err := runPayloadCheckpoint(checkpoint); err != nil {
				return err
			}
		}
		if uint64(readU32(payload.Addresses, offset)) >= uint64(vertices) ||
			uint64(readU32(payload.Addresses, offset+4)) >= uint64(records) {
			return ErrChunk
		}
	}
	next := uint64(0)
	for _, draw := range payload.Draws {
		end := uint64(draw.FirstIndex) + uint64(draw.IndexCount)
		if uint64(draw.FirstIndex) != next || draw.IndexCount == 0 || draw.IndexCount%3 != 0 || end > uint64(len(payload.Indices)/4) ||
			uint64(
				draw.Record,
			) >= uint64(
				records,
			) || draw.AlphaMode > 2 || !validBounds(draw.Bounds) || !containsBounds(payload.Bounds, draw.Bounds) ||
			len(
				draw.Sectors,
			) == 0 || !canonicalIDs(draw.Sectors, MaxCells) || !drawAlphaCompatible(draw.AlphaMode, readU32(payload.Records, int(draw.Record)*RecordStride+296)) {
			return ErrChunk
		}
		for index := uint64(draw.FirstIndex); index < end; index++ {
			if index%16384 == 0 {
				if err := runPayloadCheckpoint(checkpoint); err != nil {
					return err
				}
			}
			address := readU32(payload.Indices, int(index)*4)
			if uint64(address) >= uint64(addresses) {
				return ErrChunk
			}
			record := readU32(payload.Addresses, int(address)*AddressStride+4)
			if !drawAlphaCompatible(draw.AlphaMode, readU32(payload.Records, int(record)*RecordStride+296)) {
				return ErrChunk
			}
			if draw.AlphaMode == 2 {
				expected, actual := payload.RecordRefs[draw.Record], payload.RecordRefs[record]
				if actual.OwnerKind != expected.OwnerKind || actual.OwnerID != expected.OwnerID {
					return ErrChunk
				}
			}
		}
		next = end
	}
	if next != uint64(len(payload.Indices)/4) {
		return ErrChunk
	}
	return nil
}
func readU32(data []byte, offset int) uint32  { return binary.LittleEndian.Uint32(data[offset:]) }
func readF32(data []byte, offset int) float32 { return math.Float32frombits(readU32(data, offset)) }

func drawAlphaCompatible(draw, record uint32) bool {
	if draw == 0 {
		return record <= 1
	}
	return draw == record
}
