package qoa_test

import (
	"encoding/binary"
	"encoding/hex"
	"reflect"
	"testing"

	"github.com/karty-game/karty-sdk/codec/qoa"
	"github.com/karty-game/karty-sdk/format/asset"
)

// Generated with the official phoboslab/qoa reference implementation from
// commit f73b4a36f40bc022abeb32575716c80e49bdd572 and the PCM input below.
// This proves interoperability independently of the Go dependency used by
// the adapter.
const referenceGolden = "716f6166000000140100bb8000140020000000000000000000000000e0004000f04ab2ef2ef1ab1d"

var referencePCM = []int16{
	0, 1000, -1000, 2000, -2000, 3000, -3000, 4000, -4000, 5000,
	-5000, 6000, -6000, 7000, -7000, 8000, -8000, 9000, -9000, 0,
}

var referenceDecoded = []int16{
	1536, 1554, 36, 3601, -1867, 1738, -4022, 5387, -2340, 3153,
	-5744, 6900, -5157, 5847, -5231, 6172, -9595, 7583, -7514, -1590,
}

func TestOfficialReferenceInteroperability(t *testing.T) {
	t.Parallel()

	golden, err := hex.DecodeString(referenceGolden)
	if err != nil {
		t.Fatal(err)
	}
	metadata, decoded, err := qoa.Decode(golden)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Channels != 1 || metadata.SampleRate != 48_000 || metadata.Frames != 20 ||
		!reflect.DeepEqual(decoded, referenceDecoded) {
		t.Fatalf("Decode() = %+v, %v", metadata, decoded)
	}

	encoded, gotMetadata, err := qoa.Encode(referencePCM, 1, 48_000)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(encoded, golden) || gotMetadata != metadata {
		t.Fatalf("Encode() did not match reference: %x", encoded)
	}
}

func TestInspectRejectsUnsafeFilesWithoutPanic(t *testing.T) {
	t.Parallel()

	golden, err := hex.DecodeString(referenceGolden)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func([]byte) []byte{
		"nine channels": func(data []byte) []byte { data[8] = 9; return data },
		"overflow": func(data []byte) []byte {
			binary.BigEndian.PutUint32(data[4:8], ^uint32(0))
			return data
		},
		"frame beyond total": func(data []byte) []byte {
			binary.BigEndian.PutUint32(data[4:8], 1)
			return data
		},
		"truncated":      func(data []byte) []byte { return data[:len(data)-1] },
		"trailing bytes": func(data []byte) []byte { return append(data, 0) },
		"nonzero padding": func(data []byte) []byte {
			binary.BigEndian.PutUint32(data[4:8], 19)
			binary.BigEndian.PutUint16(data[12:14], 19)
			return data
		},
	} {
		t.Run(name, func(t *testing.T) {
			data := mutate(append([]byte(nil), golden...))
			if _, err := qoa.Inspect(data); err == nil {
				t.Fatal("Inspect() accepted unsafe QOA")
			}
			if _, _, err := qoa.Decode(data); err == nil {
				t.Fatal("Decode() accepted unsafe QOA")
			}
		})
	}
}

func TestEncodeRejectsInvalidLengthsAndBudgets(t *testing.T) {
	t.Parallel()

	for name, test := range map[string]struct {
		samples  []int16
		channels uint8
		rate     uint32
	}{
		"empty":         {channels: 1, rate: 48_000},
		"partial frame": {samples: []int16{1, 2, 3}, channels: 2, rate: 48_000},
		"channels":      {samples: []int16{1}, channels: 9, rate: 48_000},
		"sample rate":   {samples: []int16{1}, channels: 1, rate: 96_000},
		"duration":      {samples: make([]int16, 22_050*asset.MaxSoundDurationSeconds+1), channels: 1, rate: 22_050},
		"decoded budget": {
			samples: make([]int16, asset.MaxDecodedSoundBytes/2+1), channels: 1, rate: 48_000,
		},
	} {
		if _, _, err := qoa.Encode(test.samples, test.channels, test.rate); err == nil {
			t.Errorf("Encode() accepted %s", name)
		}
	}
}

func FuzzInspectAndDecode(f *testing.F) {
	golden, err := hex.DecodeString(referenceGolden)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(golden)
	f.Add([]byte("qoaf"))
	f.Add(append([]byte(nil), golden[:len(golden)-1]...))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = qoa.Inspect(data)
		_, _, _ = qoa.Decode(data)
	})
}
