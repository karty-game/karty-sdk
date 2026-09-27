package cartridge

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

func TestMediaEnvelopeStreamsRoundTripAcrossChunkBoundaries(t *testing.T) {
	t.Parallel()
	payload := make([]byte, 100_003)
	copy(payload, "qoaf generic media probe signature")
	for index := 32; index < len(payload); index++ {
		payload[index] = byte(index * 31)
	}

	var stored bytes.Buffer
	limited := &limitedWriter{destination: &stored, maximum: 137}
	writer, err := NewMediaWriter(limited, MediaKindQOAAudio, uint64(len(payload)))
	if err != nil {
		t.Fatal(err)
	}
	for offset := 0; offset < len(payload); {
		size := min(509, len(payload)-offset)
		n, err := writer.Write(payload[offset : offset+size])
		if err != nil || n != size {
			t.Fatalf("Write() = %d, %v", n, err)
		}
		offset += n
	}
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(stored.Bytes()[MediaEnvelopeHeaderSize:MediaEnvelopeHeaderSize+4], payload[:4]) ||
		bytes.Contains(stored.Bytes()[MediaEnvelopeHeaderSize:], []byte("qoaf")) {
		t.Fatal("stored payload exposes the QOA signature")
	}

	reader, err := NewMediaReader(bytes.NewReader(stored.Bytes()), MediaKindQOAAudio, int64(stored.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if reader.Header() != (MediaEnvelopeHeader{Kind: MediaKindQOAAudio, PayloadLength: uint64(len(payload))}) {
		t.Fatalf("header = %+v", reader.Header())
	}
	var decoded bytes.Buffer
	buffer := make([]byte, 211)
	for {
		n, readErr := reader.Read(buffer)
		decoded.Write(buffer[:n])
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			t.Fatal(readErr)
		}
	}
	if err := reader.Verify(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded.Bytes(), payload) {
		t.Fatal("decoded payload differs")
	}
}

func TestMediaEnvelopeHeaderLayoutAndBounds(t *testing.T) {
	t.Parallel()
	header := MediaEnvelopeHeader{Kind: MediaKindMPEG1Video, PayloadLength: 0x0102_0304}
	encoded, err := EncodeMediaEnvelopeHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	want := [MediaEnvelopeHeaderSize]byte{'K', 'M', 'E', 'D', 1, 0, 2, 0, 4, 3, 2, 1, 0, 0, 0, 0}
	if encoded != want {
		t.Fatalf("header = %x, want %x", encoded, want)
	}
	if decoded, err := DecodeMediaEnvelopeHeader(encoded[:]); err != nil || decoded != header {
		t.Fatalf("DecodeMediaEnvelopeHeader() = %+v, %v", decoded, err)
	}
	if size, err := MediaEnvelopeSize(MediaKindMPEG1Video, header.PayloadLength); err != nil ||
		size != int64(MediaEnvelopeHeaderSize)+int64(header.PayloadLength) {
		t.Fatalf("MediaEnvelopeSize() = %d, %v", size, err)
	}

	for _, test := range []MediaEnvelopeHeader{
		{},
		{Kind: 99, PayloadLength: 1},
		{Kind: MediaKindQOAAudio, PayloadLength: uint64(MaxAudioStreamPayloadSize) + 1},
		{Kind: MediaKindMPEG1Video, PayloadLength: uint64(MaxVideoPayloadSize) + 1},
	} {
		if _, err := EncodeMediaEnvelopeHeader(test); !errors.Is(err, ErrMediaEnvelope) {
			t.Fatalf("EncodeMediaEnvelopeHeader(%+v) error = %v", test, err)
		}
	}
}

func TestTransformMediaPayloadIsPositionAwareAndReversible(t *testing.T) {
	t.Parallel()
	original := make([]byte, 257)
	for index := range original {
		original[index] = byte(index)
	}
	whole := bytes.Clone(original)
	if err := TransformMediaPayload(MediaKindMPEG1Video, 0, whole); err != nil {
		t.Fatal(err)
	}
	chunked := bytes.Clone(original)
	if err := TransformMediaPayload(MediaKindMPEG1Video, 0, chunked[:73]); err != nil {
		t.Fatal(err)
	}
	if err := TransformMediaPayload(MediaKindMPEG1Video, 73, chunked[73:]); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(chunked, whole) || bytes.Equal(whole, original) {
		t.Fatal("position-aware chunk transform differs from whole transform")
	}
	if err := TransformMediaPayload(MediaKindMPEG1Video, 0, whole); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(whole, original) {
		t.Fatal("second transform did not restore payload")
	}
	if err := TransformMediaPayload(99, 0, whole); !errors.Is(err, ErrMediaEnvelope) {
		t.Fatalf("unknown kind error = %v", err)
	}
	if err := TransformMediaPayload(MediaKindMPEG1Video, uint64(MaxVideoPayloadSize), []byte{1}); !errors.Is(err, ErrMediaEnvelope) {
		t.Fatalf("out-of-range transform error = %v", err)
	}
}

func TestMediaReaderRejectsMalformedEnvelope(t *testing.T) {
	t.Parallel()
	payload := []byte("qoaf payload")
	valid := mediaFixture(t, MediaKindQOAAudio, payload)

	for name, mutate := range map[string]func([]byte) ([]byte, MediaKind, int64){
		"magic": func(data []byte) ([]byte, MediaKind, int64) {
			data[0] ^= 1
			return data, MediaKindQOAAudio, int64(len(data))
		},
		"version": func(data []byte) ([]byte, MediaKind, int64) {
			data[4] = 2
			return data, MediaKindQOAAudio, int64(len(data))
		},
		"reserved": func(data []byte) ([]byte, MediaKind, int64) {
			data[7] = 1
			return data, MediaKindQOAAudio, int64(len(data))
		},
		"kind": func(data []byte) ([]byte, MediaKind, int64) {
			return data, MediaKindMPEG1Video, int64(len(data))
		},
		"declared length": func(data []byte) ([]byte, MediaKind, int64) {
			binary.LittleEndian.PutUint64(data[8:16], uint64(len(payload)+1))
			return data, MediaKindQOAAudio, int64(len(data))
		},
		"catalog size": func(data []byte) ([]byte, MediaKind, int64) {
			return data, MediaKindQOAAudio, int64(len(data) + 1)
		},
		"short header": func(data []byte) ([]byte, MediaKind, int64) {
			return data[:8], MediaKindQOAAudio, int64(len(data))
		},
	} {
		t.Run(name, func(t *testing.T) {
			data, kind, size := mutate(bytes.Clone(valid))
			if _, err := NewMediaReader(bytes.NewReader(data), kind, size); err == nil {
				t.Fatal("accepted malformed envelope")
			}
		})
	}
}

func TestMediaReaderRejectsTruncatedAndTrailingPayload(t *testing.T) {
	t.Parallel()
	payload := []byte("\x00\x00\x01\xba MPEG payload")
	valid := mediaFixture(t, MediaKindMPEG1Video, payload)

	truncated := valid[:len(valid)-1]
	reader, err := NewMediaReader(bytes.NewReader(truncated), MediaKindMPEG1Video, int64(len(valid)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(reader); !errors.Is(err, ErrMediaEnvelope) {
		t.Fatalf("truncated payload error = %v", err)
	}

	trailing := append(bytes.Clone(valid), 1)
	reader, err = NewMediaReader(bytes.NewReader(trailing), MediaKindMPEG1Video, int64(len(valid)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(reader); !errors.Is(err, ErrMediaEnvelope) {
		t.Fatalf("trailing payload error = %v", err)
	}
}

func TestMediaWriterRequiresExactPayloadLength(t *testing.T) {
	t.Parallel()
	var stored bytes.Buffer
	writer, err := NewMediaWriter(&stored, MediaKindQOAAudio, 4)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte{1, 2}); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(writer.Finish(), ErrMediaEnvelope) {
		t.Fatal("Finish accepted a short payload")
	}

	stored.Reset()
	writer, err = NewMediaWriter(&stored, MediaKindQOAAudio, 2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte{1, 2, 3}); !errors.Is(err, ErrMediaEnvelope) {
		t.Fatal("Write accepted payload beyond declared length")
	}
}

func TestMediaReaderPreservesSourceErrors(t *testing.T) {
	t.Parallel()
	want := errors.New("source failed")
	if _, err := NewMediaReader(failingMediaReader{err: want}, MediaKindQOAAudio, 100); !errors.Is(err, want) {
		t.Fatalf("header error = %v", err)
	}

	valid := mediaFixture(t, MediaKindQOAAudio, []byte("payload"))
	source := io.MultiReader(bytes.NewReader(valid[:len(valid)-1]), failingMediaReader{err: want})
	reader, err := NewMediaReader(source, MediaKindQOAAudio, int64(len(valid)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(reader); !errors.Is(err, want) {
		t.Fatalf("payload error = %v", err)
	}
}

func FuzzMediaEnvelopeReader(f *testing.F) {
	valid := mediaFixture(f, MediaKindQOAAudio, []byte("qoaf payload"))
	f.Add(valid)
	f.Add([]byte("KMED"))
	f.Fuzz(func(t *testing.T, data []byte) {
		reader, err := NewMediaReader(bytes.NewReader(data), MediaKindQOAAudio, int64(len(data)))
		if err == nil {
			_, _ = io.ReadAll(reader)
			_ = reader.Verify()
		}
	})
}

type testingFataler interface {
	Helper()
	Fatal(...any)
}

func mediaFixture(t testingFataler, kind MediaKind, payload []byte) []byte {
	t.Helper()
	var stored bytes.Buffer
	writer, err := NewMediaWriter(&stored, kind, uint64(len(payload)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}
	return stored.Bytes()
}

type limitedWriter struct {
	destination io.Writer
	maximum     int
}

func (writer *limitedWriter) Write(data []byte) (int, error) {
	return writer.destination.Write(data[:min(len(data), writer.maximum)])
}

type failingMediaReader struct{ err error }

func (reader failingMediaReader) Read([]byte) (int, error) { return 0, reader.err }
