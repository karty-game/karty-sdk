package ktx2

import (
	"bytes"
	"encoding/binary"
	"unicode/utf8"
)

// Restrict metadata to declarations that cannot change our shader channel
// interpretation. Arbitrary swizzles and alternate orientations are rejected.
func inspectMetadata(data []byte) error {
	seen := make(map[string]bool)
	for len(data) != 0 {
		if len(data) < 4 {
			return ErrInvalid
		}
		n := uint64(binary.LittleEndian.Uint32(data))
		data = data[4:]
		if n == 0 || n > uint64(len(data)) {
			return ErrInvalid
		}
		entry := data[:n]
		padding := (4 - n%4) % 4
		if padding > uint64(len(data))-n || !allZero(data[n:n+padding]) {
			return ErrInvalid
		}
		data = data[n+padding:]
		key, value, ok := bytes.Cut(entry, []byte{0})
		if !ok || len(key) == 0 || !utf8.Valid(key) || seen[string(key)] {
			return ErrInvalid
		}
		seen[string(key)] = true
		if !validMetadataValue(string(key), value) {
			return ErrInvalid
		}
	}
	return nil
}

func validMetadataValue(key string, value []byte) bool {
	switch key {
	case "KTXorientation":
		return bytes.Equal(value, []byte{'r', 'd', 0})
	case "KTXswizzle":
		return bytes.Equal(value, []byte{'r', 'g', 'b', 'a', 0})
	case "KTXwriter", "KTXwriterScParams":
		return len(value) != 0 && value[len(value)-1] == 0 &&
			utf8.Valid(value[:len(value)-1]) && bytes.IndexByte(value[:len(value)-1], 0) < 0
	default:
		return false
	}
}
