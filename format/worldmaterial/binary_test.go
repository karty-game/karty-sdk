package worldmaterial_test

import (
	"bytes"
	"testing"

	"github.com/karty-game/karty-sdk/format/level"
)

func materialByte(encoded []byte, at int, value byte) []byte {
	out := bytes.Clone(encoded)
	out[at] = value
	return out
}
func materialMetadata(t *testing.T) []byte {
	t.Helper()
	encoded, err := level.EncodeMetadata(level.Metadata{"kartyWorldMaterialAtlas": level.StringValue("karty.world-material-atlas@1")})
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
