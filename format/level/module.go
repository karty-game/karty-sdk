package level

import "crypto/sha256"

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

	return wrapValidatedEnvelope(envelope), nil
}

// The caller has validated the envelope using Decode before constructing Wasm.
func wrapValidatedEnvelope(envelope []byte) []byte {
	return append(passiveModulePrefix(len(envelope)), envelope...)
}

// PassiveModuleIdentity returns the SHA-256 and size of canonical WrapModule
// output without constructing a second copy of its payload. The envelope is
// structurally decoded; no authored geometry is inspected.
func PassiveModuleIdentity(envelope []byte) ([sha256.Size]byte, uint64, error) {
	if _, err := Decode(envelope); err != nil {
		return [sha256.Size]byte{}, 0, err
	}
	prefix := passiveModulePrefix(len(envelope))
	digest := sha256.New()
	_, _ = digest.Write(prefix)
	_, _ = digest.Write(envelope)
	var result [sha256.Size]byte
	digest.Sum(result[:0])
	return result, uint64(len(prefix) + len(envelope)), nil
}

// Canonical modules end with their sole envelope payload. Construct only the
// bounded ABI/header prefix when inspecting or hashing an existing module.
func passiveModulePrefix(envelopeSize int) []byte {
	module := []byte{0, 0x61, 0x73, 0x6d, 1, 0, 0, 0}
	appendSection := func(identifier byte, payload []byte) {
		module = append(module, identifier)
		module = appendUnsignedLEB(module, len(payload))
		module = append(module, payload...)
	}

	appendSection(1, []byte{1, 0x60, 0, 1, 0x7f})
	appendSection(3, []byte{3, 0, 0, 0})

	pages := (moduleDataOffset + envelopeSize + wasmPageSize - 1) / wasmPageSize
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

	for _, value := range []int{int(EnvelopeVersion), moduleDataOffset, envelopeSize} {
		body := appendSignedLEB([]byte{0, 0x41}, value)
		body = append(body, 0x0b)
		code = appendUnsignedLEB(code, len(body))
		code = append(code, body...)
	}

	appendSection(10, code)

	data := appendSignedLEB([]byte{1, 0, 0x41}, moduleDataOffset)
	data = append(data, 0x0b)
	data = appendUnsignedLEB(data, envelopeSize)
	module = append(module, 11)
	module = appendUnsignedLEB(module, len(data)+envelopeSize)
	module = append(module, data...)

	return module
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
