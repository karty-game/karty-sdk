package binwire

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"testing"
)

func TestCanonicalPrimitiveRoundtrip(t *testing.T) {
	w := NewWriter("TEST", 1024)
	w.I32(-17)
	w.U8(255)
	w.U16(65535)
	w.U32(1 << 31)
	w.U64(1 << 63)
	w.F32(1.25)
	w.F64(-2.5)
	w.Bool(true)
	w.String("世界", 16)
	w.Count(0, true, 8)
	w.Count(0, false, 8)
	b, err := w.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	r := NewReader(b, "TEST", 1024)
	if r.I32() != -17 || r.U8() != 255 || r.U16() != 65535 || r.U32() != 1<<31 || r.U64() != 1<<63 || r.F32() != 1.25 || r.F64() != -2.5 ||
		!r.Bool() ||
		r.String(16) != "世界" {
		t.Fatal("primitive roundtrip")
	}
	if n, p := r.Count(8, 1, 8); n != 0 || p {
		t.Fatal("nil slice presence")
	}
	if n, p := r.Count(8, 1, 8); n != 0 || !p {
		t.Fatal("empty slice presence")
	}
	if err := r.Done(); err != nil {
		t.Fatal(err)
	}
}
func TestRejectsHeadersCountsAndAllocationsBeforeReading(t *testing.T) {
	w := NewWriter("TEST", 64)
	w.Count(1, false, 2)
	w.U64(42)
	b, _ := w.Bytes()
	for name, mutate := range map[string]func([]byte){"version": func(v []byte) { v[4] = 2 }, "flags": func(v []byte) { v[6] = 1 }, "reserved": func(v []byte) { v[12] = 1 }, "size": func(v []byte) { v[8] = 0 }, "magic": func(v []byte) { v[0] = '?' }, "count": func(v []byte) { binary.LittleEndian.PutUint32(v[16:], math.MaxUint32) }} {
		t.Run(name, func(t *testing.T) {
			raw := bytes.Clone(b)
			mutate(raw)
			r := NewReader(raw, "TEST", 64)
			r.Count(2, 8, 8)
			if r.Err() == nil {
				t.Fatal("malformed accepted")
			}
			if r.allocated != 0 {
				t.Fatal("bad record charged allocation")
			}
		})
	}
	r := NewReader(b, "TEST", 64)
	r.Count(2, 8, AllocationLimit+1)
	if !errors.Is(r.Err(), ErrCount) {
		t.Fatal("oversized allocation accepted")
	}
	r = NewReader(b, "TEST", 64)
	r.Allocate(AllocationLimit)
	r.Allocate(1)
	if !errors.Is(r.Err(), ErrCount) {
		t.Fatal("aggregate allocation accepted")
	}
	for size := range len(b) {
		r := NewReader(b[:size], "TEST", 64)
		r.Count(2, 8, 8)
		r.U64()
		if r.Done() == nil {
			t.Fatalf("truncation%d", size)
		}
	}
}
func TestCanonicalBooleansUTF8AndFloats(t *testing.T) {
	for _, bits := range []uint64{math.Float64bits(math.NaN()), math.Float64bits(math.Inf(1)), math.Float64bits(math.Inf(-1)), 1 << 63} {
		w := NewWriter("TEST", 64)
		w.U64(bits)
		b, _ := w.Bytes()
		r := NewReader(b, "TEST", 64)
		r.F64()
		if r.Done() == nil {
			t.Fatalf("floatbits%x", bits)
		}
	}
	w := NewWriter("TEST", 64)
	w.U8(2)
	b, _ := w.Bytes()
	r := NewReader(b, "TEST", 64)
	r.Bool()
	if r.Done() == nil {
		t.Fatal("invalid boolean")
	}
	w = NewWriter("TEST", 64)
	w.U32(1)
	w.U8(255)
	b, _ = w.Bytes()
	r = NewReader(b, "TEST", 64)
	r.SkipString(4)
	if r.Done() == nil {
		t.Fatal("invalid UTF8")
	}
	w = NewWriter("TEST", 64)
	w.F64(math.Copysign(0, -1))
	b, _ = w.Bytes()
	if binary.LittleEndian.Uint64(b[16:]) != 0 {
		t.Fatal("negative zero writer")
	}
}
func TestScanDoesNotAllocateStrings(t *testing.T) {
	w := NewWriter("TEST", 1024)
	w.String("bounded text", 32)
	b, _ := w.Bytes()
	baseline := testing.AllocsPerRun(100, func() {
		r := NewReader(b, "TEST", 1024)
		if r.Err() != nil {
			panic("reader")
		}
	})
	if got := testing.AllocsPerRun(100, func() {
		r := NewReader(b, "TEST", 1024)
		r.SkipString(32)
		if r.Done() != nil {
			panic("scan")
		}
	}); got != baseline {
		t.Fatalf("scan allocations%g reader baseline%g", got, baseline)
	}
}

func TestConditionalStringScanKeepsAllocationAndMalformedBounds(t *testing.T) {
	w := NewWriter("TEST", 128)
	w.String("known", 16)
	b, err := w.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	baseline := testing.AllocsPerRun(100, func() {
		r := NewReader(b, "TEST", 128)
		if r.Err() != nil {
			panic("reader")
		}
	})
	if allocated := testing.AllocsPerRun(100, func() {
		r := NewReader(b, "TEST", 128)
		if !r.StringIn(16, "other", "known") || r.Done() != nil {
			panic("conditional scan")
		}
	}); allocated != baseline {
		t.Fatalf("conditional scan allocated %g; reader baseline %g", allocated, baseline)
	}
	r := NewReader(b, "TEST", 128)
	if r.StringIn(16, "different") || r.Done() != nil || r.allocated != 5 {
		t.Fatal("unmatched tag changed validation or virtual allocation")
	}
	for size := range len(b) {
		r := NewReader(b[:size], "TEST", 128)
		if r.StringIn(16, "", "known") || r.Done() == nil {
			t.Fatalf("truncated discriminator matched: %d", size)
		}
	}
	r = NewReader(b, "TEST", 128)
	r.Allocate(AllocationLimit)
	if r.StringIn(16, "known") || !errors.Is(r.Err(), ErrCount) {
		t.Fatal("conditional string bypassed allocation limit")
	}
}
