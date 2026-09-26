package level

const (
	moduleDataOffset = 1024
	wasmPageSize     = 65536
)

// WrapModule places a validated KTYL envelope in a minimal passive core-Wasm
// module exposing the three level-cartridge ABI functions and memory.
func WrapModule(envelope []byte) ([]byte, error) {
	if _, err := Decode(envelope); err != nil {
		return nil, err
	}

	module := []byte{0, 0x61, 0x73, 0x6d, 1, 0, 0, 0}
	appendSection := func(identifier byte, payload []byte) {
		module = append(module, identifier)
		module = appendUnsignedLEB(module, len(payload))
		module = append(module, payload...)
	}

	appendSection(1, []byte{1, 0x60, 0, 1, 0x7f})
	appendSection(3, []byte{3, 0, 0, 0})

	pages := (moduleDataOffset + len(envelope) + wasmPageSize - 1) / wasmPageSize
	appendSection(5, appendUnsignedLEB([]byte{1, 0}, pages))

	exports := []byte{4}
	for index, name := range []string{
		"karty_level_abi_version",
		"karty_level_data_pointer",
		"karty_level_data_length",
	} {
		exports = appendModuleName(exports, name)
		exports = append(exports, 0, byte(index))
	}

	exports = appendModuleName(exports, "memory")
	exports = append(exports, 2, 0)
	appendSection(7, exports)

	code := []byte{3}

	for _, value := range []int{int(EnvelopeVersion), moduleDataOffset, len(envelope)} {
		body := appendSignedLEB([]byte{0, 0x41}, value)
		body = append(body, 0x0b)
		code = appendUnsignedLEB(code, len(body))
		code = append(code, body...)
	}

	appendSection(10, code)

	data := appendSignedLEB([]byte{1, 0, 0x41}, moduleDataOffset)
	data = append(data, 0x0b)
	data = appendUnsignedLEB(data, len(envelope))
	data = append(data, envelope...)
	appendSection(11, data)

	return module, nil
}

func appendModuleName(destination []byte, name string) []byte {
	destination = appendUnsignedLEB(destination, len(name))

	return append(destination, name...)
}

func appendSignedLEB(destination []byte, value int) []byte {
	for {
		current := byte(value & 0x7f)
		value >>= 7

		done := value == 0 && current&0x40 == 0
		if !done {
			current |= 0x80
		}

		destination = append(destination, current)
		if done {
			return destination
		}
	}
}

func appendUnsignedLEB(destination []byte, value int) []byte {
	for value >= 0x80 {
		destination = append(destination, byte(value)|0x80)
		value >>= 7
	}

	return append(destination, byte(value))
}
