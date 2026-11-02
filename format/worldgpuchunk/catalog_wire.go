package worldgpuchunk

import "github.com/karty-game/karty-sdk/internal/binwire"

// EncodeCatalog emits the bounded canonical KGC2 root grid catalog.
func EncodeCatalog(catalog Catalog) ([]byte, error) {
	if err := catalog.Validate(); err != nil {
		return nil, err
	}
	w := binwire.NewWriter("KGC2", MaxEncodedSize)
	w.Raw(catalog.GeometrySHA256[:])
	w.Raw(catalog.MaterialLayoutSHA256[:])
	for _, value := range catalog.Origin {
		w.F64(value)
	}
	w.F64(catalog.CellSize)
	w.Count(len(catalog.Cells), false, MaxCells)
	for _, cell := range catalog.Cells {
		w.U32(cell.ID)
		w.U32(cell.SpaceID)
		w.U32(cell.Group)
		w.I32(int(cell.Grid[0]))
		w.I32(int(cell.Grid[1]))
		w.U32(cell.GridSpan)
		for _, value := range cell.Bounds {
			w.F64(value)
		}
		w.Raw(cell.ContentSHA256[:])
		w.U64(cell.Size)
		w.U64(cell.GPUBytes)
		w.U64(cell.RNMBytes)
		writeIDs(w, cell.Sectors)
		writeIDs(w, cell.PhysicalIDs)
	}
	writeIDs(w, catalog.BakedLights)
	return w.Bytes()
}

// DecodeCatalog validates all counts before allocating typed metadata.
func DecodeCatalog(encoded []byte) (Catalog, error) {
	r := binwire.NewReader(encoded, "KGC2", MaxEncodedSize)
	result := Catalog{}
	copy(result.GeometrySHA256[:], r.Raw(32))
	copy(result.MaterialLayoutSHA256[:], r.Raw(32))
	for i := range result.Origin {
		result.Origin[i] = r.F64()
	}
	result.CellSize = r.F64()
	count, _ := r.Count(MaxCells, 128, 192)
	result.Cells = make([]Cell, count)
	for i := range result.Cells {
		cell := &result.Cells[i]
		cell.ID = r.U32()
		cell.SpaceID = r.U32()
		cell.Group = r.U32()
		cell.Grid = [2]int32{int32(r.I32()), int32(r.I32())}
		cell.GridSpan = r.U32()
		for j := range cell.Bounds {
			cell.Bounds[j] = r.F64()
		}
		copy(cell.ContentSHA256[:], r.Raw(32))
		cell.Size = r.U64()
		cell.GPUBytes = r.U64()
		cell.RNMBytes = r.U64()
		cell.Sectors = readIDs(r)
		cell.PhysicalIDs = readIDs(r)
	}
	result.BakedLights = readIDs(r)
	if r.Done() != nil || result.Validate() != nil {
		return Catalog{}, ErrChunk
	}
	return result, nil
}

func writeIDs(w *binwire.Writer, ids []uint32) {
	w.Count(len(ids), false, MaxCells)
	for _, id := range ids {
		w.U32(id)
	}
}
func readIDs(r *binwire.Reader) []uint32 {
	count, _ := r.Count(MaxCells, 4, 4)
	result := make([]uint32, count)
	for i := range result {
		result[i] = r.U32()
	}
	return result
}
