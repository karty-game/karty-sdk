package performance

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"reflect"
	"testing"
)

func TestBinaryPerformancePreservesResolvedProfiles(t *testing.T) {
	config := Default()
	off := false
	config.Profiles[1].FloorReflections = &off
	data, err := Encode(config)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(data)
	if err != nil || !reflect.DeepEqual(config, decoded) {
		t.Fatal(decoded, err)
	}
	for size := range data {
		if _, err := Decode(data[:size]); err == nil {
			t.Fatalf("accepted truncation%d", size)
		}
	}
	for _, bad := range [][]byte{[]byte(`{"target_fps":60}`), append(bytes.Clone(data), 0)} {
		if _, err := Decode(bad); err == nil {
			t.Fatal("acceptedJSON/trailing")
		}
	}
	for _, mutate := range []func([]byte){func(d []byte) { d[4] = 1 }, func(d []byte) { d[6] = 1 }, func(d []byte) { binary.LittleEndian.PutUint32(d[16:20], math.MaxUint32) }, func(d []byte) { d[len(d)-1] = 3 }} {
		bad := bytes.Clone(data)
		mutate(bad)
		if _, err := Decode(bad); err == nil {
			t.Fatal("accepted malformed config")
		}
	}
	defaults, err := Encode(Config{})
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := Decode(defaults)
	if err != nil || !reflect.DeepEqual(Default(), resolved) {
		t.Fatal("defaults changed", err)
	}
}

func TestBinaryPerformanceRejectsWidthGap(t *testing.T) {
	config := Default()
	config.Profiles[1].MinWindowWidth = 800
	config.Profiles[2].MaxWindowWidth = 799
	data, err := Encode(config)
	if err != nil {
		t.Fatal(err)
	}
	// Find the medium profile's fields using the documented header and record
	// layout, then introduce a gap without changing lengths or numeric validity.
	offset := 16 + 4 + len(config.DefaultProfile) + 4
	for _, profile := range config.Profiles {
		offset += 4 + len(profile.Name)
		if profile.Name == "medium" {
			binary.LittleEndian.PutUint32(data[offset+8:offset+12], 1401)
			break
		}
		offset += 17
	}
	if _, err := Decode(data); !errors.Is(err, ErrConfig) {
		t.Fatalf("expected decode to reject width gap, got %v", err)
	}
}

func FuzzBinaryPerformance(f *testing.F) {
	data, _ := Encode(Default())
	f.Add(data)
	f.Fuzz(func(t *testing.T, data []byte) {
		config, err := Decode(data)
		if err != nil {
			return
		}
		if err := Validate(config); err != nil {
			t.Fatal(err)
		}
	})
}
