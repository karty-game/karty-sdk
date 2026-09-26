package cartridge

import (
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	qoacodec "github.com/karty-game/karty-sdk/codec/qoa"
	"github.com/karty-game/karty-sdk/format/asset"
)

const (
	SoundSectionName = "karty.sounds.v1"
	soundMagic       = "KTYS"
	soundVersion     = uint16(1)
	soundHeaderSize  = 16
	soundEntryHeader = 24
	MaxSoundBundle   = 16 * 1024 * 1024
	MaxSoundCount    = 1024
	MaxSoundName     = 1024
	MaxSoundSize     = 8 * 1024 * 1024
)

var ErrSounds = errors.New("game sound bundle is invalid")

type SoundCodec uint16

const SoundCodecQOA SoundCodec = 1

// Sound is a validated game-scoped, one-shot sound. Frames counts samples per
// channel. Bytes contains one complete, static (non-streaming) QOA file.
type Sound struct {
	ID         uint32
	Name       string
	Codec      SoundCodec
	Channels   uint8
	SampleRate uint32
	Frames     uint32
	Bytes      []byte
}

// EncodeSounds creates a deterministic catalog ordered by logical name. IDs
// are one-based positions in that order and are deliberately local to one
// build; callers must not persist them as save-game identifiers.
func EncodeSounds(source []Sound) ([]byte, error) {
	if len(source) == 0 || len(source) > MaxSoundCount {
		return nil, ErrSounds
	}

	sounds := slices.Clone(source)
	slices.SortFunc(sounds, func(left, right Sound) int { return strings.Compare(left.Name, right.Name) })
	total := soundHeaderSize
	decodedTotal := uint64(0)
	previous := ""
	for index, sound := range sounds {
		if err := validateSound(sound, uint32(index+1), previous); err != nil {
			return nil, fmt.Errorf("sound %q: %w", sound.Name, err)
		}
		previous = sound.Name
		total += soundEntryHeader + len(sound.Name) + len(sound.Bytes)
		decodedTotal += uint64(sound.Frames) * uint64(sound.Channels) * 2
		if total > MaxSoundBundle || decodedTotal > asset.MaxDecodedSounds {
			return nil, ErrSounds
		}
	}

	result := make([]byte, total)
	copy(result, soundMagic)
	binary.LittleEndian.PutUint16(result[4:6], soundVersion)
	binary.LittleEndian.PutUint32(result[8:12], uint32(len(sounds)))
	binary.LittleEndian.PutUint32(result[12:16], uint32(total))
	offset := soundHeaderSize
	for _, sound := range sounds {
		binary.LittleEndian.PutUint32(result[offset:offset+4], sound.ID)
		binary.LittleEndian.PutUint16(result[offset+4:offset+6], uint16(sound.Codec))
		result[offset+6] = sound.Channels
		binary.LittleEndian.PutUint32(result[offset+8:offset+12], sound.SampleRate)
		binary.LittleEndian.PutUint32(result[offset+12:offset+16], sound.Frames)
		binary.LittleEndian.PutUint16(result[offset+16:offset+18], uint16(len(sound.Name)))
		binary.LittleEndian.PutUint32(result[offset+20:offset+24], uint32(len(sound.Bytes)))
		offset += soundEntryHeader
		copy(result[offset:], sound.Name)
		offset += len(sound.Name)
		copy(result[offset:], sound.Bytes)
		offset += len(sound.Bytes)
	}

	return result, nil
}

func DecodeSounds(encoded []byte) ([]Sound, error) {
	if len(encoded) < soundHeaderSize || len(encoded) > MaxSoundBundle || string(encoded[:4]) != soundMagic ||
		binary.LittleEndian.Uint16(encoded[4:6]) != soundVersion || binary.LittleEndian.Uint16(encoded[6:8]) != 0 ||
		int(binary.LittleEndian.Uint32(encoded[12:16])) != len(encoded) {
		return nil, ErrSounds
	}

	count := int(binary.LittleEndian.Uint32(encoded[8:12]))
	if count < 1 || count > MaxSoundCount {
		return nil, ErrSounds
	}

	result := make([]Sound, 0, count)
	offset := soundHeaderSize
	previous := ""
	decodedTotal := uint64(0)
	for index := range count {
		if len(encoded)-offset < soundEntryHeader {
			return nil, ErrSounds
		}
		sound := Sound{
			ID:         binary.LittleEndian.Uint32(encoded[offset : offset+4]),
			Codec:      SoundCodec(binary.LittleEndian.Uint16(encoded[offset+4 : offset+6])),
			Channels:   encoded[offset+6],
			SampleRate: binary.LittleEndian.Uint32(encoded[offset+8 : offset+12]),
			Frames:     binary.LittleEndian.Uint32(encoded[offset+12 : offset+16]),
		}
		nameLength := int(binary.LittleEndian.Uint16(encoded[offset+16 : offset+18]))
		dataLength := int(binary.LittleEndian.Uint32(encoded[offset+20 : offset+24]))
		if encoded[offset+7] != 0 || binary.LittleEndian.Uint16(encoded[offset+18:offset+20]) != 0 ||
			nameLength < 1 || nameLength > MaxSoundName || dataLength < 1 || dataLength > MaxSoundSize ||
			nameLength > len(encoded)-offset-soundEntryHeader || dataLength > len(encoded)-offset-soundEntryHeader-nameLength {
			return nil, ErrSounds
		}
		offset += soundEntryHeader
		sound.Name = string(encoded[offset : offset+nameLength])
		offset += nameLength
		sound.Bytes = encoded[offset : offset+dataLength]
		offset += dataLength
		if err := validateSound(sound, uint32(index+1), previous); err != nil {
			return nil, ErrSounds
		}
		decodedTotal += uint64(sound.Frames) * uint64(sound.Channels) * 2
		if decodedTotal > asset.MaxDecodedSounds {
			return nil, ErrSounds
		}
		result = append(result, sound)
		previous = sound.Name
	}
	if offset != len(encoded) {
		return nil, ErrSounds
	}

	return result, nil
}

func EmbedSounds(wasm, bundle []byte) ([]byte, error) {
	if _, err := DecodeSounds(bundle); err != nil {
		return nil, err
	}
	result, err := EmbedSection(wasm, SoundSectionName, bundle)
	if err != nil {
		return nil, ErrSounds
	}

	return result, nil
}

func ExtractSounds(wasm []byte) ([]Sound, error) {
	bundle, err := ExtractSection(wasm, SoundSectionName)
	if err != nil {
		return nil, ErrSounds
	}

	return DecodeSounds(bundle)
}

func validateSound(sound Sound, expectedID uint32, previous string) error {
	decodedBytes := uint64(sound.Frames) * uint64(sound.Channels) * 2
	if sound.ID != expectedID || sound.Name == "" || len(sound.Name) > MaxSoundName || !utf8.ValidString(sound.Name) ||
		sound.Name <= previous || sound.Codec != SoundCodecQOA || (sound.Channels != 1 && sound.Channels != 2) ||
		!supportedSampleRate(sound.SampleRate) || sound.Frames == 0 ||
		uint64(sound.Frames) > uint64(sound.SampleRate)*asset.MaxSoundDurationSeconds ||
		decodedBytes > asset.MaxDecodedSoundBytes || len(sound.Bytes) < 1 || len(sound.Bytes) > MaxSoundSize ||
		!validStaticQOA(sound.Bytes, sound.Channels, sound.SampleRate, sound.Frames) {
		return ErrSounds
	}

	return nil
}

func supportedSampleRate(rate uint32) bool {
	return rate == 22_050 || rate == 24_000 || rate == 44_100 || rate == 48_000
}

func validStaticQOA(encoded []byte, channels uint8, rate, frames uint32) bool {
	metadata, err := qoacodec.Inspect(encoded)

	return err == nil && metadata.Channels == channels && metadata.SampleRate == rate && metadata.Frames == frames
}
