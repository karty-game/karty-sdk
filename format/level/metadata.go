package level

import (
	"encoding/binary"
	"math"
	"slices"
	"unicode/utf8"
)

const (
	MetadataVersion    = uint16(2)
	MaxMetadataDepth   = 16
	MaxMetadataNodes   = 2048
	MaxMetadataKeySize = 1024
)

// ValueKind identifies the active field in a metadata Value.
type ValueKind uint8

const (
	ValueKindNull   ValueKind = 0
	ValueKindBool   ValueKind = 1
	ValueKindInt    ValueKind = 3
	ValueKindUint   ValueKind = 4
	ValueKindFloat  ValueKind = 5
	ValueKindString ValueKind = 6
	ValueKindBytes  ValueKind = 7
	ValueKindObject ValueKind = 8
	ValueKindArray  ValueKind = 9
)

// Metadata is a bounded, typed object. Its wire representation has sorted keys.
type Metadata map[string]Value

// Value preserves custom metadata without runtime reflection or JSON parsing.
type Value struct {
	Kind   ValueKind
	Bool   bool
	Int    int64
	Uint   uint64
	Float  float64
	String string
	Bytes  []byte
	Object Metadata
	Array  []Value
}

func NullValue() Value             { return Value{} }
func BoolValue(v bool) Value       { return Value{Kind: ValueKindBool, Bool: v} }
func IntValue(v int64) Value       { return Value{Kind: ValueKindInt, Int: v} }
func UintValue(v uint64) Value     { return Value{Kind: ValueKindUint, Uint: v} }
func FloatValue(v float64) Value   { return Value{Kind: ValueKindFloat, Float: v} }
func StringValue(v string) Value   { return Value{Kind: ValueKindString, String: v} }
func BytesValue(v []byte) Value    { return Value{Kind: ValueKindBytes, Bytes: v} }
func ObjectValue(v Metadata) Value { return Value{Kind: ValueKindObject, Object: v} }
func ArrayValue(v []Value) Value   { return Value{Kind: ValueKindArray, Array: v} }

func (v Value) AsString() (string, bool)   { return v.String, v.Kind == ValueKindString }
func (v Value) AsBool() (bool, bool)       { return v.Bool, v.Kind == ValueKindBool }
func (v Value) AsObject() (Metadata, bool) { return v.Object, v.Kind == ValueKindObject }
func (v Value) AsArray() ([]Value, bool)   { return v.Array, v.Kind == ValueKindArray }
func (v Value) AsBytes() ([]byte, bool)    { return v.Bytes, v.Kind == ValueKindBytes }
func (v Value) AsUint() (uint64, bool) {
	if v.Kind == ValueKindUint {
		return v.Uint, true
	}
	if v.Kind == ValueKindInt && v.Int >= 0 {
		return uint64(v.Int), true
	}
	return 0, false
}
func (v Value) AsInt() (int64, bool) {
	if v.Kind == ValueKindInt {
		return v.Int, true
	}
	if v.Kind == ValueKindUint && v.Uint <= math.MaxInt64 {
		return int64(v.Uint), true
	}
	return 0, false
}
func (v Value) AsFloat() (float64, bool) {
	switch v.Kind {
	case ValueKindFloat:
		return v.Float, true
	case ValueKindInt:
		return float64(v.Int), true
	case ValueKindUint:
		return float64(v.Uint), true
	default:
		return 0, false
	}
}
func (m Metadata) String(key string) (string, bool)   { return m[key].AsString() }
func (m Metadata) Uint(key string) (uint64, bool)     { return m[key].AsUint() }
func (m Metadata) Int(key string) (int64, bool)       { return m[key].AsInt() }
func (m Metadata) Object(key string) (Metadata, bool) { return m[key].AsObject() }

// EncodeMetadata creates canonical binary metadata. Nil and empty objects have
// the same representation; JSON input is deliberately unsupported.
func EncodeMetadata(metadata Metadata) ([]byte, error) {
	w := metadataWriter{data: []byte{'K', 'T', 'Y', 'M', 2, 0, 0, 0}}
	if !w.value(ObjectValue(metadata), 0) {
		return nil, ErrMetadata
	}
	return w.data, nil
}

// DecodeMetadata validates a complete payload before allocating nested values.
func DecodeMetadata(encoded []byte) (Metadata, error) {
	if !validMetadataBytes(encoded) {
		return nil, ErrMetadata
	}
	r := metadataReader{data: encoded, offset: 8, allocate: true}
	v, ok := r.value(0)
	if !ok {
		return nil, ErrMetadata
	}
	return v.Object, nil
}

// ValidateMetadata checks the binary representation without allocating values.
func ValidateMetadata(encoded []byte) error {
	if !validMetadataBytes(encoded) {
		return ErrMetadata
	}
	return nil
}

func validMetadataBytes(data []byte) bool {
	if len(data) < 13 || len(data) > MaxMetadataSize || string(data[:4]) != "KTYM" ||
		binary.LittleEndian.Uint16(data[4:6]) != MetadataVersion || binary.LittleEndian.Uint16(data[6:8]) != 0 || data[8] != 8 {
		return false
	}
	r := metadataReader{data: data, offset: 8}
	_, ok := r.value(0)
	return ok && r.offset == len(data)
}

type metadataWriter struct {
	data  []byte
	nodes int
}

func (w *metadataWriter) blob(v []byte) bool {
	if len(v) > MaxMetadataSize-4 || len(w.data) > MaxMetadataSize-4-len(v) {
		return false
	}
	w.data = binary.LittleEndian.AppendUint32(w.data, uint32(len(v)))
	w.data = append(w.data, v...)
	return true
}
func (w *metadataWriter) value(v Value, depth int) bool {
	w.nodes++
	if depth > MaxMetadataDepth || w.nodes > MaxMetadataNodes || len(w.data) >= MaxMetadataSize {
		return false
	}
	tag := byte(v.Kind)
	if v.Kind == ValueKindBool && v.Bool {
		tag = 2
	}
	w.data = append(w.data, tag)
	switch v.Kind {
	case ValueKindNull, ValueKindBool:
	case ValueKindInt, ValueKindUint, ValueKindFloat:
		if len(w.data) > MaxMetadataSize-8 {
			return false
		}
		bits := v.Uint
		if v.Kind == ValueKindInt {
			bits = uint64(v.Int)
		}
		if v.Kind == ValueKindFloat {
			if math.IsNaN(v.Float) || math.IsInf(v.Float, 0) {
				return false
			}
			bits = math.Float64bits(v.Float)
		}
		w.data = binary.LittleEndian.AppendUint64(w.data, bits)
	case ValueKindString:
		return utf8.ValidString(v.String) && w.blob([]byte(v.String))
	case ValueKindBytes:
		return w.blob(v.Bytes)
	case ValueKindObject:
		if len(v.Object) > MaxMetadataNodes-w.nodes || len(w.data) > MaxMetadataSize-4 {
			return false
		}
		keys := make([]string, 0, len(v.Object))
		for key := range v.Object {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		w.data = binary.LittleEndian.AppendUint32(w.data, uint32(len(keys)))
		for _, key := range keys {
			if len(key) < 1 || len(key) > MaxMetadataKeySize || !utf8.ValidString(key) || !w.blob([]byte(key)) ||
				!w.value(v.Object[key], depth+1) {
				return false
			}
		}
	case ValueKindArray:
		if len(v.Array) > MaxMetadataNodes-w.nodes || len(w.data) > MaxMetadataSize-4 {
			return false
		}
		w.data = binary.LittleEndian.AppendUint32(w.data, uint32(len(v.Array)))
		for _, child := range v.Array {
			if !w.value(child, depth+1) {
				return false
			}
		}
	default:
		return false
	}
	return true
}

type metadataReader struct {
	data          []byte
	offset, nodes int
	allocate      bool
}

func (r *metadataReader) take(size int) ([]byte, bool) {
	if size < 0 || size > len(r.data)-r.offset {
		return nil, false
	}
	v := r.data[r.offset : r.offset+size]
	r.offset += size
	return v, true
}
func (r *metadataReader) count() (int, bool) {
	b, ok := r.take(4)
	if !ok {
		return 0, false
	}
	n := uint64(binary.LittleEndian.Uint32(b))
	if n > uint64(len(r.data)-r.offset) {
		return 0, false
	}
	return int(n), true
}
func (r *metadataReader) blob() ([]byte, bool) {
	n, ok := r.count()
	if !ok {
		return nil, false
	}
	return r.take(n)
}
func (r *metadataReader) value(depth int) (Value, bool) {
	r.nodes++
	if depth > MaxMetadataDepth || r.nodes > MaxMetadataNodes {
		return Value{}, false
	}
	tag, ok := r.take(1)
	if !ok {
		return Value{}, false
	}
	v := Value{Kind: ValueKind(tag[0])}
	switch tag[0] {
	case 0:
	case 1, 2:
		v = BoolValue(tag[0] == 2)
	case 3, 4, 5:
		b, valid := r.take(8)
		if !valid {
			return Value{}, false
		}
		bits := binary.LittleEndian.Uint64(b)
		if tag[0] == 3 {
			v.Int = int64(bits)
		} else if tag[0] == 4 {
			v.Uint = bits
		} else {
			v.Float = math.Float64frombits(bits)
			if math.IsNaN(v.Float) || math.IsInf(v.Float, 0) {
				return Value{}, false
			}
		}
	case 6, 7:
		b, valid := r.blob()
		if !valid || (tag[0] == 6 && !utf8.Valid(b)) {
			return Value{}, false
		}
		if r.allocate {
			if tag[0] == 6 {
				v.String = string(b)
			} else {
				v.Bytes = slices.Clone(b)
			}
		}
	case 8, 9:
		n, valid := r.count()
		if !valid || n > MaxMetadataNodes-r.nodes {
			return Value{}, false
		}
		if r.allocate {
			if tag[0] == 8 {
				v.Object = make(Metadata, n)
			} else {
				v.Array = make([]Value, n)
			}
		}
		var previous []byte
		for i := 0; i < n; i++ {
			var key []byte
			if tag[0] == 8 {
				key, valid = r.blob()
				if !valid || len(key) < 1 || len(key) > MaxMetadataKeySize || !utf8.Valid(key) ||
					(i > 0 && slices.Compare(previous, key) >= 0) {
					return Value{}, false
				}
				previous = key
			}
			child, valid := r.value(depth + 1)
			if !valid {
				return Value{}, false
			}
			if r.allocate {
				if tag[0] == 8 {
					v.Object[string(key)] = child
				} else {
					v.Array[i] = child
				}
			}
		}
	default:
		return Value{}, false
	}
	return v, true
}
