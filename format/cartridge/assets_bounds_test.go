package cartridge_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/karty-game/karty-sdk/format/cartridge"
)

func TestAssetsRejectWireCountsBeforeAllocation(t *testing.T) {
	encoded, err := cartridge.EncodeAssets([]cartridge.Asset{{Name: "a", Bytes: []byte{1}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, count := range []uint32{2, cartridge.MaxAssetCount + 1, 1 << 31, 0xffffffff} {
		data := bytes.Clone(encoded)
		binary.LittleEndian.PutUint32(data[8:12], count)
		if _, err := cartridge.DecodeAssets(data); !errors.Is(err, cartridge.ErrAssets) {
			t.Fatalf("count %d: %v", count, err)
		}
	}
	oversized := make([]cartridge.Asset, cartridge.MaxAssetCount+1)
	if _, err := cartridge.EncodeAssets(oversized); !errors.Is(err, cartridge.ErrAssets) {
		t.Fatal(err)
	}
	if allocations := testing.AllocsPerRun(10, func() { _, _ = cartridge.EncodeAssets(oversized) }); allocations != 0 {
		t.Fatalf("oversized encode allocated %g times", allocations)
	}
}

func TestCustomSectionLengthBounds(t *testing.T) {
	header := []byte{0, 'a', 's', 'm', 1, 0, 0, 0}
	for name, suffix := range map[string][]byte{
		"unsigned maximum":      {0, 0xff, 0xff, 0xff, 0xff, 0x0f},
		"above u32":             {0, 0x80, 0x80, 0x80, 0x80, 0x10},
		"overlong":              {0, 0x80, 0x80, 0x80, 0x80, 0x80, 0},
		"truncated":             {0, 0x80},
		"name unsigned maximum": {0, 5, 0xff, 0xff, 0xff, 0xff, 0x0f},
		"name above u32":        {0, 5, 0x80, 0x80, 0x80, 0x80, 0x10},
	} {
		t.Run(name, func(t *testing.T) {
			wasm := append(bytes.Clone(header), suffix...)
			if _, err := cartridge.ExtractSection(wasm, "x"); !errors.Is(err, cartridge.ErrSection) {
				t.Fatal(err)
			}
		})
	}
	// Wasm permits padded u32 encodings; rejecting overflow must not reject them.
	wasm := append(bytes.Clone(header), 0, 0x83, 0x80, 0x80, 0x80, 0, 1, 'x', 42)
	data, err := cartridge.ExtractSection(wasm, "x")
	if err != nil || !bytes.Equal(data, []byte{42}) {
		t.Fatalf("padded section: %v %v", data, err)
	}
}

func FuzzDecodeAssets(f *testing.F) {
	encoded, err := cartridge.EncodeAssets([]cartridge.Asset{{Name: "a", Bytes: []byte{1}}})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(encoded)
	oversized := bytes.Clone(encoded)
	binary.LittleEndian.PutUint32(oversized[8:12], 0xffffffff)
	f.Add(oversized)
	f.Add([]byte("KTYA"))
	f.Fuzz(func(t *testing.T, data []byte) {
		assets, err := cartridge.DecodeAssets(data)
		if err != nil {
			return
		}
		canonical, err := cartridge.EncodeAssets(assets)
		if err != nil || !bytes.Equal(data, canonical) {
			t.Fatalf("round trip: %v", err)
		}
	})
}

func FuzzExtractSection(f *testing.F) {
	f.Add([]byte{0, 'a', 's', 'm', 1, 0, 0, 0, 0, 3, 1, 'x', 42})
	f.Add([]byte{0, 'a', 's', 'm', 1, 0, 0, 0, 0, 0xff, 0xff, 0xff, 0xff, 0x0f})
	f.Add([]byte{0, 'a', 's', 'm', 1, 0, 0, 0, 0, 5, 0x80, 0x80, 0x80, 0x80, 0x10})
	f.Fuzz(func(t *testing.T, data []byte) { _, _ = cartridge.ExtractSection(data, "x") })
}
