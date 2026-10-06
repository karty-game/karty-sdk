package cartridge_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/karty-game/karty-sdk/format/cartridge"
)

func TestSoundsRoundTripInsideGameCartridge(t *testing.T) {
	t.Parallel()

	sounds := []cartridge.Sound{
		{
			ID:         2,
			Name:       "ui.open",
			Codec:      cartridge.SoundCodecQOA,
			Channels:   2,
			SampleRate: 48_000,
			Frames:     20,
			Bytes:      qoaFixture(2, 48_000, 20),
		},
		{
			ID:         1,
			Name:       "ball.hit",
			Codec:      cartridge.SoundCodecQOA,
			Channels:   1,
			SampleRate: 24_000,
			Frames:     1,
			Bytes:      qoaFixture(1, 24_000, 1),
		},
	}
	bundle, err := cartridge.EncodeSounds(sounds)
	if err != nil {
		t.Fatal(err)
	}
	wasm, err := cartridge.EmbedSounds([]byte("\x00asm\x01\x00\x00\x00"), bundle)
	if err != nil {
		t.Fatal(err)
	}
	got, err := cartridge.ExtractSounds(wasm)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != 1 || got[0].Name != "ball.hit" || got[1].ID != 2 || got[1].Name != "ui.open" ||
		!bytes.Equal(got[1].Bytes, sounds[0].Bytes) {
		t.Fatalf("decoded sounds = %+v", got)
	}
}

func TestSoundsRejectMalformedData(t *testing.T) {
	t.Parallel()

	valid, err := cartridge.EncodeSounds([]cartridge.Sound{{
		ID: 1, Name: "hit", Codec: cartridge.SoundCodecQOA, Channels: 1, SampleRate: 48_000, Frames: 20,
		Bytes: qoaFixture(1, 48_000, 20),
	}})
	if err != nil {
		t.Fatal(err)
	}

	for name, mutate := range map[string]func([]byte){
		"unknown codec":  func(data []byte) { binary.LittleEndian.PutUint16(data[20:22], 2) },
		"zero ID":        func(data []byte) { binary.LittleEndian.PutUint32(data[16:20], 0) },
		"reserved field": func(data []byte) { data[23] = 1 },
		"QOA metadata":   func(data []byte) { data[len(data)-1] ^= 1; data[49] = 2 },
		"truncated":      func(data []byte) { binary.LittleEndian.PutUint32(data[12:16], uint32(len(data)-1)) },
	} {
		t.Run(name, func(t *testing.T) {
			data := append([]byte(nil), valid...)
			mutate(data)
			if _, err := cartridge.DecodeSounds(data); err == nil {
				t.Fatal("DecodeSounds() accepted malformed data")
			}
		})
	}
}

func TestSoundsRejectNonCanonicalIDsAndNames(t *testing.T) {
	t.Parallel()

	qoa := qoaFixture(1, 48_000, 1)
	for name, sounds := range map[string][]cartridge.Sound{
		"IDs": {
			{ID: 2, Name: "a", Codec: cartridge.SoundCodecQOA, Channels: 1, SampleRate: 48_000, Frames: 1, Bytes: qoa},
		},
		"duplicate names": {
			{ID: 1, Name: "same", Codec: cartridge.SoundCodecQOA, Channels: 1, SampleRate: 48_000, Frames: 1, Bytes: qoa},
			{ID: 2, Name: "same", Codec: cartridge.SoundCodecQOA, Channels: 1, SampleRate: 48_000, Frames: 1, Bytes: qoa},
		},
	} {
		if _, err := cartridge.EncodeSounds(sounds); err == nil {
			t.Errorf("EncodeSounds() accepted invalid %s", name)
		}
	}
}

func FuzzSoundsDecode(f *testing.F) {
	encoded, err := cartridge.EncodeSounds([]cartridge.Sound{{
		ID: 1, Name: "hit", Codec: cartridge.SoundCodecQOA, Channels: 1, SampleRate: 48_000, Frames: 1,
		Bytes: qoaFixture(1, 48_000, 1),
	}})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(encoded)
	f.Add([]byte("KTYS"))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = cartridge.DecodeSounds(data)
	})
}

func qoaFixture(channels uint8, rate, frames uint32) []byte {
	slices := (frames + 19) / 20
	frameSize := 8 + 16*int(channels) + 8*int(slices)*int(channels)
	result := make([]byte, 8+frameSize)
	copy(result, "qoaf")
	binary.BigEndian.PutUint32(result[4:8], frames)
	header := uint64(channels)<<56 | uint64(rate)<<32 | uint64(frames)<<16 | uint64(frameSize)
	binary.BigEndian.PutUint64(result[8:16], header)

	return result
}
