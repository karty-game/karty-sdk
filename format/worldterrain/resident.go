package worldterrain

// ResidentGeometryBytes counts immutable scalar source storage and one
// compiled indexed mesh per patch. Call only for an already validated document.
// Consumers separately reserve object overhead, staging copies and GPU images.
func ResidentGeometryBytes(document *Document) uint64 {
	if document == nil {
		return 0
	}
	bytes := DecodedBytes(document)
	for _, patch := range document.Patches {
		bytes += uint64((int(patch.Columns)+1)*(int(patch.Rows)+1)) * 24
		for y := 0; y < int(patch.Rows); y++ {
			for x := 0; x < int(patch.Columns); x++ {
				at := (int(patch.Y)+y)*(int(document.Columns)-1) + int(patch.X) + x
				if document.Holes[at] == 0 {
					bytes += 24
				}
			}
		}
	}
	return bytes
}
