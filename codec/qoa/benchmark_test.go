package qoa_test

import (
	"bytes"
	"github.com/karty-game/karty-sdk/codec/qoa"
	"io"
	"testing"
)

func BenchmarkStreamDecoder(b *testing.B) {
	encoded, _, err := qoa.EncodeStream(make([]int16, 48000*2), 2, 48000)
	if err != nil {
		b.Fatal(err)
	}
	buffer := make([]byte, 4096)
	b.ReportAllocs()
	for b.Loop() {
		stream, err := qoa.NewStreamDecoder(bytes.NewReader(encoded))
		if err != nil {
			b.Fatal(err)
		}
		for {
			_, err = stream.Read(buffer)
			if err == io.EOF {
				break
			}
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}
