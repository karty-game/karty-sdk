package ktx2

import (
	"encoding/binary"
	"os"
	"testing"
)

func TestLosslessSRGBMipsAreCompleteAndSeparateFromRNM(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("testdata/srgb-rgba8-mips.ktx2")
	if err != nil {
		t.Fatal(err)
	}
	info, err := InspectRGBA8Array(data)
	if err != nil || !info.SRGB || info.Width != 8 || info.Height != 8 || info.Layers != 2 || len(info.Levels) != 4 ||
		info.RGBABytes != 680 {
		t.Fatal(info, err)
	}
	if _, err := InspectRGBA8(data); err == nil {
		t.Fatal("prepared reflection array accepted as single linear RNM")
	}
	for _, mutate := range []func([]byte){
		func(b []byte) { binary.LittleEndian.PutUint64(b[80+3*24+16:], 1) },
		func(b []byte) { binary.LittleEndian.PutUint64(b[80+3*24:], 80) },
		func(b []byte) { binary.LittleEndian.PutUint32(b[12:], 37) },
		func(b []byte) { binary.LittleEndian.PutUint32(b[40:], 5) },
	} {
		invalid := append([]byte(nil), data...)
		mutate(invalid)
		if _, err := InspectRGBA8Array(invalid); err == nil {
			t.Fatal("malformed complete mip array accepted")
		}
	}
}
