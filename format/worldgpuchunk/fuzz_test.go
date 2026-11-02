package worldgpuchunk

import (
	"bytes"
	"testing"
)

func FuzzDecodeCatalog(f *testing.F) {
	seed, err := EncodeCatalog(catalogFixture())
	if err != nil {
		f.Fatal(err)
	}

	f.Add(seed)
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		catalog, err := DecodeCatalog(data)
		if err != nil {
			if len(catalog.Cells) != 0 || len(catalog.BakedLights) != 0 {
				t.Fatal("invalid catalog returned partial metadata")
			}

			return
		}

		canonical, err := EncodeCatalog(catalog)
		if err != nil {
			t.Fatal("accepted catalog cannot be encoded", err)
		}

		// Empty optional ID lists may arrive with absent or present counts.
		// Re-encoding must converge on the writer's canonical representation.
		roundTrip, err := DecodeCatalog(canonical)
		if err != nil {
			t.Fatal("canonical catalog cannot be decoded", err)
		}

		again, err := EncodeCatalog(roundTrip)
		if err != nil || !bytes.Equal(canonical, again) {
			t.Fatal("accepted catalog does not have a stable canonical round trip", err)
		}
	})
}

func FuzzDecodePayload(f *testing.F) {
	seed, err := EncodePayload(payloadFixture())
	if err != nil {
		f.Fatal(err)
	}

	f.Add(seed)
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		payload, err := DecodePayload(data)
		if err != nil {
			if len(payload.Vertices) != 0 || len(payload.Addresses) != 0 || len(payload.Indices) != 0 ||
				len(payload.Records) != 0 || len(payload.RecordRefs) != 0 || len(payload.Draws) != 0 {
				t.Fatal("invalid payload returned partial GPU buffers or references")
			}

			return
		}

		canonical, err := EncodePayload(payload)
		if err != nil {
			t.Fatal("accepted payload cannot be encoded", err)
		}

		roundTrip, err := DecodePayload(canonical)
		if err != nil {
			t.Fatal("canonical payload cannot be decoded", err)
		}

		again, err := EncodePayload(roundTrip)
		if err != nil || !bytes.Equal(canonical, again) {
			t.Fatal("accepted payload does not have a stable canonical round trip", err)
		}
	})
}
