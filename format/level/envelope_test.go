package level_test

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/karty-game/karty-sdk/format/level"
)

func TestEnvelopeGoldenAndBoundedReads(t *testing.T) {
	t.Parallel()

	encoded, err := level.Encode([]byte(`{"schema":"game.map@1"}`), []level.SourceEntry{
		{Name: "spawn", Kind: level.EntryData, Data: []byte{9, 8, 7}},
		{Name: "map", Kind: level.EntryData, Data: []byte{1, 2, 3, 4}},
	})
	if err != nil {
		t.Fatal(err)
	}

	const golden = "4b54594c01000000170000000200000020000000070000005e000000000000007b22736368656d61223a2267616d652e6d61704031227d0100030000000000040000006d6170010005000400000003000000737061776e01020304090807"
	if hex.EncodeToString(encoded) != golden {
		t.Fatalf("wire ABI changed:\n got %x\nwant %s", encoded, golden)
	}

	decoded, err := level.Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}

	if string(decoded.Metadata) != `{"schema":"game.map@1"}` || len(decoded.Entries) != 2 || decoded.Entries[0].Name != "map" {
		t.Fatalf("decoded envelope = %+v", decoded)
	}

	chunk, total, found := decoded.Read("map", 1, 2)
	if !found || total != 4 || string(chunk) != string([]byte{2, 3}) {
		t.Fatalf("Read() = %v, %d, %t", chunk, total, found)
	}

	if _, total, found := decoded.Read("map", 5, 1); found || total != 4 {
		t.Fatalf("out-of-range Read() = total %d, found %t", total, found)
	}
}

func TestEnvelopeEncodeRejectsInvalidInputs(t *testing.T) {
	t.Parallel()

	for name, metadata := range map[string][]byte{
		"empty":   nil,
		"array":   []byte(`[]`),
		"invalid": []byte(`{"schema":`),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := level.Encode(metadata, nil); !errors.Is(err, level.ErrMetadata) {
				t.Fatalf("Encode() error = %v", err)
			}
		})
	}

	for name, entries := range map[string][]level.SourceEntry{
		"empty name":   {{Kind: level.EntryData}},
		"unknown kind": {{Name: "map", Kind: 9}},
		"empty data":   {{Name: "map", Kind: level.EntryData}},
		"duplicate":    {{Name: "map", Kind: level.EntryData}, {Name: "map", Kind: level.EntryData}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := level.Encode([]byte(`{}`), entries); !errors.Is(err, level.ErrEntry) {
				t.Fatalf("Encode() error = %v", err)
			}
		})
	}
}

func TestEnvelopeDecodeRejectsEveryTruncation(t *testing.T) {
	t.Parallel()

	encoded := validEnvelope(t)
	for size := range encoded {
		if _, err := level.Decode(encoded[:size]); err == nil {
			t.Fatalf("Decode() accepted truncation at %d", size)
		}
	}
}

func TestEnvelopeDecodeRejectsMalformedHeadersAndEntries(t *testing.T) {
	t.Parallel()

	tests := map[string]func([]byte){
		"magic":       func(encoded []byte) { encoded[0] = 0 },
		"version":     func(encoded []byte) { binary.LittleEndian.PutUint16(encoded[4:6], 2) },
		"flags":       func(encoded []byte) { encoded[6] = 1 },
		"reserved":    func(encoded []byte) { encoded[28] = 1 },
		"total":       func(encoded []byte) { binary.LittleEndian.PutUint32(encoded[24:28], uint32(len(encoded)-1)) },
		"entry count": func(encoded []byte) { binary.LittleEndian.PutUint32(encoded[12:16], 2) },
		"entry kind":  func(encoded []byte) { encoded[level.HeaderSize+2] = 2 },
		"entry offset": func(encoded []byte) {
			binary.LittleEndian.PutUint32(encoded[level.HeaderSize+2+4:level.HeaderSize+2+8], 1)
		},
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			encoded := validEnvelope(t)
			mutate(encoded)

			if _, err := level.Decode(encoded); err == nil {
				t.Fatal("Decode() accepted malformed envelope")
			}
		})
	}
}

func FuzzEnvelopeDecode(f *testing.F) {
	f.Add(validEnvelope(f))
	f.Add([]byte("KTYL"))
	f.Fuzz(func(t *testing.T, encoded []byte) {
		decoded, err := level.Decode(encoded)
		if err != nil {
			return
		}

		if len(decoded.Raw) != len(encoded) {
			t.Fatal("decoded envelope does not retain complete input")
		}
	})
}

type testHelper interface {
	Helper()
	Fatal(args ...any)
}

func validEnvelope(t testHelper) []byte {
	t.Helper()

	encoded, err := level.Encode([]byte(`{}`), []level.SourceEntry{{Name: "a", Kind: level.EntryData, Data: []byte{1}}})
	if err != nil {
		t.Fatal(err)
	}

	return encoded
}
