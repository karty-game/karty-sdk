package level_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/karty-game/karty-sdk/format/level"
)

func TestEnvelopeRejectsDuplicateTextureIDs(t *testing.T) {
	t.Parallel()

	// Distinct, sorted logical keys must not alias the same numeric texture ID.
	upper := "@texture/0000000A"
	lower := level.TextureEntryName(10)
	for _, name := range []string{upper, lower} {
		if id, ok := level.TextureAssetID(name); !ok || id != 10 {
			t.Fatalf("TextureAssetID(%q) = %d, %t", name, id, ok)
		}
		// Preserve existing acceptance of either spelling on its own.
		encoded, err := level.Encode([]byte(`{}`), []level.SourceEntry{
			{Name: name, Kind: level.EntryTexture, Data: []byte{1}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := level.Decode(encoded); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := level.Encode([]byte(`{}`), []level.SourceEntry{
		{Name: upper, Kind: level.EntryTexture, Data: []byte{1}},
		{Name: lower, Kind: level.EntryTexture, Data: []byte{2}},
	}); !errors.Is(err, level.ErrEntry) {
		t.Errorf("Encode() accepted aliased texture IDs: %v", err)
	}

	encoded, err := level.Encode([]byte(`{}`), []level.SourceEntry{
		{Name: level.TextureEntryName(9), Kind: level.EntryTexture, Data: []byte{1}},
		{Name: lower, Kind: level.EntryTexture, Data: []byte{2}},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Keep table ordering, lengths and offsets valid; only identity is invalid.
	encoded = bytes.Replace(encoded, []byte(level.TextureEntryName(9)), []byte(upper), 1)
	if _, err := level.Decode(encoded); !errors.Is(err, level.ErrEntry) {
		t.Fatalf("Decode() accepted aliased texture IDs: %v", err)
	}
}
