package worldchunk

import (
	"math"
)

const readyHeaderBytes, readySurfaceBytes, readyBatchHeaderBytes, readyOwnerBytes = 48, 24, 48, 64

// EncodeReady writes bounded renderer streams. Builders check complete semantic
// association with DecodeReady/ValidateReady before publishing a catalog hash.
// Runtime uses DecodeReadyArtifact after whole-child integrity verification.
func EncodeReady(ready Ready) ([]byte, error) {
	if err := readyShape(&ready); err != nil {
		return nil, err
	}
	size := uint64(readyHeaderBytes + len(ready.Surfaces)*readySurfaceBytes)
	for _, batch := range ready.Batches {
		size += readyBatchHeaderBytes + uint64(len(batch.Owners))*readyOwnerBytes
		for _, count := range readyCounts(batch)[:8] {
			size += uint64(count) * 4
		}
	}
	w, err := newWire(size, "KCR1", readyHeaderBytes)
	if err != nil {
		return nil, err
	}
	if err := w.hash(ready.MeshSHA256); err != nil {
		return nil, err
	}
	w.u32(uint32(len(ready.Surfaces)))
	w.u32(uint32(len(ready.Batches)))
	for _, surface := range ready.Surfaces {
		w.i32(surface.Primary)
		w.i32(surface.Secondary)
		w.i32(surface.Band)
		w.f32(surface.SecondaryStrength)
		w.u32(uint32(surface.BandCoverage))
		w.u32(readySurfaceFlags(surface))
	}
	for _, batch := range ready.Batches {
		w.i32(batch.Surface)
		w.u32(readyFlags(batch))
		w.i32(int(batch.Winding))
		for _, count := range readyCounts(batch) {
			w.u32(uint32(count))
		}
		for _, values := range [][]float32{batch.Vertices} {
			for _, value := range values {
				w.f32(value)
			}
		}
		for _, values := range [][]uint32{batch.Indices, batch.SurfaceIDs, batch.Sources} {
			for _, value := range values {
				w.u32(value)
			}
		}
		for _, values := range [][]float32{batch.WaveUV, batch.LightmapUV, batch.LightmapBasis, batch.MaterialBasis} {
			for _, value := range values {
				w.f32(value)
			}
		}
		for _, owner := range batch.Owners {
			w.i32(owner.Sector)
			w.i32(owner.Solid)
			w.i32(owner.First)
			w.i32(owner.Count)
			for _, value := range owner.Bounds {
				w.f64(value)
			}
		}
	}
	return w.raw, nil
}

func readyCounts(batch ReadyBatch) []int {
	return []int{len(batch.Vertices), len(batch.Indices), len(batch.SurfaceIDs), len(batch.Sources), len(batch.WaveUV),
		len(batch.LightmapUV), len(batch.LightmapBasis), len(batch.MaterialBasis), len(batch.Owners)}
}

func decodeReadyBinary(encoded []byte) (Ready, error) {
	if err := wireHeader(encoded, "KCR1", readyHeaderBytes); err != nil {
		return Ready{}, err
	}
	r := wireReader{raw: encoded, at: 8}
	ready := Ready{Schema: ReadySchema, MeshSHA256: r.hash()}
	surfaces, batches := uint64(r.u32()), uint64(r.u32())
	if surfaces == 0 || surfaces > MaxTriangles || batches == 0 || batches > MaxTriangles ||
		surfaces*readySurfaceBytes+batches*readyBatchHeaderBytes > uint64(len(encoded)-readyHeaderBytes) {
		return Ready{}, ErrChunk
	}
	ready.Surfaces = make([]ReadySurface, int(surfaces))
	for index := range ready.Surfaces {
		surface := ReadySurface{Primary: r.i32(), Secondary: r.i32(), Band: r.i32(), SecondaryStrength: r.f32()}
		coverage, flags := r.u32(), r.u32()
		if coverage > 2 || flags > 3 {
			return Ready{}, ErrChunk
		}
		surface.BandCoverage, surface.BandRepeatU, surface.BandRepeatV = uint8(coverage), flags&1 != 0, flags&2 != 0
		ready.Surfaces[index] = surface
	}
	ready.Batches = make([]ReadyBatch, int(batches))
	var decoded uint64
	for index := range ready.Batches {
		if len(encoded)-r.at < readyBatchHeaderBytes {
			return Ready{}, ErrChunk
		}
		batch := ReadyBatch{Surface: r.i32()}
		flags, winding := r.u32(), r.i32()
		if flags > 63 || winding < -1 || winding > 1 {
			return Ready{}, ErrChunk
		}
		batch.Sky, batch.Outdoor, batch.StaticMaterial = flags&1 != 0, flags&2 != 0, flags&4 != 0
		batch.Transparent, batch.Invisible, batch.Masked, batch.Winding = flags&8 != 0, flags&16 != 0, flags&32 != 0, int8(winding)
		var counts [9]uint64
		var bytes uint64
		for field := range counts {
			counts[field] = uint64(r.u32())
			stride := uint64(4)
			if field == 8 {
				stride = readyOwnerBytes
			}
			bytes += counts[field] * stride
		}
		decoded += bytes + readyBatchHeaderBytes
		if decoded > MaxReadyBytes || bytes > uint64(len(encoded)-r.at) {
			return Ready{}, ErrChunk
		}
		batch.Vertices = readyReadFloats(&r, counts[0])
		batch.Indices = readyReadIndexes(&r, counts[1])
		batch.SurfaceIDs = readyReadIndexes(&r, counts[2])
		batch.Sources = readyReadIndexes(&r, counts[3])
		batch.WaveUV = readyReadFloats(&r, counts[4])
		batch.LightmapUV = readyReadFloats(&r, counts[5])
		batch.LightmapBasis = readyReadFloats(&r, counts[6])
		batch.MaterialBasis = readyReadFloats(&r, counts[7])
		batch.Owners = make([]ReadyOwner, int(counts[8]))
		for owner := range batch.Owners {
			value := ReadyOwner{Sector: r.i32(), Solid: r.i32(), First: r.i32(), Count: r.i32()}
			for axis := range value.Bounds {
				value.Bounds[axis] = r.f64()
			}
			batch.Owners[owner] = value
		}
		ready.Batches[index] = batch
	}
	if r.at != len(encoded) || readyShape(&ready) != nil {
		return Ready{}, ErrChunk
	}
	return ready, nil
}

func readyReadFloats(r *wireReader, count uint64) []float32 {
	if count == 0 {
		return nil
	}
	values := make([]float32, int(count))
	for index := range values {
		values[index] = r.f32()
	}
	return values
}

func readyReadIndexes(r *wireReader, count uint64) []uint32 {
	if count == 0 {
		return nil
	}
	values := make([]uint32, int(count))
	for index := range values {
		values[index] = r.u32()
	}
	return values
}

func readyFlags(batch ReadyBatch) uint32 {
	var flags uint32
	for index, value := range []bool{batch.Sky, batch.Outdoor, batch.StaticMaterial, batch.Transparent, batch.Invisible, batch.Masked} {
		if value {
			flags |= 1 << uint(index)
		}
	}
	return flags
}

func readySurfaceFlags(surface ReadySurface) uint32 {
	var flags uint32
	if surface.BandRepeatU {
		flags |= 1
	}
	if surface.BandRepeatV {
		flags |= 2
	}
	return flags
}

func readyShape(ready *Ready) error {
	if ready == nil || ready.Schema != ReadySchema || !hash(ready.MeshSHA256) || len(ready.Surfaces) == 0 ||
		len(
			ready.Surfaces,
		) > MaxTriangles || len(ready.Batches) == 0 || len(ready.Batches) > MaxTriangles || ReadyBytes(ready) > MaxReadyBytes {
		return ErrChunk
	}
	vertices := 0
	for _, surface := range ready.Surfaces {
		if surface.Primary < 0 || surface.Secondary < 0 || surface.Band < 0 || !finite(float64(surface.SecondaryStrength)) ||
			surface.SecondaryStrength < 0 || surface.SecondaryStrength > 1 || surface.BandCoverage > 2 {
			return ErrChunk
		}
	}
	for _, batch := range ready.Batches {
		count := len(batch.Vertices) / ReadyStride
		vertices += count
		if batch.Surface < 0 || batch.Surface >= len(ready.Surfaces) || len(batch.Vertices)%ReadyStride != 0 || count == 0 ||
			count%3 != 0 || vertices > MaxVertices || len(batch.Indices) != count || len(batch.SurfaceIDs) != count ||
			len(batch.Sources) != count/3 || len(batch.WaveUV) != count*2 || len(batch.Owners) == 0 || len(batch.Owners) > count/3 ||
			batch.Winding < -1 || batch.Winding > 1 ||
			len(batch.LightmapUV) != 0 && len(batch.LightmapUV) != count*2 ||
			len(batch.LightmapBasis) != len(batch.LightmapUV)*3 ||
			batch.StaticMaterial && len(batch.MaterialBasis) != count*8 || !batch.StaticMaterial && len(batch.MaterialBasis) != 0 {
			return ErrChunk
		}
		for index, id := range batch.Indices {
			if id != uint32(index) || uint64(batch.SurfaceIDs[index]) >= uint64(len(ready.Surfaces)) {
				return ErrChunk
			}
		}
		for _, values := range [][]float32{batch.Vertices, batch.WaveUV, batch.LightmapUV, batch.LightmapBasis, batch.MaterialBasis} {
			for _, value := range values {
				if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
					return ErrChunk
				}
			}
		}
	}
	return nil
}
