package worldchunk

import (
	"encoding/binary"
	"fmt"

	"github.com/karty-game/karty-sdk/format/world"
)

func encodeCatalogBinary(catalog Catalog) ([]byte, error) {
	if catalog.Schema == SpatialSchema {
		return encodeSpatialCatalog(catalog)
	}
	return encodeLegacyCatalog(catalog)
}

func encodeLegacyCatalog(catalog Catalog) ([]byte, error) {
	recordBytes, magic, version := chunkRecordBytes, "KCC2", catalogBinaryVersion
	if catalog.Schema == FixedSchema {
		recordBytes += 16
		magic = "KCC3"
		version = 3
	}
	sectors, solids, candidates := uint64(0), uint64(0), uint64(0)
	for _, chunk := range catalog.Chunks {
		sectors += uint64(len(chunk.Sectors))
		solids += uint64(len(chunk.Solids))
	}
	for _, list := range catalog.Candidates {
		candidates += uint64(len(list))
	}
	size := uint64(
		catalogHeaderBytes,
	) + uint64(recordBytes)*uint64(
		len(catalog.Chunks),
	) + 4*(sectors+solids+uint64(len(catalog.SectorChunks)+len(catalog.Candidates)+len(catalog.BakedLights))+candidates)
	w, err := newWireVersion(size, magic, catalogHeaderBytes, version)
	if err != nil {
		return nil, err
	}
	w.u32(uint32(len(catalog.Chunks)))
	w.u32(uint32(len(catalog.SectorChunks)))
	w.u32(uint32(candidates))
	w.u32(uint32(len(catalog.BakedLights)))
	if err := w.hash(catalog.GeometrySHA256); err != nil {
		return nil, err
	}
	w.u64(0)
	for _, c := range catalog.Chunks {
		w.u32(uint32(c.ID))
		w.u32(uint32(len(c.Sectors)))
		w.u32(uint32(len(c.Solids)))
		w.u32(0)
		if err := w.hash(c.ContentSHA256); err != nil {
			return nil, err
		}
		w.u64(c.Size)
		w.u64(c.MeshBytes)
		w.u64(c.LightmapBytes)
		if catalog.Schema == FixedSchema {
			w.u64(c.LightingBytes)
			w.u64(c.TerrainBytes)
		}
		for _, v := range c.Bounds {
			w.f64(v)
		}
	}
	for _, c := range catalog.Chunks {
		for _, v := range c.Sectors {
			w.i32(v)
		}
	}
	for _, c := range catalog.Chunks {
		for _, v := range c.Solids {
			w.i32(v)
		}
	}
	for _, v := range catalog.SectorChunks {
		w.u32(uint32(v))
	}
	for _, list := range catalog.Candidates {
		w.u32(uint32(len(list)))
		for _, v := range list {
			w.u32(uint32(v))
		}
	}
	for _, index := range catalog.BakedLights {
		w.i32(index)
	}
	return w.raw, nil
}

func decodeCatalogBinary(encoded []byte, catalog *Catalog) error {
	if len(encoded) >= 4 && string(encoded[:4]) == "KCC5" {
		return decodeSpatialCatalog(encoded, catalog)
	}
	return decodeLegacyCatalog(encoded, catalog)
}

func decodeLegacyCatalog(encoded []byte, catalog *Catalog) error {
	recordBytes, magic, version := chunkRecordBytes, "KCC2", catalogBinaryVersion
	if len(encoded) >= 4 && string(encoded[:4]) == "KCC3" {
		recordBytes += 16
		magic = "KCC3"
		version = 3
	}
	if err := wireHeaderVersion(encoded, magic, catalogHeaderBytes, version); err != nil {
		return err
	}
	r := wireReader{raw: encoded, at: 8}
	chunks, sectors, candidates := r.u32(), r.u32(), r.u32()
	bakedLights := r.u32()
	if chunks == 0 || chunks > world.MaxSectors || sectors == 0 || sectors > world.MaxSectors || candidates > chunks*sectors ||
		bakedLights > world.MaxLights {
		return fmt.Errorf("array record bound: %w", ErrChunk)
	}
	if binary.LittleEndian.Uint64(encoded[56:64]) != 0 {
		return ErrChunk
	}
	fixed := uint64(catalogHeaderBytes) + uint64(recordBytes)*uint64(chunks)
	if fixed > uint64(len(encoded)) {
		return ErrChunk
	}
	sectorTotal, solidTotal := uint64(0), uint64(0)
	for i := range chunks {
		at := catalogHeaderBytes + int(i)*recordBytes
		sc := binary.LittleEndian.Uint32(encoded[at+4 : at+8])
		so := binary.LittleEndian.Uint32(encoded[at+8 : at+12])
		if sc == 0 || sc > world.MaxSectors || so > world.MaxStaticSolids || binary.LittleEndian.Uint32(encoded[at+12:at+16]) != 0 {
			return ErrChunk
		}
		sectorTotal += uint64(sc)
		solidTotal += uint64(so)
	}
	if sectorTotal > world.MaxSectors || solidTotal > world.MaxStaticSolids {
		return fmt.Errorf("array record bound: %w", ErrChunk)
	}
	length := fixed + 4*(sectorTotal+solidTotal+uint64(sectors)*2+uint64(candidates)+uint64(bakedLights))
	if length != uint64(len(encoded)) {
		return fmt.Errorf("binary catalog length: %w", ErrChunk)
	}
	candidateStart := int(fixed + 4*(sectorTotal+solidTotal+uint64(sectors)))
	check := wireReader{raw: encoded, at: candidateStart}
	sum := uint64(0)
	for range sectors {
		if check.at+4 > len(encoded) {
			return ErrChunk
		}
		count := check.u32()
		sum += uint64(count)
		if count > chunks || sum > uint64(candidates) || uint64(check.at)+uint64(count)*4 > uint64(len(encoded)) {
			return ErrChunk
		}
		check.at += int(count) * 4
	}
	if sum != uint64(candidates) || uint64(check.at)+uint64(bakedLights)*4 != uint64(len(encoded)) {
		return ErrChunk
	}
	// Scan the complete source-index list before allocating any catalog slices.
	previous := int32(-1)
	for range bakedLights {
		index := int32(check.u32())
		if index < 0 || index >= world.MaxLights || index <= previous {
			return ErrChunk
		}
		previous = index
	}
	catalog.Schema = Schema
	if magic == "KCC3" {
		catalog.Schema = FixedSchema
	}
	catalog.GeometrySHA256 = r.hash()
	r.u64()
	catalog.Chunks = make([]Chunk, int(chunks))
	for i := range catalog.Chunks {
		c := &catalog.Chunks[i]
		c.ID = int(r.u32())
		sc, so := r.u32(), r.u32()
		r.u32()
		c.ContentSHA256 = r.hash()
		c.Size = r.u64()
		c.MeshBytes = r.u64()
		c.LightmapBytes = r.u64()
		if magic == "KCC3" {
			c.LightingBytes = r.u64()
			c.TerrainBytes = r.u64()
		}
		for j := range c.Bounds {
			c.Bounds[j] = r.f64()
		}
		c.Sectors = make([]int, int(sc))
		if so > 0 {
			c.Solids = make([]int, int(so))
		}
	}
	for i := range catalog.Chunks {
		for j := range catalog.Chunks[i].Sectors {
			catalog.Chunks[i].Sectors[j] = r.i32()
		}
	}
	for i := range catalog.Chunks {
		for j := range catalog.Chunks[i].Solids {
			catalog.Chunks[i].Solids[j] = r.i32()
		}
	}
	catalog.SectorChunks = make([]int, int(sectors))
	for i := range catalog.SectorChunks {
		catalog.SectorChunks[i] = int(r.u32())
	}
	catalog.Candidates = make([][]int, int(sectors))
	for i := range catalog.Candidates {
		count := r.u32()
		catalog.Candidates[i] = make([]int, int(count))
		for j := range catalog.Candidates[i] {
			catalog.Candidates[i][j] = int(r.u32())
		}
	}
	if bakedLights > 0 {
		catalog.BakedLights = make([]int, int(bakedLights))
		for i := range catalog.BakedLights {
			catalog.BakedLights[i] = r.i32()
		}
	}
	return nil
}
