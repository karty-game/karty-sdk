// Package qoa provides bounded one-shot and incremental adapters around
// Karty's pinned QOA codec.
package qoa

import (
	"encoding/binary"
	"errors"

	braheezy "github.com/braheezy/qoa"
	"github.com/karty-game/karty-sdk/format/asset"
)

const (
	MaxEncodedBytes    = 8 * 1024 * 1024
	maxFrameSamples    = 256 * 20
	qoaHeaderSize      = 8
	frameHeaderSize    = 8
	lmsBytesPerChannel = 16
)

// MaxStreamDurationSeconds bounds long-form QOA metadata without requiring
// the complete decoded PCM to fit in memory.
const MaxStreamDurationSeconds = asset.MaxAudioStreamDurationSeconds

// MaxStreamEncodedBytes bounds complete long-form QOA files processed by the
// packaging APIs. Runtime playback should use StreamDecoder instead.
const MaxStreamEncodedBytes = asset.MaxEncodedAudioStreamBytes

var ErrInvalid = errors.New("QOA data is invalid")

// Metadata describes bounded, static QOA content. Frames is the number of
// samples per channel; DecodedBytes is interleaved signed PCM16 storage.
type Metadata struct {
	Channels     uint8
	SampleRate   uint32
	Frames       uint32
	DecodedBytes uint64
}

// Inspect validates a complete static QOA file using uint64 arithmetic. It
// rejects streaming files, noncanonical frames, trailing data, and content
// outside Karty's one-shot sound budgets.
func Inspect(encoded []byte) (Metadata, error) {
	return inspect(encoded, MaxEncodedBytes, asset.MaxSoundDurationSeconds, asset.MaxDecodedSoundBytes)
}

// InspectStream validates a complete, static long-form QOA file. It applies
// streaming-audio duration and encoded-size bounds without requiring decoded
// PCM to fit in the one-shot sound budget.
func InspectStream(encoded []byte) (Metadata, error) {
	return inspect(encoded, MaxStreamEncodedBytes, MaxStreamDurationSeconds, 0)
}

func inspect(encoded []byte, maxEncodedBytes int, maxDurationSeconds int, maxDecodedBytes uint64) (Metadata, error) {
	if len(encoded) < qoaHeaderSize+frameHeaderSize || len(encoded) > maxEncodedBytes ||
		string(encoded[:4]) != "qoaf" {
		return Metadata{}, ErrInvalid
	}

	frames := binary.BigEndian.Uint32(encoded[4:8])
	if frames == 0 {
		return Metadata{}, ErrInvalid
	}

	var metadata Metadata
	var decodedFrames uint64
	offset := uint64(qoaHeaderSize)
	encodedLength := uint64(len(encoded))
	for offset < encodedLength {
		if encodedLength-offset < frameHeaderSize {
			return Metadata{}, ErrInvalid
		}
		header := binary.BigEndian.Uint64(encoded[offset : offset+frameHeaderSize])
		channels := uint8(header >> 56)
		sampleRate := uint32(header>>32) & 0x00ff_ffff
		frameSamples := uint32(header>>16) & 0xffff
		frameSize := header & 0xffff
		if (channels != 1 && channels != 2) || !supportedRate(sampleRate) ||
			frameSamples == 0 || frameSamples > maxFrameSamples {
			return Metadata{}, ErrInvalid
		}

		if metadata.Channels == 0 {
			metadata = Metadata{Channels: channels, SampleRate: sampleRate, Frames: frames}
		} else if metadata.Channels != channels || metadata.SampleRate != sampleRate {
			return Metadata{}, ErrInvalid
		}

		slices := (uint64(frameSamples) + 19) / 20
		expectedSize := uint64(frameHeaderSize) + uint64(lmsBytesPerChannel)*uint64(channels) +
			8*slices*uint64(channels)
		if frameSize != expectedSize || frameSize > encodedLength-offset || decodedFrames+uint64(frameSamples) > uint64(frames) {
			return Metadata{}, ErrInvalid
		}
		if !unusedSamplesAreZero(encoded, offset, channels, frameSamples, slices) {
			return Metadata{}, ErrInvalid
		}

		offset += frameSize
		decodedFrames += uint64(frameSamples)
		if offset < encodedLength && frameSamples != maxFrameSamples {
			return Metadata{}, ErrInvalid
		}
	}
	if offset != encodedLength || decodedFrames != uint64(frames) {
		return Metadata{}, ErrInvalid
	}

	metadata.DecodedBytes = uint64(metadata.Frames) * uint64(metadata.Channels) * 2
	if (maxDecodedBytes != 0 && metadata.DecodedBytes > maxDecodedBytes) ||
		uint64(metadata.Frames) > uint64(metadata.SampleRate)*uint64(maxDurationSeconds) {
		return Metadata{}, ErrInvalid
	}

	return metadata, nil
}

// Decode returns interleaved signed PCM16 only after Inspect has established
// safe allocation and frame bounds.
func Decode(encoded []byte) (metadata Metadata, samples []int16, err error) {
	defer func() {
		if recover() != nil {
			metadata, samples, err = Metadata{}, nil, ErrInvalid
		}
	}()

	metadata, err = Inspect(encoded)
	if err != nil {
		return Metadata{}, nil, err
	}

	description, decoded, decodeErr := braheezy.Decode(encoded)
	if decodeErr != nil || description == nil || uint8(description.Channels) != metadata.Channels ||
		description.SampleRate != metadata.SampleRate || description.Samples != metadata.Frames ||
		uint64(len(decoded)) != uint64(metadata.Frames)*uint64(metadata.Channels) {
		return Metadata{}, nil, ErrInvalid
	}

	return metadata, decoded, nil
}

// Encode converts complete interleaved signed PCM16 into deterministic static
// QOA after validating lengths and Karty's runtime budgets.
func Encode(samples []int16, channels uint8, sampleRate uint32) (encoded []byte, metadata Metadata, err error) {
	return encode(samples, channels, sampleRate, asset.MaxSoundDurationSeconds, asset.MaxDecodedSoundBytes, MaxEncodedBytes, Inspect)
}

// EncodeStream converts complete interleaved signed PCM16 into deterministic
// static QOA using the long-form streaming-audio bounds. The resulting file is
// suitable for separate staging and incremental playback with StreamDecoder.
func EncodeStream(samples []int16, channels uint8, sampleRate uint32) (encoded []byte, metadata Metadata, err error) {
	return encode(samples, channels, sampleRate, MaxStreamDurationSeconds, 0, MaxStreamEncodedBytes, InspectStream)
}

func encode(
	samples []int16,
	channels uint8,
	sampleRate uint32,
	maxDurationSeconds int,
	maxDecodedBytes uint64,
	maxEncodedBytes int,
	inspectEncoded func([]byte) (Metadata, error),
) (encoded []byte, metadata Metadata, err error) {
	defer func() {
		if recover() != nil {
			encoded, metadata, err = nil, Metadata{}, ErrInvalid
		}
	}()

	if (channels != 1 && channels != 2) || !supportedRate(sampleRate) || len(samples) == 0 || len(samples)%int(channels) != 0 {
		return nil, Metadata{}, ErrInvalid
	}
	frames := uint64(len(samples) / int(channels))
	decodedBytes := uint64(len(samples)) * 2
	if frames > uint64(^uint32(0)) || frames > uint64(sampleRate)*uint64(maxDurationSeconds) ||
		(maxDecodedBytes != 0 && decodedBytes > maxDecodedBytes) ||
		qoaEncodedSize(frames, channels) > uint64(maxEncodedBytes) {
		return nil, Metadata{}, ErrInvalid
	}

	encoder := braheezy.NewEncoder(sampleRate, uint32(channels), uint32(frames))
	encoded, encodeErr := encoder.Encode(samples)
	if encodeErr != nil {
		return nil, Metadata{}, ErrInvalid
	}
	metadata, err = inspectEncoded(encoded)
	if err != nil || metadata.Channels != channels || metadata.SampleRate != sampleRate || metadata.Frames != uint32(frames) {
		return nil, Metadata{}, ErrInvalid
	}

	return encoded, metadata, nil
}

func qoaEncodedSize(frames uint64, channels uint8) uint64 {
	fullFrames := frames / maxFrameSamples
	remaining := frames % maxFrameSamples
	fullFrameSize := uint64(frameHeaderSize+lmsBytesPerChannel*int(channels)) +
		8*(maxFrameSamples/20)*uint64(channels)
	size := uint64(qoaHeaderSize) + fullFrames*fullFrameSize
	if remaining != 0 {
		slices := (remaining + 19) / 20
		size += uint64(frameHeaderSize+lmsBytesPerChannel*int(channels)) + 8*slices*uint64(channels)
	}
	return size
}

func unusedSamplesAreZero(encoded []byte, frameOffset uint64, channels uint8, samples uint32, slices uint64) bool {
	used := samples % 20
	if used == 0 {
		return true
	}
	unusedBits := uint((20 - used) * 3)
	mask := uint64(1)<<unusedBits - 1
	lastSlices := frameOffset + frameHeaderSize + uint64(lmsBytesPerChannel)*uint64(channels) +
		(slices-1)*8*uint64(channels)
	for channel := uint64(0); channel < uint64(channels); channel++ {
		offset := lastSlices + channel*8
		if binary.BigEndian.Uint64(encoded[offset:offset+8])&mask != 0 {
			return false
		}
	}

	return true
}

func supportedRate(rate uint32) bool {
	return rate == 22_050 || rate == 24_000 || rate == 44_100 || rate == 48_000
}
