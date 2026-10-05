package cartridge_test

import (
	"bytes"
	"errors"
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
	if version := uint16(encoded[4]) | uint16(encoded[5])<<8; version != 1 {
		t.Fatalf("legacy manifest version = %d, want 1", version)
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

func TestManifestV2FeaturesRoundTrip(t *testing.T) {
	t.Parallel()

	want := cartridge.Manifest{
		ProjectName: "sounds", Compiler: "tinygo", Width: 640, Height: 360,
		Features: []string{
			cartridge.FeatureAudioStreamQOAv1,
			cartridge.FeatureSoundQOAv1,
			cartridge.FeatureTextureQOIv1,
			cartridge.FeatureVideoMPEG1v1,
			cartridge.FeatureWorldMaterialAtlasV1,
			cartridge.FeatureWorldMaterialMappingV1,
			cartridge.FeatureWorldSectorsV1,
		},
	}
	encoded, err := cartridge.EncodeManifest(want)
	if err != nil {
		t.Fatal(err)
	}
	if version := uint16(encoded[4]) | uint16(encoded[5])<<8; version != 2 {
		t.Fatalf("manifest version = %d, want 2", version)
	}

	got, err := cartridge.DecodeManifest(encoded)
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
		"unknown feature": {
			ProjectName: "pong", Compiler: "tinygo", Width: 640, Height: 360,
			Features: []string{"sound/future@1"},
		},
		"unsorted features": {
			ProjectName: "pong", Compiler: "tinygo", Width: 640, Height: 360,
			Features: []string{cartridge.FeatureTextureQOIv1, cartridge.FeatureSoundQOAv1},
		},
		"duplicate atlas": {
			ProjectName: "pong", Compiler: "tinygo", Width: 640, Height: 360,
			Features: []string{cartridge.FeatureWorldMaterialAtlasV1, cartridge.FeatureWorldMaterialAtlasV1},
		},
		"unknown atlas version": {
			ProjectName: "pong", Compiler: "tinygo", Width: 640, Height: 360,
			Features: []string{"world/material-atlas@2"},
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

func TestManifestDecoderRejectsMalformedAtlasFeatures(t *testing.T) {
	t.Parallel()
	manifest := cartridge.Manifest{
		ProjectName: "world", Compiler: "tinygo", Width: 640, Height: 360,
		Features: []string{cartridge.FeatureWorldMaterialAtlasV1},
	}
	encoded, err := cartridge.EncodeManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	unknown := bytes.Replace(encoded, []byte(cartridge.FeatureWorldMaterialAtlasV1), []byte("world/material-atlas@2"), 1)
	if _, err := cartridge.DecodeManifest(unknown); !errors.Is(err, cartridge.ErrManifest) {
		t.Fatal("decoded unsupported atlas capability")
	}
	// Keep the existing two-feature count while replacing the feature body
	// with two identical complete length-prefixed atlas features.
	manifest.Features = []string{cartridge.FeatureWorldMaterialAtlasV1, cartridge.FeatureWorldSectorsV1}
	encoded, err = cartridge.EncodeManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	start := 24 + len(manifest.ProjectName) + len(manifest.Compiler)
	feature := encoded[start : start+2+len(cartridge.FeatureWorldMaterialAtlasV1)]
	duplicate := append(bytes.Clone(encoded[:start]), feature...)
	duplicate = append(duplicate, feature...)
	if _, err := cartridge.DecodeManifest(duplicate); !errors.Is(err, cartridge.ErrManifest) {
		t.Fatal("decoded duplicate atlas capabilities")
	}
}
