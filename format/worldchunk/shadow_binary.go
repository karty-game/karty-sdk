package worldchunk

import (
	"fmt"
)

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
