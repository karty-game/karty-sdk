package worldsky_test

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"math"
	"reflect"
	"runtime"
	"testing"

	"github.com/karty-game/karty-sdk/format/worldsky"
)

func packed(t *testing.T, pixels []byte) []byte {
	t.Helper()
	var output bytes.Buffer
	writer := gzip.NewWriter(&output)
	if _, err := writer.Write(pixels); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func fixture(t *testing.T) worldsky.Document {
	t.Helper()
	image := &worldsky.Image{Width: 2, Height: 1, Packed: packed(t, []byte{0, 1, 2, 255, 0, 0, 0, 0})}
	settings := worldsky.DefaultSettings()
	settings.HorizonUVScale = 1.5
	return worldsky.Document{Version: worldsky.Version, Settings: settings, CloudSparse: image, Horizon: image}
}

func TestRoundTripOwnershipAndImageFreeFallback(t *testing.T) {
	t.Parallel()
	for _, document := range []worldsky.Document{fixture(t), {Version: worldsky.Version, Settings: worldsky.DefaultSettings()}} {
		encoded, err := worldsky.Encode(document)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := worldsky.Decode(encoded)
		if err != nil || !reflect.DeepEqual(document, decoded) {
			t.Fatalf("roundtrip: %+v %v", decoded, err)
		}
		reencoded, err := worldsky.Encode(decoded)
		if err != nil || !bytes.Equal(encoded, reencoded) {
			t.Fatalf("canonical bytes: %v", err)
		}
		clear(encoded)
		if err := worldsky.Validate(&decoded); err != nil {
			t.Fatalf("decoded resource borrows encoded storage: %v", err)
		}
		if decoded.CloudSparse != nil {
			decoded.CloudSparse.Packed[0] = 0
			if decoded.Horizon.Packed[0] == 0 {
				t.Fatal("decoded image payloads alias")
			}
		}
	}
}

func TestUnpackStrictStreamAndDestinationReuse(t *testing.T) {
	t.Parallel()
	pixels := []byte{0, 1, 2, 255}
	valid := packed(t, pixels)
	corrupt := bytes.Clone(valid)
	corrupt[len(corrupt)-8] ^= 1
	for name, payload := range map[string][]byte{
		"bad header":      {0, 1, 2},
		"short output":    packed(t, pixels[:3]),
		"long output":     packed(t, append(bytes.Clone(pixels), 0)),
		"checksum":        corrupt,
		"missing trailer": valid[:len(valid)-1],
		"trailing bytes":  append(bytes.Clone(valid), 0),
		"second member":   append(bytes.Clone(valid), valid...),
	} {
		t.Run(name, func(t *testing.T) {
			image := worldsky.Image{Width: 1, Height: 1, Packed: payload}
			if _, err := image.Unpack(nil); !errors.Is(err, worldsky.ErrImage) {
				t.Fatalf("invalid stream accepted: %v", err)
			}
			document := worldsky.Document{Version: worldsky.Version, Settings: worldsky.DefaultSettings(), Background: &image}
			if _, err := worldsky.Encode(document); !errors.Is(err, worldsky.ErrImage) {
				t.Fatalf("invalid stream encoded: %v", err)
			}
		})
	}
	image := worldsky.Image{Width: 1, Height: 1, Packed: valid}
	destination := make([]byte, 0, 32)
	got, err := image.Unpack(destination)
	if err != nil || !bytes.Equal(got, pixels) || &got[0] != &destination[:cap(destination)][0] {
		t.Fatalf("reuse: %v", err)
	}
}

func TestDimensionsSettingsAndVersions(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(*worldsky.Document){
		"zero version":         func(d *worldsky.Document) { d.Version = 0 },
		"future version":       func(d *worldsky.Document) { d.Version++ },
		"negative width":       func(d *worldsky.Document) { d.CloudSparse.Width = -1 },
		"large dimension":      func(d *worldsky.Document) { d.CloudSparse.Width = math.MaxInt },
		"large pixels":         func(d *worldsky.Document) { d.CloudSparse.Width, d.CloudSparse.Height = 4096, 4096 },
		"missing packed":       func(d *worldsky.Document) { d.CloudSparse.Packed = nil },
		"zero scale":           func(d *worldsky.Document) { d.Settings.CloudUVScale[0] = 0 },
		"nan scale":            func(d *worldsky.Document) { d.Settings.CloudUVScale[0] = math.NaN() },
		"zero horizon scale":   func(d *worldsky.Document) { d.Settings.HorizonUVScale = 0 },
		"large horizon scale":  func(d *worldsky.Document) { d.Settings.HorizonUVScale = 4.1 },
		"nan horizon scale":    func(d *worldsky.Document) { d.Settings.HorizonUVScale = math.NaN() },
		"inf horizon scale":    func(d *worldsky.Document) { d.Settings.HorizonUVScale = math.Inf(1) },
		"infinite haze":        func(d *worldsky.Document) { d.Settings.HorizonHaze = math.Inf(1) },
		"large haze":           func(d *worldsky.Document) { d.Settings.HorizonHaze = 2 },
		"negative parallax":    func(d *worldsky.Document) { d.Settings.HorizonParallax = -1 },
		"large cloud parallax": func(d *worldsky.Document) { d.Settings.CloudParallax[0] = .1 },
		"large dense scale":    func(d *worldsky.Document) { d.Settings.CloudDenseScrollScale = 5 },
		"large dense offset":   func(d *worldsky.Document) { d.Settings.CloudDenseOffset[0] = 2 },
		"negative emission":    func(d *worldsky.Document) { d.Settings.HorizonEmission[0] = -.1 },
	} {
		t.Run(name, func(t *testing.T) {
			document := fixture(t)
			mutate(&document)
			if _, err := worldsky.Encode(document); err == nil {
				t.Fatal("invalid document encoded")
			}
		})
	}
	// Huge dimensions are rejected before multiplication on native 32-bit/Wasm.
	if _, err := (worldsky.Image{Width: math.MaxInt, Height: math.MaxInt, Packed: []byte{0}}).Unpack(
		nil,
	); !errors.Is(
		err,
		worldsky.ErrBounds,
	) {
		t.Fatalf("overflow dimensions: %v", err)
	}
}

func TestMalformedBinaryAndCompletePreflight(t *testing.T) {
	t.Parallel()
	encoded, err := worldsky.Encode(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func([]byte) []byte{
		"magic":                func(b []byte) []byte { b[0] = 0; return b },
		"wire version":         func(b []byte) []byte { b[4] = 2; return b },
		"semantic version":     func(b []byte) []byte { b[16] = 2; return b },
		"flags":                func(b []byte) []byte { b[6] = 1; return b },
		"reserved":             func(b []byte) []byte { b[12] = 1; return b },
		"truncation":           func(b []byte) []byte { return b[:len(b)-1] },
		"trailing bytes":       func(b []byte) []byte { return append(b, 0) },
		"negative zero":        func(b []byte) []byte { binary.LittleEndian.PutUint64(b[18:], 1<<63); return b },
		"invalid presence":     func(b []byte) []byte { b[122] = 2; return b },
		"packed size overflow": func(b []byte) []byte { binary.LittleEndian.PutUint32(b[131:], math.MaxUint32); return b },
		"last gzip checksum":   func(b []byte) []byte { b[len(b)-8] ^= 1; return b },
	} {
		t.Run(name, func(t *testing.T) {
			decoded, err := worldsky.Decode(mutate(bytes.Clone(encoded)))
			if err == nil || !reflect.DeepEqual(decoded, worldsky.Document{}) {
				t.Fatalf("malformed data published: %+v %v", decoded, err)
			}
		})
	}
	for _, b := range [][]byte{nil, make([]byte, worldsky.MaxEncodedSize+1)} {
		if _, err := worldsky.Decode(b); !errors.Is(err, worldsky.ErrBounds) {
			t.Fatalf("encoded bound: %v", err)
		}
	}
	// Every dimension is checked before any gzip stream is expanded: the corrupt
	// first stream cannot hide a bound violation in a later image.
	document := fixture(t)
	document.CloudSparse = &worldsky.Image{Width: 1, Height: 1, Packed: []byte{0}}
	document.Horizon = &worldsky.Image{Width: math.MaxInt, Height: 1, Packed: []byte{0}}
	if err := worldsky.Validate(&document); !errors.Is(err, worldsky.ErrBounds) {
		t.Fatalf("incomplete dimension preflight: %v", err)
	}
}

func TestLateFailureDoesNotCopyPackedImages(t *testing.T) {
	// Keep allocation accounting separate from parallel tests. A large valid
	// first image plus a corrupt final stream must retain only bounded scratch.
	pixels := make([]byte, 1024*1024)
	state := uint32(1)
	for index := range pixels {
		state = state*1664525 + 1013904223
		pixels[index] = byte(state >> 24)
	}
	document := worldsky.Document{Version: worldsky.Version, Settings: worldsky.DefaultSettings(),
		CloudSparse: &worldsky.Image{Width: 512, Height: 512, Packed: packed(t, pixels)},
		Horizon:     &worldsky.Image{Width: 1, Height: 1, Packed: packed(t, []byte{0, 0, 0, 0})}}
	encoded, err := worldsky.Encode(document)
	if err != nil {
		t.Fatal(err)
	}
	encoded[len(encoded)-8] ^= 1
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	decoded, err := worldsky.Decode(encoded)
	runtime.ReadMemStats(&after)
	if !errors.Is(err, worldsky.ErrImage) || !reflect.DeepEqual(decoded, worldsky.Document{}) {
		t.Fatalf("late failure published: %v", err)
	}
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated >= uint64(len(document.CloudSparse.Packed))/2 {
		t.Fatalf("late failure copied packed images: %d allocated bytes", allocated)
	}
}

func TestEncodedOverheadBoundPrecedesStreamValidation(t *testing.T) {
	t.Parallel()
	// These deliberately invalid gzip payloads establish that complete encoded
	// size, including fixed records, is checked before opening the first stream.
	payload := make([]byte, worldsky.MaxEncodedSize/2)
	const overhead = 16 + 2 + 13*8 + 4 + 2*12
	for _, test := range []struct {
		name string
		trim int
		want error
	}{
		{"packed payloads equal limit", 0, worldsky.ErrBounds},
		{"complete wire one byte over", overhead - 1, worldsky.ErrBounds},
		{"complete wire exactly fits", overhead, worldsky.ErrImage},
	} {
		t.Run(test.name, func(t *testing.T) {
			document := worldsky.Document{Version: worldsky.Version, Settings: worldsky.DefaultSettings(),
				CloudSparse: &worldsky.Image{Width: 1, Height: 1, Packed: payload},
				Horizon:     &worldsky.Image{Width: 1, Height: 1, Packed: payload[:len(payload)-test.trim]}}
			if err := worldsky.Validate(&document); !errors.Is(err, test.want) {
				t.Fatalf("complete encoded bound: got %v want %v", err, test.want)
			}
			if _, err := worldsky.Encode(document); !errors.Is(err, test.want) {
				t.Fatalf("encoder bound differs from validation: got %v want %v", err, test.want)
			}
		})
	}
}

func FuzzDecode(f *testing.F) {
	encoded, err := worldsky.Encode(worldsky.Document{Version: worldsky.Version, Settings: worldsky.DefaultSettings()})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(encoded)
	f.Fuzz(func(t *testing.T, input []byte) {
		document, err := worldsky.Decode(input)
		if err != nil {
			return
		}
		encoded, err := worldsky.Encode(document)
		if err != nil || !bytes.Equal(encoded, input) {
			t.Fatalf("noncanonical accepted input: %v", err)
		}
	})
}
