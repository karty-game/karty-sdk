package level

import (
	"bytes"
	"errors"
	"fmt"
)

const MaxPassiveModuleSize = 32 * 1024 * 1024

var ErrPassiveModule = errors.New("level module is not a canonical passive module")

// ValidatePassiveModule accepts only exact WrapModule output. Run it before
// instantiating independently downloaded chunk modules: arbitrary Wasm imports,
// start functions, loops, memory growth or unbounded initial memories are never
// part of this data transport contract. Legacy level execution is unchanged.
// Validation allocates bounded canonical-module and data-construction buffers
// plus the envelope decoder's bounded entry table. It executes no instruction.
func ValidatePassiveModule(wasm []byte) error {
	_, err := DecodePassiveModule(wasm)
	return err
}

// DecodePassiveModule validates exact WrapModule output and decodes its level
// envelope without executing Wasm. The returned Raw, Metadata and payloads alias
// wasm, which the caller must retain and treat as immutable. It has the same
// input bounds, canonical ABI and errors as ValidatePassiveModule.
func DecodePassiveModule(wasm []byte) (Envelope, error) {
	if len(wasm) < 8 || len(wasm) > MaxPassiveModuleSize || !bytes.Equal(wasm[:8], []byte{0, 'a', 's', 'm', 1, 0, 0, 0}) {
		return Envelope{}, ErrPassiveModule
	}
	var envelope []byte
	sections := [...]byte{1, 3, 5, 7, 10, 11}
	position := 0
	for offset := 8; offset < len(wasm); {
		section := wasm[offset]
		if position >= len(sections) || section != sections[position] {
			return Envelope{}, ErrPassiveModule
		}
		position++
		offset++
		size, next, ok := passiveU32(wasm, offset)
		if !ok || uint64(size) > uint64(len(wasm)-next) {
			return Envelope{}, ErrPassiveModule
		}
		end := next + int(size) // Size is now bounded by the <=32 MiB input.
		if section == 11 {
			if envelope != nil {
				return Envelope{}, ErrPassiveModule
			}
			data := wasm[next:end]
			// One active segment, implicit memory zero, i32.const 1024; end.
			prefix := []byte{1, 0, 0x41, 0x80, 0x08, 0x0b}
			if len(data) < len(prefix) || !bytes.Equal(data[:len(prefix)], prefix) {
				return Envelope{}, ErrPassiveModule
			}
			length, start, ok := passiveU32(data, len(prefix))
			if !ok || length > MaxEnvelopeSize || length != uint32(len(data)-start) {
				return Envelope{}, ErrPassiveModule
			}
			envelope = data[start:]
		}
		offset = end
	}
	if envelope == nil || position != len(sections) {
		return Envelope{}, ErrPassiveModule
	}
	decoded, err := Decode(envelope)
	if err != nil {
		return Envelope{}, fmt.Errorf("passive envelope: %w: %w", ErrPassiveModule, err)
	}
	canonical := wrapValidatedEnvelope(envelope)
	if !bytes.Equal(wasm, canonical) {
		return Envelope{}, ErrPassiveModule
	}
	return decoded, nil
}

// Decode a shortest-form unsigned 32-bit LEB without allocation. Header lengths
// must be canonical before any decoder or canonical-module buffer allocation.
func passiveU32(source []byte, offset int) (uint32, int, bool) {
	var value uint32
	for index := range 5 {
		if offset >= len(source) {
			return 0, offset, false
		}
		current := source[offset]
		offset++
		if index == 4 && current > 0x0f {
			return 0, offset, false
		}
		value |= uint32(current&0x7f) << (index * 7)
		if current&0x80 == 0 {
			if index > 0 && current == 0 {
				return 0, offset, false
			}
			return value, offset, true
		}
	}
	return 0, offset, false
}
