package cartridge

import (
	"bytes"
	"encoding/binary"
	"math"
	"strings"
	"testing"
)

func TestMediaCatalogRejectsTruncatedBatchesAndUnboundedCounts(t *testing.T) {
	hash := strings.Repeat("a", 64)
	video, err := EncodeVideos([]Video{{ID: 1, Name: "intro", Size: 128, SHA256: hash, Chunks: []string{hash}}})
	if err != nil {
		t.Fatal(err)
	}
	audio, err := EncodeAudioStreams(
		[]AudioStream{
			{
				ID:         1,
				Name:       "theme",
				Kind:       AudioStreamMusic,
				Channels:   1,
				SampleRate: 24000,
				Frames:     24000,
				Size:       128,
				SHA256:     hash,
				Chunks:     []string{hash},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct {
		data   []byte
		decode func([]byte) error
	}{
		{video, func(data []byte) error { _, err := DecodeVideos(data); return err }},
		{audio, func(data []byte) error { _, err := DecodeAudioStreams(data); return err }},
	} {
		for size := range fixture.data {
			if fixture.decode(fixture.data[:size]) == nil {
				t.Fatalf("accepted truncated catalog%d", size)
			}
		}
		for _, mutate := range []func([]byte){
			func(data []byte) { binary.LittleEndian.PutUint32(data[8:12], math.MaxUint32) },
			func(data []byte) { binary.LittleEndian.PutUint32(data[16:20], math.MaxUint32) },
			func(data []byte) { data[len(data)-1] = 'z' },
		} {
			data := bytes.Clone(fixture.data)
			mutate(data)
			if fixture.decode(data) == nil {
				t.Fatal("accepted malformed count/final digest")
			}
		}
	}
}
