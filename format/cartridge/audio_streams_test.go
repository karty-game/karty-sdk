package cartridge

import (
	"bytes"
	"strings"
	"testing"

	"github.com/karty-game/karty-sdk/format/asset"
)

func TestAudioStreamCatalogRoundTrip(t *testing.T) {
	t.Parallel()
	if MaxAudioStreamPayloadSize != asset.MaxEncodedAudioStreamBytes ||
		MaxAudioStreamSize != MaxAudioStreamPayloadSize+MediaEnvelopeHeaderSize {
		t.Fatal("cartridge payload and stored-size bounds differ")
	}
	digest := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	streams := []AudioStream{
		{ID: 1, Name: "theme", Kind: AudioStreamMusic, Channels: 2, SampleRate: 48_000, Frames: 48_000, Size: 100, SHA256: digest, Chunks: []string{digest}},
		{ID: 1, Name: "rain", Kind: AudioStreamEnvironment, Channels: 1, SampleRate: 24_000, Frames: 24_000, Size: 100, SHA256: digest, Chunks: []string{digest}},
	}
	encoded, err := EncodeAudioStreams(streams)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeAudioStreams(encoded)
	if err != nil || len(decoded) != 2 || decoded[1].Name != "rain" || decoded[0].Path() != "content/"+digest+".kaud" {
		t.Fatalf("decoded = %+v, error = %v", decoded, err)
	}
}

func TestAudioStreamCatalogRejectsInvalidEntries(t *testing.T) {
	t.Parallel()
	digest := strings.Repeat("a", 64)
	valid := AudioStream{ID: 1, Name: "theme", Kind: AudioStreamMusic, Channels: 2, SampleRate: 48_000,
		Frames: 48_000, Size: 100, SHA256: digest, Chunks: []string{digest}}

	for name, mutate := range map[string]func(*AudioStream){
		"ID":          func(stream *AudioStream) { stream.ID = 2 },
		"name":        func(stream *AudioStream) { stream.Name = "" },
		"kind":        func(stream *AudioStream) { stream.Kind = "voice" },
		"channels":    func(stream *AudioStream) { stream.Channels = 3 },
		"sample rate": func(stream *AudioStream) { stream.SampleRate = 96_000 },
		"zero frames": func(stream *AudioStream) { stream.Frames = 0 },
		"duration": func(stream *AudioStream) {
			stream.Frames = uint32(stream.SampleRate*asset.MaxAudioStreamDurationSeconds + 1)
		},
		"zero size":    func(stream *AudioStream) { stream.Size = 0 },
		"size":         func(stream *AudioStream) { stream.Size = MaxAudioStreamSize + 1 },
		"file digest":  func(stream *AudioStream) { stream.SHA256 = "../escape" },
		"chunk count":  func(stream *AudioStream) { stream.Size = AudioStreamChunkSize + 1 },
		"chunk digest": func(stream *AudioStream) { stream.Chunks[0] = "bad" },
	} {
		t.Run(name, func(t *testing.T) {
			entry := valid
			entry.Chunks = append([]string(nil), valid.Chunks...)
			mutate(&entry)
			if _, err := EncodeAudioStreams([]AudioStream{entry}); err == nil {
				t.Fatal("accepted invalid audio stream")
			}
		})
	}
}

func TestAudioStreamCatalogRequiresCanonicalOrdering(t *testing.T) {
	t.Parallel()
	digest := strings.Repeat("b", 64)
	entry := func(id uint32, name string, kind AudioStreamKind) AudioStream {
		return AudioStream{ID: id, Name: name, Kind: kind, Channels: 1, SampleRate: 24_000,
			Frames: 24_000, Size: 10, SHA256: digest, Chunks: []string{digest}}
	}
	for name, streams := range map[string][]AudioStream{
		"duplicate names": {entry(1, "same", AudioStreamMusic), entry(2, "same", AudioStreamMusic)},
		"name order":      {entry(1, "z", AudioStreamMusic), entry(2, "a", AudioStreamMusic)},
		"kind order":      {entry(1, "rain", AudioStreamEnvironment), entry(1, "theme", AudioStreamMusic)},
		"kind-local IDs":  {entry(1, "theme", AudioStreamMusic), entry(2, "rain", AudioStreamEnvironment)},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := EncodeAudioStreams(streams); err == nil {
				t.Fatal("accepted noncanonical catalog")
			}
		})
	}
}

func TestAudioStreamCatalogRejectsMalformedJSON(t *testing.T) {
	t.Parallel()
	digest := strings.Repeat("c", 64)
	valid, err := EncodeAudioStreams([]AudioStream{{
		ID: 1, Name: "rain", Kind: AudioStreamEnvironment, Channels: 1, SampleRate: 48_000,
		Frames: 48_000, Size: 32, SHA256: digest, Chunks: []string{digest},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{
		"trailing":      append(bytes.Clone(valid), 'x'),
		"version":       bytes.Replace(valid, []byte(`"version":1`), []byte(`"version":2`), 1),
		"unknown field": bytes.Replace(valid, []byte(`"streams":`), []byte(`"extra":1,"streams":`), 1),
		"empty":         []byte(`{"version":1,"streams":[]}`),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeAudioStreams(data); err == nil {
				t.Fatal("accepted malformed catalog")
			}
		})
	}
}

func FuzzDecodeAudioStreams(f *testing.F) {
	digest := strings.Repeat("d", 64)
	valid, err := EncodeAudioStreams([]AudioStream{{
		ID: 1, Name: "theme", Kind: AudioStreamMusic, Channels: 2, SampleRate: 48_000,
		Frames: 48_000, Size: 32, SHA256: digest, Chunks: []string{digest},
	}})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(valid)
	f.Add([]byte(`{}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = DecodeAudioStreams(data)
	})
}
