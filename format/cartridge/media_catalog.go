package cartridge

import "encoding/binary"

const MediaCatalogBinaryVersion = uint16(2)

func catalogHeader(magic string, count int) []byte {
	data := append([]byte(magic), 2, 0, 0, 0)
	return binary.LittleEndian.AppendUint32(data, uint32(count))
}
func catalogName(data []byte, id uint32, name string) []byte {
	data = binary.LittleEndian.AppendUint32(data, id)
	data = binary.LittleEndian.AppendUint32(data, uint32(len(name)))
	return append(data, name...)
}
func catalogHashes(data []byte, size int64, hash string, chunks []string) []byte {
	data = binary.LittleEndian.AppendUint64(data, uint64(size))
	data = append(data, hash...)
	data = binary.LittleEndian.AppendUint32(data, uint32(len(chunks)))
	for _, chunk := range chunks {
		data = append(data, chunk...)
	}
	return data
}

func EncodeVideos(videos []Video) ([]byte, error) {
	if err := validateVideos(videos); err != nil {
		return nil, err
	}
	data := catalogHeader("KTVC", len(videos))
	for _, video := range videos {
		data = catalogHashes(catalogName(data, video.ID, video.Name), video.Size, video.SHA256, video.Chunks)
		if len(data) > MaxVideoCatalog {
			return nil, ErrVideos
		}
	}
	return data, nil
}
func EncodeAudioStreams(streams []AudioStream) ([]byte, error) {
	if err := validateAudioStreams(streams); err != nil {
		return nil, err
	}
	data := catalogHeader("KTAC", len(streams))
	for _, stream := range streams {
		data = catalogName(data, stream.ID, stream.Name)
		kind := byte(1)
		if stream.Kind == AudioStreamEnvironment {
			kind = 2
		}
		data = append(data, kind, stream.Channels, 0, 0)
		data = binary.LittleEndian.AppendUint32(data, stream.SampleRate)
		data = binary.LittleEndian.AppendUint32(data, stream.Frames)
		data = catalogHashes(data, stream.Size, stream.SHA256, stream.Chunks)
		if len(data) > MaxAudioStreamCatalog {
			return nil, ErrAudioStreams
		}
	}
	return data, nil
}

// DecodeVideos scans every bounded entry before allocating the catalog and chunk arrays.
func DecodeVideos(data []byte) ([]Video, error) {
	count, ok := validCatalogHeader(data, "KTVC", MaxVideoCatalog, MaxVideoCount)
	if !ok {
		return nil, ErrVideos
	}
	for _, allocate := range []bool{false, true} {
		r := catalogReader{data: data, offset: 12, allocate: allocate}
		var videos []Video
		if allocate {
			videos = make([]Video, count)
		}
		for index := 0; index < count; index++ {
			id, name, ok := r.name()
			if !ok {
				return nil, ErrVideos
			}
			size, hash, chunks, ok := r.hashes(MaxVideoSize, VideoChunkSize)
			if !ok {
				return nil, ErrVideos
			}
			if allocate {
				videos[index] = Video{ID: id, Name: name, Size: size, SHA256: hash, Chunks: chunks}
			}
		}
		if r.offset != len(data) {
			return nil, ErrVideos
		}
		if allocate {
			return videos, validateVideos(videos)
		}
	}
	return nil, ErrVideos
}
func DecodeAudioStreams(data []byte) ([]AudioStream, error) {
	count, ok := validCatalogHeader(data, "KTAC", MaxAudioStreamCatalog, MaxAudioStreamCount)
	if !ok {
		return nil, ErrAudioStreams
	}
	for _, allocate := range []bool{false, true} {
		r := catalogReader{data: data, offset: 12, allocate: allocate}
		var streams []AudioStream
		if allocate {
			streams = make([]AudioStream, count)
		}
		for index := 0; index < count; index++ {
			id, name, ok := r.name()
			if !ok {
				return nil, ErrAudioStreams
			}
			fields, ok := r.take(12)
			if !ok || (fields[0] != 1 && fields[0] != 2) || fields[2] != 0 || fields[3] != 0 {
				return nil, ErrAudioStreams
			}
			kind := AudioStreamMusic
			if fields[0] == 2 {
				kind = AudioStreamEnvironment
			}
			size, hash, chunks, ok := r.hashes(MaxAudioStreamSize, AudioStreamChunkSize)
			if !ok {
				return nil, ErrAudioStreams
			}
			if allocate {
				streams[index] = AudioStream{
					ID:         id,
					Name:       name,
					Kind:       kind,
					Channels:   fields[1],
					SampleRate: binary.LittleEndian.Uint32(fields[4:8]),
					Frames:     binary.LittleEndian.Uint32(fields[8:12]),
					Size:       size,
					SHA256:     hash,
					Chunks:     chunks,
				}
			}
		}
		if r.offset != len(data) {
			return nil, ErrAudioStreams
		}
		if allocate {
			return streams, validateAudioStreams(streams)
		}
	}
	return nil, ErrAudioStreams
}
func validCatalogHeader(data []byte, magic string, maxBytes, maxCount int) (int, bool) {
	if len(data) < 12 || len(data) > maxBytes || string(data[:4]) != magic ||
		binary.LittleEndian.Uint16(data[4:6]) != MediaCatalogBinaryVersion ||
		binary.LittleEndian.Uint16(data[6:8]) != 0 {
		return 0, false
	}
	count := binary.LittleEndian.Uint32(data[8:12])
	if count < 1 || uint64(count) > uint64(maxCount) || uint64(count)*85 > uint64(len(data)-12) {
		return 0, false
	}
	return int(count), true
}

type catalogReader struct {
	data     []byte
	offset   int
	allocate bool
}

func (r *catalogReader) take(n int) ([]byte, bool) {
	if n < 0 || n > len(r.data)-r.offset {
		return nil, false
	}
	v := r.data[r.offset : r.offset+n]
	r.offset += n
	return v, true
}
func (r *catalogReader) name() (uint32, string, bool) {
	fields, ok := r.take(8)
	if !ok {
		return 0, "", false
	}
	id, n := binary.LittleEndian.Uint32(fields[:4]), binary.LittleEndian.Uint32(fields[4:8])
	if n < 1 || n > MaxManifestString {
		return 0, "", false
	}
	name, ok := r.take(int(n))
	if !ok || !validManifestString(string(name)) {
		return 0, "", false
	}
	return id, string(name), true
}
func (r *catalogReader) hashes(maxSize, chunkSize int64) (int64, string, []string, bool) {
	fields, ok := r.take(76)
	if !ok {
		return 0, "", nil, false
	}
	sizeValue := binary.LittleEndian.Uint64(fields[:8])
	count := binary.LittleEndian.Uint32(fields[72:76])
	if sizeValue <= MediaEnvelopeHeaderSize || sizeValue > uint64(maxSize) ||
		uint64(count) != (sizeValue+uint64(chunkSize)-1)/uint64(chunkSize) ||
		uint64(count)*64 > uint64(len(r.data)-r.offset) ||
		!validManifestHash(string(fields[8:72])) {
		return 0, "", nil, false
	}
	var chunks []string
	if r.allocate {
		chunks = make([]string, int(count))
	}
	for index := 0; index < int(count); index++ {
		hash, ok := r.take(64)
		if !ok || !validManifestHash(string(hash)) {
			return 0, "", nil, false
		}
		if r.allocate {
			chunks[index] = string(hash)
		}
	}
	return int64(sizeValue), string(fields[8:72]), chunks, true
}
