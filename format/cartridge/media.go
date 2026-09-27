package cartridge

import (
	"encoding/binary"
	"errors"
	"io"
)

const (
	MediaEnvelopeHeaderSize = 16
	mediaEnvelopeMagic      = "KMED"
	mediaEnvelopeVersion    = uint16(1)
	mediaMaskBufferSize     = 32 * 1024
)

var ErrMediaEnvelope = errors.New("invalid Karty media envelope")

// MediaKind identifies the codec carried by an opaque media sidecar.
type MediaKind uint8

const (
	MediaKindQOAAudio   MediaKind = 1
	MediaKindMPEG1Video MediaKind = 2
)

// MediaEnvelopeHeader describes the unmasked payload following a fixed-size
// Karty media header. PayloadLength excludes MediaEnvelopeHeaderSize.
type MediaEnvelopeHeader struct {
	Kind          MediaKind
	PayloadLength uint64
}

// MediaEnvelopeSize returns the complete stored size for a valid payload.
func MediaEnvelopeSize(kind MediaKind, payloadLength uint64) (int64, error) {
	if !validMediaPayload(kind, payloadLength) {
		return 0, ErrMediaEnvelope
	}
	return int64(MediaEnvelopeHeaderSize) + int64(payloadLength), nil
}

// EncodeMediaEnvelopeHeader encodes a fixed-size version 1 media header.
func EncodeMediaEnvelopeHeader(header MediaEnvelopeHeader) ([MediaEnvelopeHeaderSize]byte, error) {
	var encoded [MediaEnvelopeHeaderSize]byte
	if !validMediaPayload(header.Kind, header.PayloadLength) {
		return encoded, ErrMediaEnvelope
	}
	copy(encoded[:4], mediaEnvelopeMagic)
	binary.LittleEndian.PutUint16(encoded[4:6], mediaEnvelopeVersion)
	encoded[6] = byte(header.Kind)
	binary.LittleEndian.PutUint64(encoded[8:16], header.PayloadLength)
	return encoded, nil
}

// DecodeMediaEnvelopeHeader validates one complete fixed-size media header.
func DecodeMediaEnvelopeHeader(encoded []byte) (MediaEnvelopeHeader, error) {
	if len(encoded) != MediaEnvelopeHeaderSize || string(encoded[:4]) != mediaEnvelopeMagic ||
		binary.LittleEndian.Uint16(encoded[4:6]) != mediaEnvelopeVersion || encoded[7] != 0 {
		return MediaEnvelopeHeader{}, ErrMediaEnvelope
	}
	header := MediaEnvelopeHeader{
		Kind:          MediaKind(encoded[6]),
		PayloadLength: binary.LittleEndian.Uint64(encoded[8:16]),
	}
	if !validMediaPayload(header.Kind, header.PayloadLength) {
		return MediaEnvelopeHeader{}, ErrMediaEnvelope
	}
	return header, nil
}

// MediaWriter writes a header followed by position-masked payload bytes. It
// uses bounded scratch storage and never retains the complete payload.
type MediaWriter struct {
	destination io.Writer
	header      MediaEnvelopeHeader
	position    uint64
	err         error
	finished    bool
	buffer      [mediaMaskBufferSize]byte
}

// NewMediaWriter validates the kind and payload length and immediately writes
// the fixed header to destination.
func NewMediaWriter(destination io.Writer, kind MediaKind, payloadLength uint64) (*MediaWriter, error) {
	if destination == nil {
		return nil, ErrMediaEnvelope
	}
	header := MediaEnvelopeHeader{Kind: kind, PayloadLength: payloadLength}
	encoded, err := EncodeMediaEnvelopeHeader(header)
	if err != nil {
		return nil, err
	}
	if err = writeMediaBytes(destination, encoded[:]); err != nil {
		return nil, err
	}
	return &MediaWriter{destination: destination, header: header}, nil
}

func (writer *MediaWriter) Write(payload []byte) (int, error) {
	if writer == nil || writer.destination == nil || writer.finished {
		return 0, ErrMediaEnvelope
	}
	if writer.err != nil {
		return 0, writer.err
	}
	if uint64(len(payload)) > writer.header.PayloadLength-writer.position {
		writer.err = ErrMediaEnvelope
		return 0, writer.err
	}
	written := 0
	for written < len(payload) {
		size := min(len(writer.buffer), len(payload)-written)
		copy(writer.buffer[:size], payload[written:written+size])
		if err := TransformMediaPayload(writer.header.Kind, writer.position, writer.buffer[:size]); err != nil {
			writer.err = err
			return written, err
		}
		n, err := writer.destination.Write(writer.buffer[:size])
		if n < 0 || n > size {
			n, err = 0, ErrMediaEnvelope
		}
		writer.position += uint64(n)
		written += n
		if err == nil && n == 0 {
			err = io.ErrShortWrite
		}
		if err != nil {
			writer.err = err
			return written, err
		}
	}
	return written, nil
}

// Finish verifies that exactly the declared payload length was written. It
// does not close the destination.
func (writer *MediaWriter) Finish() error {
	if writer == nil || writer.destination == nil {
		return ErrMediaEnvelope
	}
	if writer.err != nil {
		return writer.err
	}
	if writer.finished {
		return nil
	}
	if writer.position != writer.header.PayloadLength {
		writer.err = ErrMediaEnvelope
		return writer.err
	}
	writer.finished = true
	return nil
}

// MediaReader validates a header and exposes its unmasked payload as a stream.
type MediaReader struct {
	source   io.Reader
	header   MediaEnvelopeHeader
	position uint64
	err      error
	verified bool
}

// NewMediaReader reads and validates the header. storedSize must be the exact
// catalog size, including the header, and must match the declared payload.
func NewMediaReader(source io.Reader, expectedKind MediaKind, storedSize int64) (*MediaReader, error) {
	if source == nil || storedSize <= MediaEnvelopeHeaderSize {
		return nil, ErrMediaEnvelope
	}
	var encoded [MediaEnvelopeHeaderSize]byte
	if err := readMediaHeader(source, encoded[:]); err != nil {
		return nil, err
	}
	header, err := DecodeMediaEnvelopeHeader(encoded[:])
	if err != nil || header.Kind != expectedKind {
		return nil, ErrMediaEnvelope
	}
	want, err := MediaEnvelopeSize(header.Kind, header.PayloadLength)
	if err != nil || want != storedSize {
		return nil, ErrMediaEnvelope
	}
	return &MediaReader{source: source, header: header}, nil
}

func (reader *MediaReader) Header() MediaEnvelopeHeader { return reader.header }

func (reader *MediaReader) Read(destination []byte) (int, error) {
	if reader == nil || reader.source == nil {
		return 0, ErrMediaEnvelope
	}
	if len(destination) == 0 {
		return 0, nil
	}
	if reader.err != nil {
		return 0, reader.err
	}
	remaining := reader.header.PayloadLength - reader.position
	if remaining == 0 {
		if err := reader.Verify(); err != nil {
			return 0, err
		}
		return 0, io.EOF
	}
	if uint64(len(destination)) > remaining {
		destination = destination[:remaining]
	}
	n, err := reader.source.Read(destination)
	if n < 0 || n > len(destination) {
		reader.err = ErrMediaEnvelope
		return 0, reader.err
	}
	if n > 0 {
		if maskErr := TransformMediaPayload(reader.header.Kind, reader.position, destination[:n]); maskErr != nil {
			reader.err = maskErr
			return 0, maskErr
		}
		reader.position += uint64(n)
	}
	if errors.Is(err, io.EOF) {
		if reader.position != reader.header.PayloadLength {
			err = ErrMediaEnvelope
		} else {
			reader.verified = true
		}
	} else if err == nil && n == 0 {
		err = io.ErrNoProgress
	}
	if err != nil && !errors.Is(err, io.EOF) {
		reader.err = err
	}
	return n, err
}

// Verify requires the payload to be fully consumed and rejects trailing data.
func (reader *MediaReader) Verify() error {
	if reader == nil || reader.source == nil {
		return ErrMediaEnvelope
	}
	if reader.err != nil {
		return reader.err
	}
	if reader.position != reader.header.PayloadLength {
		return ErrMediaEnvelope
	}
	if reader.verified {
		return nil
	}
	var trailing [1]byte
	n, err := reader.source.Read(trailing[:])
	if n != 0 || err == nil {
		reader.err = ErrMediaEnvelope
		return reader.err
	}
	if !errors.Is(err, io.EOF) {
		reader.err = err
		return err
	}
	reader.verified = true
	return nil
}

func validMediaPayload(kind MediaKind, payloadLength uint64) bool {
	if payloadLength == 0 {
		return false
	}
	maximum, ok := maxMediaPayload(kind)
	return ok && payloadLength <= maximum
}

// TransformMediaPayload masks or unmasks data in place at its payload-relative
// byte offset. Applying it twice with the same kind and offset restores the
// input. This is format obfuscation, not encryption or authentication.
func TransformMediaPayload(kind MediaKind, payloadOffset uint64, data []byte) error {
	maximum, ok := maxMediaPayload(kind)
	if !ok || payloadOffset > maximum || uint64(len(data)) > maximum-payloadOffset {
		return ErrMediaEnvelope
	}
	maskMedia(data, kind, payloadOffset)
	return nil
}

func maxMediaPayload(kind MediaKind) (uint64, bool) {
	switch kind {
	case MediaKindQOAAudio:
		return uint64(MaxAudioStreamPayloadSize), true
	case MediaKindMPEG1Video:
		return uint64(MaxVideoPayloadSize), true
	default:
		return 0, false
	}
}

func readMediaHeader(source io.Reader, destination []byte) error {
	_, err := io.ReadFull(source, destination)
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return ErrMediaEnvelope
	}
	return err
}

func maskMedia(data []byte, kind MediaKind, position uint64) {
	for offset := 0; offset < len(data); {
		absolute := position + uint64(offset)
		key := mediaMaskBlock(kind, absolute/8)
		start := uint(absolute%8) * 8
		for start < 64 && offset < len(data) {
			data[offset] ^= byte(key >> start)
			offset++
			start += 8
		}
	}
}

func mediaMaskBlock(kind MediaKind, block uint64) uint64 {
	value := block + 0x9e3779b97f4a7c15 + uint64(kind)*0xd1b54a32d192ed03
	value = (value ^ (value >> 30)) * 0xbf58476d1ce4e5b9
	value = (value ^ (value >> 27)) * 0x94d049bb133111eb
	return value ^ (value >> 31)
}

func writeMediaBytes(destination io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := destination.Write(data)
		if n < 0 || n > len(data) {
			return ErrMediaEnvelope
		}
		data = data[n:]
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}
