package ktx2

import (
	"encoding/binary"
	"os"
	"testing"
)

func rgba8Fixture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/linear-rgba8-array.ktx2")
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func TestInspectRGBA8OfficialLosslessArray(t *testing.T) {
	t.Parallel()
	b := rgba8Fixture(t)
	info, err := InspectRGBA8(b)
	if err != nil || info.Width != 4 || info.Height != 4 || info.Layers != 3 || info.SRGB || !info.HasAlpha || len(info.Levels) != 1 ||
		info.RGBABytes != 192 ||
		info.InflatedBytes != 192 {
		t.Fatal(info, err)
	}
	if _, err := Inspect(b); err == nil {
		t.Fatal("lossless profile accepted as UASTC")
	}
	if _, err := InspectRGBA8(fixture(4, 4, 3, false)); err == nil {
		t.Fatal("UASTC accepted as raw lossless profile")
	}
	for end := range len(b) {
		if _, err := InspectRGBA8(b[:end]); err == nil {
			t.Fatal("truncated array accepted", end)
		}
	}
}
func TestInspectRGBA8RejectsFormatTransferChannelsAndBounds(t *testing.T) {
	t.Parallel()
	cases := map[string]func([]byte){
		"format": func(b []byte) { b[12] = 43 }, "type": func(b []byte) { b[16] = 4 }, "mips": func(b []byte) { b[40] = 2 },
		"cube": func(b []byte) { b[36] = 6 }, "nonarray": func(b []byte) { b[32] = 0 },
		"transfer": func(b []byte) { b[104+14] = 2 }, "premultiplied": func(b []byte) { b[104+15] = 1 },
		"alpha": func(b []byte) { b[104+28+3*16+3] = 2 }, "sample range": func(b []byte) { b[104+40] = 254 },
		"offset":   func(b []byte) { binary.LittleEndian.PutUint64(b[80:], ^uint64(0)) },
		"inflated": func(b []byte) { b[96]++ },
		"budget": func(b []byte) {
			binary.LittleEndian.PutUint32(b[20:], 2048)
			binary.LittleEndian.PutUint32(b[24:], 2048)
			binary.LittleEndian.PutUint32(b[32:], 256)
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			b := rgba8Fixture(t)
			change(b)
			if _, err := InspectRGBA8(b); err == nil {
				t.Fatal("malformed profile accepted")
			}
		})
	}
}
