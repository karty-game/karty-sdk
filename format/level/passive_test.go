package level

import (
	"bytes"
	"errors"
	"slices"
	"testing"
)

func passiveFixture(t testing.TB) []byte {
	t.Helper()
	metadata, err := EncodeMetadata(Metadata{"kartyWorldChunk": StringValue("karty.world-chunks@1"), "chunkID": UintValue(0)})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := Encode(
		metadata,
		[]SourceEntry{{Name: "@world/chunks/mesh", Kind: EntryData, Data: []byte("mesh")}},
	)
	if err != nil {
		t.Fatal(err)
	}
	module, err := WrapModule(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return module
}

func replacePassiveSection(t testing.TB, module []byte, kind byte, replacement []byte) []byte {
	t.Helper()
	for offset := 8; offset < len(module); {
		start := offset
		section := module[offset]
		offset++
		size, next, ok := passiveU32(module, offset)
		if !ok {
			t.Fatal("invalid fixture")
		}
		end := next + int(size)
		if section == kind {
			result := slices.Clone(module[:start])
			result = append(result, section)
			result = appendUnsignedLEB(result, len(replacement))
			result = append(result, replacement...)
			return append(result, module[end:]...)
		}
		offset = end
	}
	t.Fatal("missing fixture section")
	return nil
}

func TestValidatePassiveModuleAcceptsCompilerBytes(t *testing.T) {
	module := passiveFixture(t)
	before := slices.Clone(module)
	if err := ValidatePassiveModule(module); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, module) {
		t.Fatal("validator changed module ownership")
	}
	decoded, err := DecodePassiveModule(module)
	if err != nil {
		t.Fatal(err)
	}
	start := bytes.Index(module, []byte("KTYL"))
	if start < 0 || &decoded.Raw[0] != &module[start] || !bytes.Equal(decoded.Raw, module[start:]) {
		t.Fatal("decoded envelope does not borrow the validated module data")
	}
	data, total, found := decoded.Read("@world/chunks/mesh", 0, MaxEntrySize)
	if !found || string(data) != "mesh" || total != 4 {
		t.Fatal("decoded envelope lost entry identity or payload")
	}
	if allocations := testing.AllocsPerRun(10, func() {
		if err := ValidatePassiveModule(module); err != nil {
			panic(err)
		}
	}); allocations > 100 {
		t.Fatalf("unexpected unbounded validation allocations: %g", allocations)
	}
}

// This mutation is valid core Wasm: a fourth void function loops forever when
// used as a start function, while the original three level exports remain valid.
func moduleWithLoopingStart(t testing.TB, module []byte) []byte {
	t.Helper()
	module = replacePassiveSection(t, module, 1, []byte{2, 0x60, 0, 1, 0x7f, 0x60, 0, 0})
	module = replacePassiveSection(t, module, 3, []byte{4, 0, 0, 0, 1})
	for offset := 8; offset < len(module); {
		start := offset
		section := module[offset]
		offset++
		size, next, ok := passiveU32(module, offset)
		if !ok {
			t.Fatal("invalid fixture")
		}
		end := next + int(size)
		if section == 10 {
			code := slices.Clone(module[next:end])
			code[0] = 4
			code = append(code, 7, 0, 0x03, 0x40, 0x0c, 0, 0x0b, 0x0b)
			module = replacePassiveSection(t, module, 10, code)
			result := slices.Clone(module[:start])
			result = append(result, 8, 1, 3)
			return append(result, module[start:]...)
		}
		offset = end
	}
	t.Fatal("missing code section")
	return nil
}

func TestValidatePassiveModuleRejectsExecutableAndUnboundedModules(t *testing.T) {
	module := passiveFixture(t)
	// Keep a valid envelope in each mutation: envelope validity alone cannot
	// authorize module execution or guarantee bounded memory and work.
	loopCode := []byte{3, 7, 0, 0x03, 0x7f, 0x0c, 0, 0x0b, 0x0b, 4, 0, 0x41, 0, 0x0b, 4, 0, 0x41, 0, 0x0b}
	mutations := map[string][]byte{
		"start section": moduleWithLoopingStart(t, module),
		"loop export":   replacePassiveSection(t, module, 10, loopCode),
		"memory grow": replacePassiveSection(
			t,
			module,
			10,
			[]byte{3, 6, 0, 0x41, 1, 0x40, 0, 0x0b, 4, 0, 0x41, 0, 0x0b, 4, 0, 0x41, 0, 0x0b},
		),
		"huge initial memory":         replacePassiveSection(t, module, 5, []byte{1, 0, 0x80, 0x80, 4}),
		"import":                      append(slices.Clone(module), 2, 1, 0),
		"custom section":              append(slices.Clone(module), 0, 1, 0),
		"wrong wasm version":          append([]byte{0, 'a', 's', 'm', 2, 0, 0, 0}, module[8:]...),
		"duplicate data":              replacePassiveSection(t, module, 11, []byte{2, 0, 0x41, 0x80, 0x08, 0x0b, 0}),
		"truncated":                   slices.Clone(module[:len(module)-1]),
		"noncanonical section length": append(append(slices.Clone(module[:9]), module[9]|0x80, 0), module[10:]...),
		"overflow section length":     append(slices.Clone(module[:9]), 0xff, 0xff, 0xff, 0xff, 0x7f),
	}
	for name, mutated := range mutations {
		t.Run(name, func(t *testing.T) {
			if err := ValidatePassiveModule(mutated); !errors.Is(err, ErrPassiveModule) {
				t.Fatalf("unsafe module accepted: %v", err)
			}
			if decoded, err := DecodePassiveModule(mutated); !errors.Is(err, ErrPassiveModule) || decoded.Raw != nil {
				t.Fatalf("unsafe module exposed an envelope: %v", err)
			}
		})
	}
	for length := 0; length < 8; length++ {
		if err := ValidatePassiveModule(module[:length]); err == nil {
			t.Fatal("short header accepted")
		}
		if decoded, err := DecodePassiveModule(module[:length]); !errors.Is(err, ErrPassiveModule) || decoded.Raw != nil {
			t.Fatal("short header exposed an envelope")
		}
	}
	oversized := make([]byte, MaxPassiveModuleSize+1)
	copy(oversized, module)
	if err := ValidatePassiveModule(oversized); !errors.Is(err, ErrPassiveModule) {
		t.Fatal("oversized input accepted")
	}
	if decoded, err := DecodePassiveModule(oversized); !errors.Is(err, ErrPassiveModule) || decoded.Raw != nil {
		t.Fatal("oversized input exposed an envelope")
	}
}

func TestValidatePassiveModuleRejectsMalformedEnvelope(t *testing.T) {
	module := passiveFixture(t)
	start := bytes.Index(module, []byte("KTYL"))
	if start < 0 {
		t.Fatal("fixture envelope missing")
	}
	module[start] = 'X'
	if err := ValidatePassiveModule(module); !errors.Is(err, ErrPassiveModule) {
		t.Fatal("malformed data envelope accepted")
	}
	if decoded, err := DecodePassiveModule(module); !errors.Is(err, ErrPassiveModule) || !errors.Is(err, ErrHeader) || decoded.Raw != nil {
		t.Fatalf("malformed data exposed an envelope or lost its underlying error: %v", err)
	}
}
