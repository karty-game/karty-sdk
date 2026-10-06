package qoa_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"

	"github.com/karty-game/karty-sdk/codec/qoa"
)

func TestStreamDecoderMatchesCompleteDecodeWithSmallReads(t *testing.T) {
	t.Parallel()
	samples := make([]int16, 12_345*2)
	for index := range samples {
		samples[index] = int16(index*31%20_000 - 10_000)
	}
	encoded, metadata, err := qoa.Encode(samples, 2, 48_000)
	if err != nil {
		t.Fatal(err)
	}
	_, complete, err := qoa.Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := qoa.NewStreamDecoder(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	if stream.Metadata() != metadata {
		t.Fatalf("metadata = %+v, want %+v", stream.Metadata(), metadata)
	}
	var got []byte
	buffer := make([]byte, 37)
	for {
		n, readErr := stream.Read(buffer)
		got = append(got, buffer[:n]...)
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			t.Fatal(readErr)
		}
	}
	want := make([]byte, len(complete)*2)
	for index, sample := range complete {
		want[index*2], want[index*2+1] = byte(sample), byte(uint16(sample)>>8)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("streamed PCM differs from complete decode")
	}
}

func TestStreamDecoderRejectsTruncationAndTrailingData(t *testing.T) {
	t.Parallel()
	encoded, _, err := qoa.Encode(make([]int16, 6001), 1, 48_000)
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{encoded[:len(encoded)-1], append(append([]byte(nil), encoded...), 1)} {
		stream, createErr := qoa.NewStreamDecoder(bytes.NewReader(data))
		if createErr == nil {
			_, createErr = io.ReadAll(stream)
		}
		if createErr == nil {
			t.Fatal("invalid stream was accepted")
		}
	}
}

func TestStreamDecoderRejectsInvalidMetadataAndFrames(t *testing.T) {
	t.Parallel()
	encoded, _, err := qoa.Encode(make([]int16, 6001), 1, 48_000)
	if err != nil {
		t.Fatal(err)
	}
	firstFrameSize := int(binary.BigEndian.Uint16(encoded[14:16]))
	secondFrame := 8 + firstFrameSize

	for name, mutate := range map[string]func([]byte) []byte{
		"streaming header": func(data []byte) []byte {
			binary.BigEndian.PutUint32(data[4:8], 0)
			return data
		},
		"duration": func(data []byte) []byte {
			binary.BigEndian.PutUint32(data[4:8], 48_000*qoa.MaxStreamDurationSeconds+1)
			return data
		},
		"unsupported rate": func(data []byte) []byte {
			data[9], data[10], data[11] = 1, 0, 0
			return data
		},
		"short non-final frame": func(data []byte) []byte {
			binary.BigEndian.PutUint16(data[12:14], 5100)
			binary.BigEndian.PutUint16(data[14:16], uint16(firstFrameSize-8))
			return data
		},
		"changed channels": func(data []byte) []byte {
			data[secondFrame] = 2
			return data
		},
		"nonzero padding": func(data []byte) []byte {
			data[len(data)-1] = 1
			return data
		},
	} {
		t.Run(name, func(t *testing.T) {
			data := mutate(bytes.Clone(encoded))
			stream, streamErr := qoa.NewStreamDecoder(bytes.NewReader(data))
			if streamErr == nil {
				_, streamErr = io.ReadAll(stream)
			}
			if !errors.Is(streamErr, qoa.ErrInvalid) {
				t.Fatalf("error = %v, want %v", streamErr, qoa.ErrInvalid)
			}
		})
	}
}

func TestStreamDecoderPreservesSourceErrors(t *testing.T) {
	t.Parallel()
	encoded, _, err := qoa.Encode(make([]int16, 6000), 1, 48_000)
	if err != nil {
		t.Fatal(err)
	}
	want := errors.New("source failed")
	for name, source := range map[string]io.Reader{
		"header":   io.MultiReader(bytes.NewReader(encoded[:4]), failingReader{err: want}),
		"frame":    io.MultiReader(bytes.NewReader(encoded[:len(encoded)-1]), failingReader{err: want}),
		"trailing": io.MultiReader(bytes.NewReader(encoded), failingReader{err: want}),
	} {
		t.Run(name, func(t *testing.T) {
			stream, got := qoa.NewStreamDecoder(source)
			if got == nil {
				_, got = io.ReadAll(stream)
			}
			if !errors.Is(got, want) {
				t.Fatalf("error = %v, want source error", got)
			}
		})
	}
}

func TestStreamDecoderNilAndZeroLengthRead(t *testing.T) {
	t.Parallel()
	if _, err := qoa.NewStreamDecoder(nil); !errors.Is(err, qoa.ErrInvalid) {
		t.Fatalf("nil source error = %v", err)
	}
	encoded, _, err := qoa.Encode(make([]int16, 20), 1, 48_000)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := qoa.NewStreamDecoder(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	if n, err := stream.Read(nil); n != 0 || err != nil {
		t.Fatalf("Read(nil) = %d, %v", n, err)
	}
}

func FuzzStreamDecoder(f *testing.F) {
	encoded, _, err := qoa.Encode(make([]int16, 6000), 1, 48_000)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(encoded)
	f.Add(encoded[:len(encoded)-1])
	f.Add([]byte("qoaf"))
	f.Fuzz(func(t *testing.T, data []byte) {
		stream, err := qoa.NewStreamDecoder(bytes.NewReader(data))
		if err == nil {
			_, _ = io.ReadAll(stream)
		}
	})
}

type failingReader struct{ err error }

func (reader failingReader) Read([]byte) (int, error) { return 0, reader.err }
