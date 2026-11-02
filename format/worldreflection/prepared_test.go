package worldreflection_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldgpuchunk"
	"github.com/karty-game/karty-sdk/format/worldreflection"
)

func TestPreparedReflectionKeepsOneImageAndExactSource(t *testing.T) {
	t.Parallel()
	metadata, envelope := fixture(t)
	metadata.Producer = worldreflection.GPUProducer
	metadata.Probes[0].Image = nil

	texture, err := os.ReadFile("testdata/prepared.ktx2")
	if err != nil {
		t.Fatal(err)
	}

	prepared := worldreflection.Prepared{Metadata: metadata, Texture: texture, DrawProbes: [][]byte{{1, 0, 0, 0, 0, 0, 0, 0}}}

	data, err := worldreflection.EncodePrepared(prepared)
	if err != nil {
		t.Fatal(err)
	}

	document := &world.Document{Sectors: make([]world.Sector, 2), StaticSolids: &world.StaticSolids{Items: make([]world.Solid, 1)}}

	decoded, info, err := worldreflection.DecodePreparedSource(data, envelope, document)
	if err != nil || len(info.Levels) != 4 || info.RGBABytes != worldreflection.GPUImageBytes ||
		len(decoded.Metadata.Probes[0].Image) != 0 || !bytes.Equal(decoded.Texture, texture) ||
		len(decoded.DrawProbes) != 1 || !bytes.Equal(decoded.DrawProbes[0], prepared.DrawProbes[0]) {
		t.Fatal("prepared source, complete allocation or single image changed", info, err)
	}

	if _, err := worldreflection.Decode(data); err == nil {
		t.Fatal("prepared data accepted as offline QOI capture")
	}

	for _, size := range []int{0, 3, 8, 64, len(data) - 1} {
		if _, err := worldreflection.DecodePrepared(data[:size]); err == nil {
			t.Fatal("truncated complete bundle accepted", size)
		}
	}

	envelope.Metadata = []byte("changed lighting inputs")
	if _, _, err := worldreflection.DecodePreparedSource(data, envelope, document); err == nil {
		t.Fatal("stale prepared reflection source accepted")
	}

	prepared.Metadata.Probes[0].Image = []byte("duplicate capture")
	if _, err := worldreflection.EncodePrepared(prepared); err == nil {
		t.Fatal("prepared image accepted an additional QOI copy")
	}
}

func TestPreparedDrawProbeTablesRejectMalformedSlotsAndBounds(t *testing.T) {
	t.Parallel()
	metadata, _ := fixture(t)
	metadata.Producer = worldreflection.GPUProducer
	metadata.Probes[0].Image = nil

	texture, err := os.ReadFile("testdata/prepared.ktx2")
	if err != nil {
		t.Fatal(err)
	}

	for _, tables := range [][][]byte{
		{{}}, {{1, 0, 0}}, {{2, 0, 0, 0}},
		{make([]byte, 4*(worldgpuchunk.MaxDraws+1))},
		make([][]byte, worldgpuchunk.MaxCells+1),
	} {
		if _, err := worldreflection.EncodePrepared(
			worldreflection.Prepared{Metadata: metadata, Texture: texture, DrawProbes: tables},
		); err == nil {
			t.Fatal("malformed or oversized final slot table admitted")
		}
	}

	prepared := worldreflection.Prepared{Metadata: metadata, Texture: texture, DrawProbes: [][]byte{{1, 0, 0, 0}}}

	data, err := worldreflection.EncodePrepared(prepared)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := worldreflection.DecodePrepared(data)
	if err != nil {
		t.Fatal(err)
	}

	borrowed := false

	for at := range data {
		borrowed = borrowed || &data[at] == &decoded.DrawProbes[0][0]
	}

	if !borrowed {
		t.Fatal("final probe slots copied during decoding")
	}

	decoded.DrawProbes[0][0] = 2

	if _, err := worldreflection.DecodePrepared(data); err == nil {
		t.Fatal("out-of-range final slot accepted from wire")
	}
}
