package performance

import (
	"encoding/binary"
	"math"
	"unicode/utf8"
)

const BinaryVersion = uint16(2)

// Encode resolves authored defaults and creates a canonical binary profile ladder.
func Encode(config Config) ([]byte, error) {
	config, err := Normalize(config)
	if err != nil {
		return nil, err
	}
	data := []byte{'K', 'T', 'P', 'F', 2, 0, 0, 0}
	data = binary.LittleEndian.AppendUint64(data, math.Float64bits(config.TargetFPS))
	data = appendText(data, config.DefaultProfile)
	data = binary.LittleEndian.AppendUint32(data, uint32(len(config.Profiles)))
	for _, profile := range config.Profiles {
		if profile.MinWindowWidth > math.MaxInt32 || profile.MaxWindowWidth > math.MaxInt32 {
			return nil, ErrConfig
		}
		data = appendText(data, profile.Name)
		data = binary.LittleEndian.AppendUint64(data, math.Float64bits(profile.RenderScale))
		data = binary.LittleEndian.AppendUint32(data, uint32(profile.MinWindowWidth))
		data = binary.LittleEndian.AppendUint32(data, uint32(profile.MaxWindowWidth))
		flag := byte(0)
		if profile.FloorReflections != nil {
			flag = 1
			if *profile.FloorReflections {
				flag = 2
			}
		}
		data = append(data, flag)
	}
	return data, nil
}

func appendText(data []byte, text string) []byte {
	data = binary.LittleEndian.AppendUint32(data, uint32(len(text)))
	return append(data, text...)
}

// Decode rejects JSON and validates all lengths before the bounded profile allocation.
func Decode(data []byte) (Config, error) {
	if len(data) < 24 || len(data) > MaxConfig || string(data[:4]) != "KTPF" ||
		binary.LittleEndian.Uint16(data[4:6]) != BinaryVersion || binary.LittleEndian.Uint16(data[6:8]) != 0 {
		return Config{}, ErrConfig
	}
	r := configReader{data: data, offset: 8}
	fps, ok := r.take(8)
	if !ok {
		return Config{}, ErrConfig
	}
	name, ok := r.text()
	if !ok {
		return Config{}, ErrConfig
	}
	countBytes, ok := r.take(4)
	if !ok {
		return Config{}, ErrConfig
	}
	count := binary.LittleEndian.Uint32(countBytes)
	if count != 4 || uint64(count)*21 > uint64(len(data)-r.offset) {
		return Config{}, ErrConfig
	}
	config := Config{
		TargetFPS:      math.Float64frombits(binary.LittleEndian.Uint64(fps)),
		DefaultProfile: name,
		Profiles:       make([]Profile, int(count)),
	}
	for index := range config.Profiles {
		name, ok := r.text()
		if !ok {
			return Config{}, ErrConfig
		}
		fields, ok := r.take(17)
		if !ok || fields[16] > 2 {
			return Config{}, ErrConfig
		}
		profile := Profile{
			Name:           name,
			RenderScale:    math.Float64frombits(binary.LittleEndian.Uint64(fields[:8])),
			MinWindowWidth: int(int32(binary.LittleEndian.Uint32(fields[8:12]))),
			MaxWindowWidth: int(int32(binary.LittleEndian.Uint32(fields[12:16]))),
		}
		if fields[16] != 0 {
			value := fields[16] == 2
			profile.FloorReflections = &value
		}
		config.Profiles[index] = profile
	}
	if r.offset != len(data) {
		return Config{}, ErrConfig
	}
	if err := Validate(config); err != nil {
		return Config{}, err
	}
	return config, nil
}

type configReader struct {
	data   []byte
	offset int
}

func (r *configReader) take(n int) ([]byte, bool) {
	if n < 0 || n > len(r.data)-r.offset {
		return nil, false
	}
	v := r.data[r.offset : r.offset+n]
	r.offset += n
	return v, true
}
func (r *configReader) text() (string, bool) {
	count, ok := r.take(4)
	if !ok {
		return "", false
	}
	n := binary.LittleEndian.Uint32(count)
	if n > 128 {
		return "", false
	}
	v, ok := r.take(int(n))
	if !ok || !utf8.Valid(v) {
		return "", false
	}
	return string(v), true
}
