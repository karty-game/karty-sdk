package cartridge

import (
	"bytes"
	"strings"
	"testing"
)

func TestVideoCatalogRoundTripAndBounds(t *testing.T) {
	hash := strings.Repeat("a", 64)
	entry := Video{ID: 1, Name: "intro", Size: 128, SHA256: hash, Chunks: []string{hash}}
	data, err := EncodeVideos([]Video{entry})
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeVideos(data)
	if err != nil || len(got) != 1 || got[0].Path() != "content/"+hash+".kvid" {
		t.Fatalf("round trip: %v %v", got, err)
	}
	for _, mutate := range []func(*Video){
		func(v *Video) { v.ID = 0 }, func(v *Video) { v.Size = MaxVideoSize + 1 },
		func(v *Video) { v.Chunks = nil }, func(v *Video) { v.SHA256 = "../escape" },
	} {
		bad := entry
		mutate(&bad)
		if _, err := EncodeVideos([]Video{bad}); err == nil {
			t.Fatal("accepted invalid video")
		}
	}
	for _, bad := range [][]byte{append(bytes.Clone(data), 'x'), bytes.Replace(data, []byte(`"version":1`), []byte(`"version":2`), 1)} {
		if _, err := DecodeVideos(bad); err == nil {
			t.Fatal("accepted invalid catalog")
		}
	}
}
