package qoa

import (
	"encoding/binary"
	"errors"
	"io"

	braheezy "github.com/braheezy/qoa"
)

// StreamDecoder incrementally validates QOA frames and exposes interleaved
// little-endian PCM16. It retains at most one encoded and one decoded frame.
type StreamDecoder struct {
	source    io.Reader
	metadata  Metadata
	remaining uint64
	pending   []byte
	complete  bool
}

// NewStreamDecoder reads the file header and first frame. Static QOA with a
// known sample count is required so allocation and duration bounds are known.
func NewStreamDecoder(source io.Reader) (*StreamDecoder, error) {
	if source == nil {
		return nil, ErrInvalid
	}
	var header [qoaHeaderSize]byte
	if err := readStreamBytes(source, header[:]); err != nil {
		return nil, err
	}
	if string(header[:4]) != "qoaf" {
		return nil, ErrInvalid
	}
	frames := binary.BigEndian.Uint32(header[4:])
	if frames == 0 {
		return nil, ErrInvalid
	}
	decoder := &StreamDecoder{source: source, remaining: uint64(frames)}
	if err := decoder.nextFrame(); err != nil {
		return nil, err
	}
	decoder.metadata.Frames = frames
	decoder.metadata.DecodedBytes = uint64(frames) * uint64(decoder.metadata.Channels) * 2
	if uint64(frames) > uint64(decoder.metadata.SampleRate)*MaxStreamDurationSeconds {
		return nil, ErrInvalid
	}
	return decoder, nil
}

func (decoder *StreamDecoder) Metadata() Metadata { return decoder.metadata }

func (decoder *StreamDecoder) Read(destination []byte) (int, error) {
	if len(destination) == 0 {
		return 0, nil
	}
	for len(decoder.pending) == 0 {
		if decoder.remaining == 0 {
			if !decoder.complete {
				var trailing [1]byte
				n, err := decoder.source.Read(trailing[:])
				if n != 0 || err == nil {
					return 0, ErrInvalid
				}
				if err != io.EOF {
					return 0, err
				}
				decoder.complete = true
			}
			return 0, io.EOF
		}
		if err := decoder.nextFrame(); err != nil {
			return 0, err
		}
	}
	n := copy(destination, decoder.pending)
	decoder.pending = decoder.pending[n:]
	return n, nil
}

func (decoder *StreamDecoder) nextFrame() error {
	var header [frameHeaderSize]byte
	if err := readStreamBytes(decoder.source, header[:]); err != nil {
		return err
	}
	value := binary.BigEndian.Uint64(header[:])
	channels := uint8(value >> 56)
	sampleRate := uint32(value>>32) & 0x00ff_ffff
	frameSamples := uint32(value>>16) & 0xffff
	frameSize := uint64(value & 0xffff)
	if (channels != 1 && channels != 2) || !supportedRate(sampleRate) || frameSamples == 0 || frameSamples > maxFrameSamples {
		return ErrInvalid
	}
	slices := (uint64(frameSamples) + 19) / 20
	expected := uint64(frameHeaderSize) + uint64(lmsBytesPerChannel)*uint64(channels) + 8*slices*uint64(channels)
	if frameSize != expected || uint64(frameSamples) > decoder.remaining {
		return ErrInvalid
	}
	if decoder.metadata.Channels != 0 && (decoder.metadata.Channels != channels || decoder.metadata.SampleRate != sampleRate) {
		return ErrInvalid
	}
	if decoder.remaining > uint64(frameSamples) && frameSamples != maxFrameSamples {
		return ErrInvalid
	}
	frame := make([]byte, int(frameSize))
	copy(frame, header[:])
	if err := readStreamBytes(decoder.source, frame[frameHeaderSize:]); err != nil {
		return err
	}
	if !unusedSamplesAreZero(frame, 0, channels, frameSamples, slices) {
		return ErrInvalid
	}
	file := make([]byte, qoaHeaderSize+len(frame))
	copy(file, "qoaf")
	binary.BigEndian.PutUint32(file[4:8], frameSamples)
	copy(file[qoaHeaderSize:], frame)
	description, samples, err := decodeStreamFrame(file)
	if err != nil || description == nil || description.Channels != uint32(channels) || description.SampleRate != sampleRate ||
		description.Samples != frameSamples || len(samples) != int(frameSamples)*int(channels) {
		return ErrInvalid
	}
	if decoder.metadata.Channels == 0 {
		decoder.metadata.Channels, decoder.metadata.SampleRate = channels, sampleRate
	}
	pcm := make([]byte, len(samples)*2)
	for index, sample := range samples {
		binary.LittleEndian.PutUint16(pcm[index*2:], uint16(sample))
	}
	decoder.pending = pcm
	decoder.remaining -= uint64(frameSamples)
	return nil
}

func readStreamBytes(source io.Reader, destination []byte) error {
	_, err := io.ReadFull(source, destination)
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return ErrInvalid
	}
	return err
}

func decodeStreamFrame(encoded []byte) (description *braheezy.QOA, samples []int16, err error) {
	defer func() {
		if recover() != nil {
			description, samples, err = nil, nil, ErrInvalid
		}
	}()
	return braheezy.Decode(encoded)
}
