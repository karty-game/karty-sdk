package ktx2

import "encoding/binary"

// inspectZstd checks one dictionary-free frame and exact content size. Compressed
// block payloads remain opaque here; their full validation belongs to inflation.
func inspectZstd(data []byte, expected uint64) error {
	if len(data) < 6 || binary.LittleEndian.Uint32(data) != 0xfd2fb528 {
		return ErrInvalid
	}
	descriptor := data[4]
	if descriptor&0x1b != 0 {
		return ErrInvalid
	} // No reserved bits or dictionaries.
	offset := 5
	single := descriptor&0x20 != 0
	if !single {
		if offset >= len(data) {
			return ErrInvalid
		}
		window := data[offset]
		offset++
		base := uint64(1) << (10 + uint(window>>3))
		windowSize := base + base/8*uint64(window&7)
		if windowSize > max(uint64(1024), expected) {
			return ErrInvalid
		}
	}
	flag := descriptor >> 6
	length := [...]int{0, 2, 4, 8}[flag]
	if flag == 0 && single {
		length = 1
	}
	if length == 0 || length > len(data)-offset {
		return ErrInvalid
	}
	var size uint64
	for i := range length {
		size |= uint64(data[offset+i]) << (8 * i)
	}
	if length == 2 {
		size += 256
	}
	if size != expected {
		return ErrInvalid
	}
	offset += length
	for {
		if len(data)-offset < 3 {
			return ErrInvalid
		}
		header := uint32(data[offset]) | uint32(data[offset+1])<<8 | uint32(data[offset+2])<<16
		offset += 3
		typeID, count := (header>>1)&3, header>>3
		if typeID == 3 || count > 128<<10 {
			return ErrInvalid
		}
		stored := int(count)
		if typeID == 1 {
			stored = 1
		}
		if stored > len(data)-offset {
			return ErrInvalid
		}
		offset += stored
		if header&1 != 0 {
			break
		}
	}
	if descriptor&4 != 0 {
		offset += 4
	}
	if offset != len(data) {
		return ErrInvalid
	}
	return nil
}
