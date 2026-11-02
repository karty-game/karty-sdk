package ktx2

import (
	"encoding/binary"
	"os"
	"testing"
)

// Structural fixture: raw Zstd blocks contain opaque UASTC bytes. Inspector
// success deliberately does not assert that those bytes transcode successfully.
func fixture(width, height, layers uint32, srgb bool) []byte {
	levels := 1
	for n := max(width, height); n > 1; n >>= 1 {
		levels++
	}
	data := make([]byte, HeaderSize+levels*24+44)
	copy(data, identifier)
	put32 := func(offset int, value uint32) { binary.LittleEndian.PutUint32(data[offset:], value) }
	put32(16, 1)
	put32(20, width)
	put32(24, height)
	put32(32, layers)
	put32(36, 1)
	put32(40, uint32(levels))
	put32(44, 2)
	dfd := HeaderSize + levels*24
	put32(48, uint32(dfd))
	put32(52, 44)
	put32(dfd, 44)
	put32(dfd+8, 40<<16|2)
	data[dfd+12], data[dfd+13], data[dfd+14] = 166, 1, 1
	if srgb {
		data[dfd+14] = 2
	}
	data[dfd+16], data[dfd+17], data[dfd+20] = 3, 3, 16
	data[dfd+30], data[dfd+31] = 127, 3
	put32(dfd+40, 0xffffffff)
	for i := levels - 1; i >= 0; i-- {
		count := ((max(1, width>>i) + 3) / 4) * ((max(1, height>>i) + 3) / 4) * layers * 16
		frame := []byte{0x28, 0xb5, 0x2f, 0xfd, 0xa0, 0, 0, 0, 0}
		binary.LittleEndian.PutUint32(frame[5:], count)
		for remaining := count; remaining > 0; {
			block := min(remaining, 128<<10)
			remaining -= block
			header := block << 3
			if remaining == 0 {
				header |= 1
			}
			frame = append(frame, byte(header), byte(header>>8), byte(header>>16))
			frame = append(frame, make([]byte, block)...)
		}
		index := HeaderSize + i*24
		binary.LittleEndian.PutUint64(data[index:], uint64(len(data)))
		binary.LittleEndian.PutUint64(data[index+8:], uint64(len(frame)))
		binary.LittleEndian.PutUint64(data[index+16:], uint64(count))
		data = append(data, frame...) //nolint:makezero // The initialized container header precedes the appended mip payloads.
	}
	return data
}

func TestInspectPreparedUASTCArray(t *testing.T) {
	data := fixture(256, 256, 2, true)
	info, err := Inspect(data)
	if err != nil || info.Width != 256 || info.Layers != 2 || !info.SRGB || len(info.Levels) != 9 ||
		info.RGBABytes != 699048 || info.InflatedBytes != 174816 {
		t.Fatal(info, err)
	}
}

func TestInspectUpstreamEncodedOpaqueArray(t *testing.T) {
	data, err := os.ReadFile("testdata/opaque-array.ktx2")
	if err != nil {
		t.Fatal(err)
	}
	info, err := Inspect(data)
	if err != nil || info.Width != 256 || info.Height != 256 || info.Layers != 2 || info.SRGB || info.HasAlpha || len(info.Levels) != 9 ||
		info.InflatedBytes != 174816 {
		t.Fatal(info, err)
	}
}

func TestInspectRejectsMalformedContainers(t *testing.T) {
	tests := map[string]func([]byte) []byte{
		"truncated header":         func(b []byte) []byte { return b[:79] },
		"trailing data":            func(b []byte) []byte { return append(b, 0) },
		"overflowing level offset": func(b []byte) []byte { binary.LittleEndian.PutUint64(b[80:], ^uint64(0)); return b },
		"overflowing level length": func(b []byte) []byte { binary.LittleEndian.PutUint64(b[88:], ^uint64(0)); return b },
		"overlapping levels":       func(b []byte) []byte { copy(b[80:88], b[104:112]); return b },
		"inflated length lie":      func(b []byte) []byte { b[96]++; return b },
		"missing mips":             func(b []byte) []byte { b[40]--; return b },
		"nonarray":                 func(b []byte) []byte { b[32] = 0; return b },
		"excessive layers":         func(b []byte) []byte { binary.LittleEndian.PutUint32(b[32:], ^uint32(0)); return b },
		"cube":                     func(b []byte) []byte { b[36] = 6; return b },
		"basislz":                  func(b []byte) []byte { b[44] = 1; return b },
		"swizzled channels":        func(b []byte) []byte { b[80+9*24+31] = 5; return b },
		"premultiplied":            func(b []byte) []byte { b[80+9*24+15] = 1; return b },
		"unknown transfer":         func(b []byte) []byte { b[80+9*24+14] = 3; return b },
		"non4x4":                   func(b []byte) []byte { b[80+9*24+16] = 5; return b },
		"zstd dictionary":          func(b []byte) []byte { o := binary.LittleEndian.Uint64(b[80:]); b[o+4] |= 1; return b },
		"zstd content length":      func(b []byte) []byte { o := binary.LittleEndian.Uint64(b[80:]); b[o+5]++; return b },
		"zstd reserved block":      func(b []byte) []byte { o := binary.LittleEndian.Uint64(b[80:]); b[o+9] |= 6; return b },
	}
	for name, modify := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Inspect(modify(fixture(256, 256, 2, false))); err == nil {
				t.Fatal("malformed container accepted")
			}
		})
	}
}

func TestInspectMetadataRejectsInterpretationChanges(t *testing.T) {
	makeEntry := func(key, value string) []byte {
		entry := append(append([]byte(key), 0), value...)
		data := binary.LittleEndian.AppendUint32(nil, uint32(len(entry)))
		data = append(data, entry...)
		for len(data)%4 != 0 {
			data = append(data, 0)
		}
		return data
	}
	for _, test := range []struct {
		key, value string
		valid      bool
	}{
		{"KTXorientation", "rd\x00", true}, {"KTXorientation", "ru\x00", false},
		{"KTXswizzle", "rgba\x00", true}, {"KTXswizzle", "rrrg\x00", false},
		{"KTXwriter", "fixture\x00", true}, {"unknown", "opaque", false},
	} {
		if (inspectMetadata(makeEntry(test.key, test.value)) == nil) != test.valid {
			t.Fatal(test)
		}
	}
	entry := makeEntry("KTXwriter", "fixture\x00")
	if inspectMetadata(append(entry, entry...)) == nil {
		t.Fatal("duplicate metadata accepted")
	}
}

func TestZstdFramePreflightRejectsUnboundedWindowsAndExtraFrames(t *testing.T) {
	// Content size is 16, block is a final RLE block of sixteen zero bytes.
	frame := []byte{0x28, 0xb5, 0x2f, 0xfd, 0xa0, 16, 0, 0, 0, 0x83, 0, 0, 0}
	if err := inspectZstd(frame, 16); err != nil {
		t.Fatal(err)
	}
	if inspectZstd(append(append([]byte{}, frame...), frame...), 16) == nil {
		t.Fatal("concatenated frames accepted")
	}
	// Non-single-segment framing with a window larger than the output budget.
	largeWindow := append([]byte{0x28, 0xb5, 0x2f, 0xfd, 0x80, 80}, frame[5:]...)
	if inspectZstd(largeWindow, 16) == nil {
		t.Fatal("excessive Zstd window accepted")
	}
}

func FuzzInspect(f *testing.F) {
	f.Add(fixture(4, 4, 1, true))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) { _, _ = Inspect(data) })
}
