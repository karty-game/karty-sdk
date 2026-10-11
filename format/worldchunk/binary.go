package worldchunk

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"

	"github.com/karty-game/karty-sdk/format/world"
)

const (
	binaryVersion        = 1
	catalogBinaryVersion = 2
	catalogHeaderBytes   = 64
	chunkRecordBytes     = 120
	meshHeaderBytes      = 144
	vertexRecordBytes    = 84
	triangleRecordBytes  = 220
	shadowHeaderBytes    = 80
	shadowRecordBytes    = 68
)

type wireWriter struct {
	raw []byte
	at  int
}

func (w *wireWriter) u32(v uint32)  { binary.LittleEndian.PutUint32(w.raw[w.at:w.at+4], v); w.at += 4 }
func (w *wireWriter) i32(v int)     { w.u32(uint32(int32(v))) }
func (w *wireWriter) u64(v uint64)  { binary.LittleEndian.PutUint64(w.raw[w.at:w.at+8], v); w.at += 8 }
func (w *wireWriter) f32(v float32) { w.u32(math.Float32bits(v)) }
func (w *wireWriter) f64(v float64) { w.u64(math.Float64bits(v)) }
func (w *wireWriter) flag(v bool) {
	if v {
		w.u32(1)
	} else {
		w.u32(0)
	}
}
func (w *wireWriter) hash(value string) error {
	if len(value) != 64 {
		return ErrChunk
	}
	if _, err := hex.Decode(w.raw[w.at:w.at+32], []byte(value)); err != nil {
		return ErrChunk
	}
	w.at += 32
	return nil
}

type wireReader struct {
	raw []byte
	at  int
}

func (r *wireReader) u32() uint32 {
	v := binary.LittleEndian.Uint32(r.raw[r.at : r.at+4])
	r.at += 4
	return v
}
func (r *wireReader) i32() int { return int(int32(r.u32())) }
func (r *wireReader) u64() uint64 {
	v := binary.LittleEndian.Uint64(r.raw[r.at : r.at+8])
	r.at += 8
	return v
}
func (r *wireReader) f32() float32 { return math.Float32frombits(r.u32()) }
func (r *wireReader) f64() float64 { return math.Float64frombits(r.u64()) }
func (r *wireReader) hash() string {
	v := hex.EncodeToString(r.raw[r.at : r.at+32])
	r.at += 32
	return v
}
func (r *wireReader) flag() (bool, error) {
	v := r.u32()
	if v > 1 {
		return false, ErrChunk
	}
	return v == 1, nil
}
func (r *wireReader) byteWord() (uint8, error) {
	v := r.u32()
	if v > 255 {
		return 0, ErrChunk
	}
	return uint8(v), nil
}

func wireHeader(encoded []byte, magic string, size int) error {
	return wireHeaderVersion(encoded, magic, size, binaryVersion)
}
func wireHeaderVersion(encoded []byte, magic string, size, version int) error {
	if len(encoded) < size || len(encoded) > MaxEncodedSize || !bytes.Equal(encoded[:4], []byte(magic)) ||
		binary.LittleEndian.Uint16(encoded[4:6]) != uint16(version) || binary.LittleEndian.Uint16(encoded[6:8]) != uint16(size) {
		return fmt.Errorf("binary header: %w", ErrChunk)
	}
	return nil
}
func newWire(size uint64, magic string, header int) (wireWriter, error) {
	return newWireVersion(size, magic, header, binaryVersion)
}
func newWireVersion(size uint64, magic string, header, version int) (wireWriter, error) {
	if size > MaxEncodedSize {
		return wireWriter{}, ErrChunk
	}
	w := wireWriter{raw: make([]byte, int(size)), at: 8}
	copy(w.raw, magic)
	binary.LittleEndian.PutUint16(w.raw[4:6], uint16(version))
	binary.LittleEndian.PutUint16(w.raw[6:8], uint16(header))
	return w, nil
}
func surfaceCode(surface string) uint32 {
	switch surface {
	case "sector-floor":
		return 1
	case "sector-ceiling":
		return 2
	case "sector-wall":
		return 3
	case "solid-top":
		return 4
	case "solid-bottom":
		return 5
	case "solid-side":
		return 6
	default:
		return 0
	}
}
func surfaceName(code uint32) (string, error) {
	switch code {
	case 1:
		return "sector-floor", nil
	case 2:
		return "sector-ceiling", nil
	case 3:
		return "sector-wall", nil
	case 4:
		return "solid-top", nil
	case 5:
		return "solid-bottom", nil
	case 6:
		return "solid-side", nil
	default:
		return "", ErrChunk
	}
}
func (w *wireWriter) basis(value Basis) {
	for _, v := range value.Tangent {
		w.f32(v)
	}
	for _, v := range value.Bitangent {
		w.f32(v)
	}
}
func (r *wireReader) basis() (value Basis) {
	for i := range value.Tangent {
		value.Tangent[i] = r.f32()
	}
	for i := range value.Bitangent {
		value.Bitangent[i] = r.f32()
	}
	return value
}

func encodeMeshBinary(mesh Mesh) ([]byte, error) {
	size := uint64(meshHeaderBytes) + 4*uint64(len(mesh.Sectors)+len(mesh.Solids)+len(mesh.Indices)) +
		vertexRecordBytes*uint64(len(mesh.Vertices)) + triangleRecordBytes*uint64(len(mesh.Triangles))
	w, err := newWire(size, "KCM1", meshHeaderBytes)
	if err != nil {
		return nil, err
	}
	for _, count := range []int{len(mesh.Sectors), len(mesh.Solids), len(mesh.Vertices), len(mesh.Indices), len(mesh.Triangles)} {
		w.u32(uint32(count))
	}
	w.u32(0)
	if err := w.hash(mesh.GeometrySHA256); err != nil {
		return nil, err
	}
	if err := w.hash(mesh.SurfaceSHA256); err != nil {
		return nil, err
	}
	for _, v := range mesh.Bounds {
		w.f64(v)
	}
	for _, v := range mesh.Sectors {
		w.i32(v)
	}
	for _, v := range mesh.Solids {
		w.i32(v)
	}
	for _, v := range mesh.Vertices {
		for _, x := range v.Position {
			w.f32(x)
		}
		for _, uv := range v.UV {
			for _, x := range uv {
				w.f32(x)
			}
		}
		for _, x := range v.BandUV {
			w.f32(x)
		}
		for _, uv := range v.SecondaryUV {
			for _, x := range uv {
				w.f32(x)
			}
		}
		for _, x := range v.LightmapUV {
			w.f32(x)
		}
		for _, x := range v.WaveUV {
			w.f32(x)
		}
	}
	for _, index := range mesh.Indices {
		w.u32(index)
	}
	for _, t := range mesh.Triangles {
		writeTriangle(&w, t)
	}
	return w.raw, nil
}
func writeTriangle(w *wireWriter, t Triangle) {
	w.i32(t.Sector)
	w.i32(t.Solid)
	w.u32(surfaceCode(t.Surface))
	w.i32(t.SurfaceEdge)
	for _, vector := range [3][3]float32{t.Normal, t.Tangent, t.Bitangent} {
		for _, v := range vector {
			w.f32(v)
		}
	}
	w.u32(t.Material)
	w.u32(uint32(t.MappingCount))
	for _, v := range t.Weights {
		w.f32(v)
	}
	for _, basis := range t.ProjectionBases {
		w.basis(basis)
	}
	w.u32(t.Band.Material)
	w.u32(uint32(t.Band.Coverage))
	w.flag(t.Band.RepeatU)
	w.flag(t.Band.RepeatV)
	w.basis(t.Band.Basis)
	w.u32(t.Secondary.Material)
	w.f32(t.Secondary.Strength)
	w.u32(uint32(t.Secondary.MappingCount))
	for _, v := range t.Secondary.Weights {
		w.f32(v)
	}
	w.flag(t.Sky)
	w.flag(t.Outdoor)
	w.i32(t.Chart)
}
func readTriangle(r *wireReader) (t Triangle, err error) {
	t.Sector = r.i32()
	t.Solid = r.i32()
	t.Surface, err = surfaceName(r.u32())
	if err != nil {
		return t, err
	}
	t.SurfaceEdge = r.i32()
	for _, vector := range []*[3]float32{&t.Normal, &t.Tangent, &t.Bitangent} {
		for i := range vector {
			vector[i] = r.f32()
		}
	}
	t.Material = r.u32()
	t.MappingCount, err = r.byteWord()
	if err != nil {
		return t, err
	}
	for i := range t.Weights {
		t.Weights[i] = r.f32()
	}
	for i := range t.ProjectionBases {
		t.ProjectionBases[i] = r.basis()
	}
	t.Band.Material = r.u32()
	t.Band.Coverage, err = r.byteWord()
	if err != nil {
		return t, err
	}
	t.Band.RepeatU, err = r.flag()
	if err != nil {
		return t, err
	}
	t.Band.RepeatV, err = r.flag()
	if err != nil {
		return t, err
	}
	t.Band.Basis = r.basis()
	t.Secondary.Material = r.u32()
	t.Secondary.Strength = r.f32()
	t.Secondary.MappingCount, err = r.byteWord()
	if err != nil {
		return t, err
	}
	for i := range t.Secondary.Weights {
		t.Secondary.Weights[i] = r.f32()
	}
	t.Sky, err = r.flag()
	if err != nil {
		return t, err
	}
	t.Outdoor, err = r.flag()
	if err != nil {
		return t, err
	}
	t.Chart = r.i32()
	return t, nil
}
func decodeMeshBinary(encoded []byte, mesh *Mesh) error {
	if err := wireHeader(encoded, "KCM1", meshHeaderBytes); err != nil {
		return err
	}
	r := wireReader{raw: encoded, at: 8}
	sectors, solids, vertices, indices, triangles := r.u32(), r.u32(), r.u32(), r.u32(), r.u32()
	if sectors == 0 || sectors > world.MaxSectors || solids > world.MaxStaticSolids || vertices == 0 || vertices > MaxVertices ||
		triangles > MaxTriangles || vertices != 3*triangles || indices != vertices {
		return fmt.Errorf("array record bound: %w", ErrChunk)
	}
	if r.u32() != 0 {
		return ErrChunk
	}
	size := uint64(meshHeaderBytes) + 4*(uint64(sectors)+uint64(solids)+uint64(indices)) +
		vertexRecordBytes*uint64(vertices) + triangleRecordBytes*uint64(triangles)
	if size != uint64(len(encoded)) {
		return fmt.Errorf("binary mesh length: %w", ErrChunk)
	}
	mesh.Schema = MeshSchema
	mesh.GeometrySHA256 = r.hash()
	mesh.SurfaceSHA256 = r.hash()
	for i := range mesh.Bounds {
		mesh.Bounds[i] = r.f64()
	}
	mesh.Sectors = make([]int, int(sectors))
	for i := range mesh.Sectors {
		mesh.Sectors[i] = r.i32()
	}
	if solids > 0 {
		mesh.Solids = make([]int, int(solids))
		for i := range mesh.Solids {
			mesh.Solids[i] = r.i32()
		}
	}
	var work cooperativeWork
	mesh.Vertices = make([]Vertex, int(vertices))
	for i := range mesh.Vertices {
		work.checkpoint(1024)
		v := &mesh.Vertices[i]
		for j := range v.Position {
			v.Position[j] = r.f32()
		}
		for j := range v.UV {
			for k := range v.UV[j] {
				v.UV[j][k] = r.f32()
			}
		}
		for j := range v.BandUV {
			v.BandUV[j] = r.f32()
		}
		for j := range v.SecondaryUV {
			for k := range v.SecondaryUV[j] {
				v.SecondaryUV[j][k] = r.f32()
			}
		}
		for j := range v.LightmapUV {
			v.LightmapUV[j] = r.f32()
		}
		for j := range v.WaveUV {
			v.WaveUV[j] = r.f32()
		}
	}
	mesh.Indices = make([]uint32, int(indices))
	for i := range mesh.Indices {
		work.checkpoint(1024)
		mesh.Indices[i] = r.u32()
	}
	mesh.Triangles = make([]Triangle, int(triangles))
	for i := range mesh.Triangles {
		work.checkpoint(1024)
		var err error
		mesh.Triangles[i], err = readTriangle(&r)
		if err != nil {
			return err
		}
	}
	return nil
}

func encodeCatalogBinary(catalog Catalog) ([]byte, error) {
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
	) + chunkRecordBytes*uint64(
		len(catalog.Chunks),
	) + 4*(sectors+solids+uint64(len(catalog.SectorChunks)+len(catalog.Candidates)+len(catalog.BakedLights))+candidates)
	w, err := newWireVersion(size, "KCC2", catalogHeaderBytes, catalogBinaryVersion)
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
	if err := wireHeaderVersion(encoded, "KCC2", catalogHeaderBytes, catalogBinaryVersion); err != nil {
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
	fixed := uint64(catalogHeaderBytes) + chunkRecordBytes*uint64(chunks)
	if fixed > uint64(len(encoded)) {
		return ErrChunk
	}
	sectorTotal, solidTotal := uint64(0), uint64(0)
	for i := range chunks {
		at := catalogHeaderBytes + int(i)*chunkRecordBytes
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

func encodeShadowBinary(proxy ShadowProxy) ([]byte, error) {
	w, err := newWire(uint64(shadowHeaderBytes)+shadowRecordBytes*uint64(len(proxy.Triangles)), "KCS1", shadowHeaderBytes)
	if err != nil {
		return nil, err
	}
	w.u32(uint32(len(proxy.Triangles)))
	w.u32(0)
	if err := w.hash(proxy.GeometrySHA256); err != nil {
		return nil, err
	}
	if err := w.hash(proxy.SurfaceSHA256); err != nil {
		return nil, err
	}
	for _, t := range proxy.Triangles {
		for _, p := range t.Positions {
			for _, v := range p {
				w.f32(v)
			}
		}
		for _, v := range t.Normal {
			w.f32(v)
		}
		w.u32(t.Material)
		w.i32(t.Sector)
		w.i32(t.Solid)
		w.u32(surfaceCode(t.Surface))
		w.i32(t.SurfaceEdge)
	}
	return w.raw, nil
}
func decodeShadowBinary(encoded []byte, proxy *ShadowProxy) error {
	if err := wireHeader(encoded, "KCS1", shadowHeaderBytes); err != nil {
		return err
	}
	r := wireReader{raw: encoded, at: 8}
	count := r.u32()
	if count > MaxShadowTriangles {
		return fmt.Errorf("array record bound: %w", ErrChunk)
	}
	if r.u32() != 0 || uint64(shadowHeaderBytes)+shadowRecordBytes*uint64(count) != uint64(len(encoded)) {
		return ErrChunk
	}
	proxy.Schema = ShadowSchema
	proxy.GeometrySHA256 = r.hash()
	proxy.SurfaceSHA256 = r.hash()
	proxy.Triangles = make([]ShadowTriangle, int(count))
	var work cooperativeWork
	for i := range proxy.Triangles {
		work.checkpoint(1024)
		t := &proxy.Triangles[i]
		for j := range t.Positions {
			for k := range t.Positions[j] {
				t.Positions[j][k] = r.f32()
			}
		}
		for j := range t.Normal {
			t.Normal[j] = r.f32()
		}
		t.Material = r.u32()
		t.Sector = r.i32()
		t.Solid = r.i32()
		var err error
		t.Surface, err = surfaceName(r.u32())
		if err != nil {
			return err
		}
		t.SurfaceEdge = r.i32()
	}
	return nil
}
