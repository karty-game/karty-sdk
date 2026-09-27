package cartridge

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"

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
	ID         uint32          `json:"id"`
	Name       string          `json:"name"`
	Kind       AudioStreamKind `json:"kind"`
	Channels   uint8           `json:"channels"`
	SampleRate uint32          `json:"sampleRate"`
	Frames     uint32          `json:"frames"`
	Size       int64           `json:"size"`
	SHA256     string          `json:"sha256"`
	Chunks     []string        `json:"chunks"`
}

// Path returns the opaque distribution path for this QOA stream. The Karty
// extension discourages treating verified sidecars as loose source media.
func (stream AudioStream) Path() string { return "content/" + stream.SHA256 + ".kaud" }

type audioStreamCatalog struct {
	Version int           `json:"version"`
	Streams []AudioStream `json:"streams"`
}

func EncodeAudioStreams(streams []AudioStream) ([]byte, error) {
	if err := validateAudioStreams(streams); err != nil {
		return nil, err
	}
	data, err := json.Marshal(audioStreamCatalog{Version: 1, Streams: streams})
	if err != nil || len(data) > MaxAudioStreamCatalog {
		return nil, ErrAudioStreams
	}
	return data, nil
}

func DecodeAudioStreams(data []byte) ([]AudioStream, error) {
	if len(data) > MaxAudioStreamCatalog {
		return nil, ErrAudioStreams
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var catalog audioStreamCatalog
	if decoder.Decode(&catalog) != nil || catalog.Version != 1 || decoder.Decode(new(any)) != io.EOF {
		return nil, ErrAudioStreams
	}
	if err := validateAudioStreams(catalog.Streams); err != nil {
		return nil, err
	}
	return catalog.Streams, nil
}

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
