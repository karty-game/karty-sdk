package cartridge_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/karty-game/karty-sdk/format/cartridge"
)

func TestManifestRoundTripInsideGameCartridge(t *testing.T) {
	t.Parallel()

	want := cartridge.Manifest{
		ProjectName: "pong",
		Compiler:    "tinygo",
		Width:       960,
		Height:      540,
		Levels: []cartridge.LevelDependency{
			{Name: "levels.left", Kind: "arena", ContentSHA256: strings.Repeat("0", 64), Size: 128, EnvelopeVersion: 1},
			{
				Name: "levels.right", Kind: "arena",
				ContentSHA256: "abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd",
				Size:          256, EnvelopeVersion: 1,
			},
		},
	}

	encoded, err := cartridge.EncodeManifest(want)
	if err != nil {
		t.Fatal(err)
	}

	wasm, err := cartridge.EmbedSection([]byte("\x00asm\x01\x00\x00\x00"), cartridge.ManifestSectionName, encoded)
	if err != nil {
		t.Fatal(err)
	}

	extracted, err := cartridge.ExtractSection(wasm, cartridge.ManifestSectionName)
	if err != nil {
		t.Fatal(err)
	}

	got, err := cartridge.DecodeManifest(extracted)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decoded manifest = %+v, want %+v", got, want)
	}
}

func TestManifestRejectsMalformedData(t *testing.T) {
	t.Parallel()

	validHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	for name, manifest := range map[string]cartridge.Manifest{
		"unsorted levels": {
			ProjectName: "pong", Compiler: "tinygo", Width: 640, Height: 360,
			Levels: []cartridge.LevelDependency{
				{Name: "z", Kind: "level", ContentSHA256: validHash, Size: 1, EnvelopeVersion: 1},
				{Name: "a", Kind: "level", ContentSHA256: validHash, Size: 1, EnvelopeVersion: 1},
			},
		},
		"uppercase hash": {
			ProjectName: "pong", Compiler: "tinygo", Width: 640, Height: 360,
			Levels: []cartridge.LevelDependency{{Name: "a", Kind: "level", ContentSHA256: "A" + validHash[1:], Size: 1, EnvelopeVersion: 1}},
		},
	} {
		if _, err := cartridge.EncodeManifest(manifest); err == nil {
			t.Errorf("EncodeManifest() accepted %s", name)
		}
	}

	if _, err := cartridge.DecodeManifest([]byte("not a manifest")); err == nil {
		t.Fatal("DecodeManifest() accepted malformed bytes")
	}
}
