package cartridge

import (
	"errors"

	"github.com/karty-game/karty-sdk/format/asset"
)

const (
	AudioStreamSectionName    = "karty.audio-streams.v1"
	AudioStreamChunkSize      = 64 << 10
	MaxAudioStreamPayloadSize = asset.MaxEncodedAudioStreamBytes
	MaxAudioStreamSize        = MaxAudioStreamPayloadSize + MediaEnvelopeHeaderSize
	MaxAudioStreamCount       = 128
	MaxAudioStreamCatalog     = 6 << 20
)

var ErrAudioStreams = errors.New("invalid streaming audio catalog")

type AudioStreamKind string

const (
	AudioStreamMusic       AudioStreamKind = "music"
	AudioStreamEnvironment AudioStreamKind = "environment"
)

// AudioStream describes a separately staged Karty media envelope containing
// static QOA. Size and digests cover the stored envelope bytes. Music entries
// precede environment entries; IDs are one-based and names are sorted within
// each kind.
type AudioStream struct {
	ID         uint32
	Name       string
	Kind       AudioStreamKind
	Channels   uint8
	SampleRate uint32
	Frames     uint32
	Size       int64
	SHA256     string
	Chunks     []string
}

// Path returns the opaque distribution path for this QOA stream. The Karty
// extension discourages treating verified sidecars as loose source media.
func (stream AudioStream) Path() string { return "content/" + stream.SHA256 + ".kaud" }

func validateAudioStreams(streams []AudioStream) error {
	if len(streams) == 0 || len(streams) > MaxAudioStreamCount {
		return ErrAudioStreams
	}
	previous := map[AudioStreamKind]string{}
	nextID := map[AudioStreamKind]uint32{AudioStreamMusic: 1, AudioStreamEnvironment: 1}
	environmentStarted := false
	for _, stream := range streams {
		if stream.Kind != AudioStreamMusic && stream.Kind != AudioStreamEnvironment {
			return ErrAudioStreams
		}
		if stream.Kind == AudioStreamEnvironment {
			environmentStarted = true
		} else if environmentStarted {
			return ErrAudioStreams
		}
		if stream.ID != nextID[stream.Kind] || !validManifestString(stream.Name) || stream.Name <= previous[stream.Kind] ||
			(stream.Channels != 1 && stream.Channels != 2) || !supportedSampleRate(stream.SampleRate) || stream.Frames == 0 ||
			uint64(stream.Frames) > uint64(stream.SampleRate)*asset.MaxAudioStreamDurationSeconds ||
			stream.Size <= MediaEnvelopeHeaderSize || stream.Size > MaxAudioStreamSize || !validManifestHash(stream.SHA256) ||
			int64(len(stream.Chunks)) != (stream.Size+AudioStreamChunkSize-1)/AudioStreamChunkSize {
			return ErrAudioStreams
		}
		for _, digest := range stream.Chunks {
			if !validManifestHash(digest) {
				return ErrAudioStreams
			}
		}
		previous[stream.Kind] = stream.Name
		nextID[stream.Kind]++
	}
	return nil
}
