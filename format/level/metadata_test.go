package level_test

import (
	"bytes"
	"encoding/binary"
	"math"
	"strings"
	"testing"

	"github.com/karty-game/karty-sdk/format/level"
)

func metadataBytes(t testHelper, metadata level.Metadata) []byte {
	t.Helper()
	data, err := level.EncodeMetadata(metadata)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func TestMetadataPreservesTypedValuesAndCanonicalOrder(t *testing.T) {
	source := level.Metadata{
		"uint":  level.UintValue(math.MaxUint64),
		"int":   level.IntValue(math.MinInt64),
		"float": level.FloatValue(2.5),
		"bytes": level.BytesValue([]byte{0, 255}),
		"custom": level.ArrayValue([]level.Value{
			level.NullValue(),
			level.BoolValue(true),
			level.ObjectValue(level.Metadata{"name": level.StringValue("héllo")}),
		}),
	}
	data := metadataBytes(t, source)
	decoded, err := level.DecodeMetadata(data)
	if err != nil {
		t.Fatal(err)
	}
	uintValue, _ := decoded.Uint("uint")
	intValue, _ := decoded.Int("int")
	if uintValue != math.MaxUint64 || intValue != math.MinInt64 || decoded["float"].Float != 2.5 ||
		!bytes.Equal(decoded["bytes"].Bytes, []byte{0, 255}) {
		t.Fatal("typed precision lost")
	}
	again := metadataBytes(t, decoded)
	if !bytes.Equal(data, again) || string(data[:4]) != "KTYM" {
		t.Fatal("metadata is not canonical binary")
	}
	for size := range data {
		if _, err := level.DecodeMetadata(data[:size]); err == nil {
			t.Fatalf("accepted truncation %d", size)
		}
	}
}
func TestMetadataRejectsMalformedAndLegacyJSON(t *testing.T) {
	valid := metadataBytes(t, level.Metadata{"a": level.StringValue("x"), "b": level.BoolValue(false)})
	for _, data := range [][]byte{[]byte(`{}`), []byte(`{"name":"legacy"}`), append(bytes.Clone(valid), 0)} {
		if _, err := level.DecodeMetadata(data); err == nil {
			t.Fatal("accepted malformed or JSON metadata")
		}
	}
	for _, mutate := range []func([]byte){
		func(data []byte) { data[4] = 1 }, func(data []byte) { data[6] = 1 },
		func(data []byte) { data[8] = 9 }, func(data []byte) { binary.LittleEndian.PutUint32(data[9:13], math.MaxUint32) },
		func(data []byte) { data[17] = 'b' }, // Duplicate sorted key.
		func(data []byte) { data[18] = 255 }, // Unknown value tag.
	} {
		data := bytes.Clone(valid)
		mutate(data)
		if _, err := level.DecodeMetadata(data); err == nil {
			t.Fatal("accepted malformed metadata")
		}
	}
}
func TestMetadataBounds(t *testing.T) {
	for _, key := range []string{"", strings.Repeat("x", level.MaxMetadataKeySize+1)} {
		if _, err := level.EncodeMetadata(level.Metadata{key: level.NullValue()}); err == nil {
			t.Fatal("accepted invalid key")
		}
	}
	metadataBytes(t, level.Metadata{strings.Repeat("x", level.MaxMetadataKeySize): level.NullValue()})
	for _, value := range []level.Value{level.FloatValue(math.NaN()), level.FloatValue(math.Inf(1)), level.StringValue(string([]byte{255})), level.BytesValue(make([]byte, level.MaxMetadataSize))} {
		if _, err := level.EncodeMetadata(level.Metadata{"v": value}); err == nil {
			t.Fatal("accepted invalid value")
		}
	}
	value := level.NullValue()
	for range level.MaxMetadataDepth - 1 {
		value = level.ArrayValue([]level.Value{value})
	}
	metadataBytes(t, level.Metadata{"v": value}) // root0, leaf16.
	value = level.ArrayValue([]level.Value{value})
	if _, err := level.EncodeMetadata(level.Metadata{"v": value}); err == nil {
		t.Fatal("accepted depth17")
	}
	if _, err := level.EncodeMetadata(level.Metadata{"v": level.ArrayValue(make([]level.Value, level.MaxMetadataNodes))}); err == nil {
		t.Fatal("accepted too many nodes")
	}
}
func FuzzMetadataDecode(f *testing.F) {
	f.Add(metadataBytes(f, level.Metadata{"v": level.UintValue(42)}))
	f.Fuzz(func(t *testing.T, data []byte) {
		metadata, err := level.DecodeMetadata(data)
		if err != nil {
			return
		}
		encoded := metadataBytes(t, metadata)
		if !bytes.Equal(data, encoded) {
			t.Fatal("noncanonical input accepted")
		}
	})
}
