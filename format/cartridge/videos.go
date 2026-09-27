package cartridge

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

const (
	VideoSectionName    = "karty.videos.v1"
	FeatureVideoMPEG1v1 = "video/mpeg1@1"
	VideoChunkSize      = 128 << 10
	MaxVideoPayloadSize = 64 << 20
	MaxVideoSize        = MaxVideoPayloadSize + MediaEnvelopeHeaderSize
	MaxVideoCount       = 64
	MaxVideoCatalog     = 3 << 20
)

var ErrVideos = errors.New("invalid MPEG-1 video catalog")

// Video describes a separate Karty media envelope containing MPEG-PS. Size and
// digests cover the stored envelope bytes. Each chunk is authenticated before
// it reaches the decoder, allowing bounded streaming without a full download.
// IDs are one-based positions in name order, local to a single build.
type Video struct {
	ID     uint32   `json:"id"`
	Name   string   `json:"name"`
	Size   int64    `json:"size"`
	SHA256 string   `json:"sha256"`
	Chunks []string `json:"chunks"`
}

// Path returns the opaque distribution path for this MPEG-1 stream. The Karty
// extension keeps staged media distinct from author-owned source files.
func (video Video) Path() string { return "content/" + video.SHA256 + ".kvid" }

type videoCatalog struct {
	Version int     `json:"version"`
	Videos  []Video `json:"videos"`
}

func EncodeVideos(videos []Video) ([]byte, error) {
	if err := validateVideos(videos); err != nil {
		return nil, err
	}
	data, err := json.Marshal(videoCatalog{Version: 1, Videos: videos})
	if err != nil || len(data) > MaxVideoCatalog {
		return nil, ErrVideos
	}
	return data, nil
}

func DecodeVideos(data []byte) ([]Video, error) {
	if len(data) > MaxVideoCatalog {
		return nil, ErrVideos
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var catalog videoCatalog
	if decoder.Decode(&catalog) != nil || catalog.Version != 1 {
		return nil, ErrVideos
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, ErrVideos
	}
	if err := validateVideos(catalog.Videos); err != nil {
		return nil, err
	}
	return catalog.Videos, nil
}

func validateVideos(videos []Video) error {
	if len(videos) == 0 || len(videos) > MaxVideoCount {
		return ErrVideos
	}
	previous := ""
	for index, video := range videos {
		if video.ID != uint32(index+1) || !validManifestString(video.Name) || video.Name <= previous ||
			video.Size <= MediaEnvelopeHeaderSize || video.Size > MaxVideoSize || !validManifestHash(video.SHA256) ||
			int64(len(video.Chunks)) != (video.Size+VideoChunkSize-1)/VideoChunkSize {
			return ErrVideos
		}
		for _, hash := range video.Chunks {
			if !validManifestHash(hash) {
				return ErrVideos
			}
		}
		previous = video.Name
	}
	return nil
}
