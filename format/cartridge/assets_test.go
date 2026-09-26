package cartridge_test

import (
	"bytes"
	"testing"

	"github.com/karty-game/karty-sdk/format/cartridge"
)

func TestAssetsRoundTripInsideGameCartridge(t *testing.T) {
	t.Parallel()

	bundle, err := cartridge.EncodeAssets([]cartridge.Asset{
		{Name: "sprites.player", Bytes: []byte("png")},
		{Name: "ui.panel", Bytes: []byte("panel")},
	})
	if err != nil {
		t.Fatal(err)
	}

	wasm, err := cartridge.EmbedAssets([]byte("\x00asm\x01\x00\x00\x00"), bundle)
	if err != nil {
		t.Fatal(err)
	}

	assets, err := cartridge.ExtractAssets(wasm)
	if err != nil {
		t.Fatal(err)
	}

	if len(assets) != 2 || assets[0].Name != "sprites.player" || !bytes.Equal(assets[1].Bytes, []byte("panel")) {
		t.Fatalf("decoded assets = %+v", assets)
	}
}

func TestAssetsRejectMalformedAndDuplicateData(t *testing.T) {
	t.Parallel()

	if _, err := cartridge.EncodeAssets([]cartridge.Asset{{Name: "same", Bytes: []byte{1}}, {Name: "same", Bytes: []byte{2}}}); err == nil {
		t.Fatal("EncodeAssets() accepted duplicate names")
	}

	if _, err := cartridge.ExtractAssets([]byte("not wasm")); err == nil {
		t.Fatal("ExtractAssets() accepted malformed Wasm")
	}
}
