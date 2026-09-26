package level_test

import (
	"bytes"
	"testing"

	"github.com/karty-game/karty-sdk/format/level"
)

func TestWrapModuleIsDeterministicAndContainsEnvelope(t *testing.T) {
	t.Parallel()

	envelope, err := level.Encode([]byte(`{"schema":"test@1"}`), []level.SourceEntry{
		{Name: "map", Kind: level.EntryData, Data: []byte{1, 2, 3}},
	})
	if err != nil {
		t.Fatal(err)
	}

	first, err := level.WrapModule(envelope)
	if err != nil {
		t.Fatal(err)
	}

	second, err := level.WrapModule(envelope)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(first, second) || !bytes.HasPrefix(first, []byte("\x00asm")) || !bytes.Contains(first, envelope) {
		t.Fatal("WrapModule() did not produce deterministic core Wasm containing the envelope")
	}
}

func TestWrapModuleRejectsInvalidEnvelope(t *testing.T) {
	t.Parallel()

	if _, err := level.WrapModule([]byte("KTYL")); err == nil {
		t.Fatal("WrapModule() accepted an invalid envelope")
	}
}
