// Package binwire implements bounded, canonical primitives for generated typed
// format codecs. It has no reflection or dynamic serialization.
package binwire

import (
	"encoding/binary"
	"errors"
	"math"
	"unicode/utf8"
)

const HeaderSize = 16
const AllocationLimit = 32 * 1024 * 1024

var (
	ErrWire    = errors.New("invalid binary payload")
	ErrVersion = errors.New("unsupported binary version")
	ErrCount   = errors.New("binary count or allocation bound")
)

type Writer struct {
	data    []byte
	maximum int
	yieldAt int
	err     error
}

func NewWriter(magic string, maximum int) *Writer {
	w := &Writer{maximum: maximum}
	if len(magic) != 4 || maximum < HeaderSize {
		w.err = ErrWire
		return w
	}
	w.data = make([]byte, HeaderSize)
	copy(w.data, magic)
	binary.LittleEndian.PutUint16(w.data[4:], 1)
	return w
}

func (w *Writer) put(size int) []byte {
	if w.err != nil {
		return nil
	}
	if size < 0 || size > w.maximum-len(w.data) {
		w.err = ErrCount
		return nil
	}
	at := len(w.data)
	w.yieldAt = checkpoint(at, w.yieldAt)
	w.data = append(w.data, make([]byte, size)...)
	return w.data[at:]
}
func (w *Writer) U8(v uint8) {
	if b := w.put(1); b != nil {
		b[0] = v
	}
}
func (w *Writer) U16(v uint16) {
	if b := w.put(2); b != nil {
		binary.LittleEndian.PutUint16(b, v)
	}
}
func (w *Writer) U32(v uint32) {
	if b := w.put(4); b != nil {
		binary.LittleEndian.PutUint32(b, v)
	}
}
func (w *Writer) I32(v int) {
	if int64(v) < math.MinInt32 || int64(v) > math.MaxInt32 {
		w.err = ErrCount
		return
	}
	w.U32(uint32(int32(v)))
}
func (w *Writer) U64(v uint64) {
	if b := w.put(8); b != nil {
		binary.LittleEndian.PutUint64(b, v)
	}
}
func (w *Writer) F64(v float64) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		w.err = ErrWire
		return
	}
	if v == 0 {
		v = 0
	}
	w.U64(math.Float64bits(v))
}
func (w *Writer) F32(v float32) {
	if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
		w.err = ErrWire
		return
	}
	if v == 0 {
		v = 0
	}
	w.U32(math.Float32bits(v))
}
func (w *Writer) Bool(v bool) {
	if v {
		w.U8(1)
	} else {
		w.U8(0)
	}
}
func (w *Writer) String(v string, maximum int) {
	if len(v) > maximum || !utf8.ValidString(v) {
		w.err = ErrCount
		return
	}
	w.U32(uint32(len(v)))
	copy(w.put(len(v)), v)
}
func (w *Writer) Count(length int, absent bool, maximum int) {
	if length < 0 || length > maximum {
		w.err = ErrCount
		return
	}
	if absent {
		w.U32(0)
	} else {
		w.U32(uint32(length) + 1)
	}
}
func (w *Writer) Err() error { return w.err }

func (w *Writer) Bytes() ([]byte, error) {
	if w.err != nil {
		return nil, w.err
	}
	binary.LittleEndian.PutUint32(w.data[8:], uint32(len(w.data)-HeaderSize))
	return w.data, nil
}

type Reader struct {
	data      []byte
	at        int
	allocated uint64
	yieldAt   int
	err       error
}

func NewReader(data []byte, magic string, maximum int) *Reader {
	r := &Reader{data: data, at: HeaderSize}
	if len(data) < HeaderSize || len(data) > maximum || len(magic) != 4 || string(data[:4]) != magic {
		r.err = ErrWire
		return r
	}
	if binary.LittleEndian.Uint16(data[4:]) != 1 {
		r.err = ErrVersion
		return r
	}
	if binary.LittleEndian.Uint16(data[6:]) != 0 || binary.LittleEndian.Uint32(data[12:]) != 0 ||
		uint64(binary.LittleEndian.Uint32(data[8:])) != uint64(len(data)-HeaderSize) {
		r.err = ErrWire
	}
	return r
}
func (r *Reader) take(size int) []byte {
	if r.err != nil {
		return nil
	}
	if size < 0 || size > len(r.data)-r.at {
		r.err = ErrWire
		return nil
	}
	b := r.data[r.at : r.at+size]
	r.at += size
	r.yieldAt = checkpoint(r.at, r.yieldAt)
	return b
}
func (r *Reader) U8() uint8 {
	if b := r.take(1); b != nil {
		return b[0]
	}
	return 0
}
func (r *Reader) U16() uint16 {
	if b := r.take(2); b != nil {
		return binary.LittleEndian.Uint16(b)
	}
	return 0
}
func (r *Reader) U32() uint32 {
	if b := r.take(4); b != nil {
		return binary.LittleEndian.Uint32(b)
	}
	return 0
}
func (r *Reader) I32() int { return int(int32(r.U32())) }
func (r *Reader) U64() uint64 {
	if b := r.take(8); b != nil {
		return binary.LittleEndian.Uint64(b)
	}
	return 0
}
func (r *Reader) F64() float64 {
	bits := r.U64()
	v := math.Float64frombits(bits)
	if math.IsNaN(v) || math.IsInf(v, 0) || (v == 0 && bits != 0) {
		r.err = ErrWire
	}
	return v
}
func (r *Reader) F32() float32 {
	bits := r.U32()
	v := math.Float32frombits(bits)
	if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) || (v == 0 && bits != 0) {
		r.err = ErrWire
	}
	return v
}
func (r *Reader) Bool() bool {
	v := r.U8()
	if v > 1 {
		r.err = ErrWire
	}
	return v == 1
}
func (r *Reader) Allocate(size uint64) {
	if size > AllocationLimit-r.allocated {
		r.err = ErrCount
		return
	}
	r.allocated += size
}
func (r *Reader) text(maximum int) []byte {
	size := r.U32()
	if uint64(size) > uint64(maximum) || uint64(size) > uint64(len(r.data)-min(r.at, len(r.data))) {
		r.err = ErrCount
		return nil
	}
	b := r.take(int(size))
	if !utf8.Valid(b) {
		r.err = ErrWire
		return nil
	}
	r.Allocate(uint64(size))
	if r.err != nil {
		return nil
	}
	return b
}
func (r *Reader) String(maximum int) string { return string(r.text(maximum)) }
func (r *Reader) SkipString(maximum int)    { r.text(maximum) }

// StringIn scans one bounded string without allocating a decoded copy. It
// supports conditional typed fields whose presence is selected by a known tag.
func (r *Reader) StringIn(maximum int, choices ...string) bool {
	value := r.text(maximum)
	if r.err != nil {
		return false
	}
	for _, choice := range choices {
		if string(value) == choice {
			return true
		}
	}
	return false
}
func (r *Reader) Count(maximum int, minimumBytes, itemAllocation uint64) (int, bool) {
	word := r.U32()
	if r.err != nil || word == 0 {
		return 0, false
	}
	count := uint64(word - 1)
	if count > uint64(maximum) || minimumBytes == 0 || count > uint64(len(r.data)-r.at)/minimumBytes ||
		itemAllocation > 0 && count > AllocationLimit/itemAllocation {
		r.err = ErrCount
		return 0, false
	}
	r.Allocate(count * itemAllocation)
	if r.err != nil {
		return 0, false
	}
	return int(count), true
}
func (r *Reader) Err() error { return r.err }
func (r *Reader) Done() error {
	if r.err != nil {
		return r.err
	}
	if r.at != len(r.data) {
		return ErrWire
	}
	return nil
}
