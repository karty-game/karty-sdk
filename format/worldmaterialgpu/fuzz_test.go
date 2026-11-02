package worldmaterialgpu

import (
	"bytes"
	"testing"

	"github.com/karty-game/karty-sdk/format/worldmaterial"
)

func FuzzDescriptor(f *testing.F) {
	layout, err := worldmaterial.NewLayout([]uint32{7, 42})
	if err != nil {
		f.Fatal(err)
	}

	descriptor, err := NewDescriptor(layout)
	if err != nil {
		f.Fatal(err)
	}

	seed, err := EncodeDescriptor(descriptor)
	if err != nil {
		f.Fatal(err)
	}

	f.Add(seed)
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		descriptor, err := DecodeDescriptor(data)
		if err != nil {
			if len(descriptor.MaterialIDs) != 0 {
				t.Fatal("invalid descriptor returned partial material IDs")
			}

			return
		}

		encoded, err := EncodeDescriptor(descriptor)
		if err != nil || !bytes.Equal(data, encoded) {
			t.Fatal("accepted descriptor is not a canonical round trip", err)
		}
	})
}
