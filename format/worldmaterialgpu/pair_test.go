package worldmaterialgpu

import (
	"encoding/binary"
	"slices"
	"testing"

	"github.com/karty-game/karty-sdk/format/cartridge"
	"github.com/karty-game/karty-sdk/format/level"
	"github.com/karty-game/karty-sdk/format/worldmaterial"
)

func materialLayout(t *testing.T) worldmaterial.Layout {
	t.Helper()
	layout, err := worldmaterial.NewLayout([]uint32{7, 42})
	if err != nil {
		t.Fatal(err)
	}
	return layout
}

func TestDescriptorRequiresCompleteCanonicalOrder(t *testing.T) {
	layout := materialLayout(t)
	d, err := NewDescriptor(layout)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := EncodeDescriptor(d)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeDescriptor(encoded)
	if err != nil || decoded.Validate(layout) != nil {
		t.Fatal(decoded, err)
	}
	if _, err = DecodeDescriptor(append(encoded, 0)); err == nil {
		t.Fatal("trailing descriptor data accepted")
	}
	for _, mutate := range []func(*Descriptor){
		func(d *Descriptor) { d.LayoutSHA256[0] ^= 1 },
		func(d *Descriptor) { slices.Reverse(d.MaterialIDs) },
		func(d *Descriptor) { d.MaterialIDs = d.MaterialIDs[:1] },
		func(d *Descriptor) { d.MaterialIDs[1] = d.MaterialIDs[0] },
	} {
		value := d
		value.MaterialIDs = slices.Clone(d.MaterialIDs)
		mutate(&value)
		if value.Validate(layout) == nil {
			t.Fatal("invalid layout binding accepted", value)
		}
	}
}

// Structural-only containers use one Zstd RLE block per mip. Successful format
// preflight is not evidence of UASTC validity; runtime must still transcode them.
func structuralArray(srgb bool, layers uint32) []byte {
	const indexEnd = 80 + 9*24
	b := make([]byte, indexEnd+44)
	copy(b, []byte{0xab, 'K', 'T', 'X', ' ', '2', '0', 0xbb, 13, 10, 26, 10})
	u32 := func(o int, v uint32) { binary.LittleEndian.PutUint32(b[o:], v) }
	u32(16, 1)
	u32(20, 256)
	u32(24, 256)
	u32(32, layers)
	u32(36, 1)
	u32(40, 9)
	u32(44, 2)
	u32(48, indexEnd)
	u32(52, 44)
	u32(indexEnd, 44)
	u32(indexEnd+8, 40<<16|2)
	b[indexEnd+12], b[indexEnd+13], b[indexEnd+14] = 166, 1, 1
	if srgb {
		b[indexEnd+14] = 2
	}
	b[indexEnd+16], b[indexEnd+17], b[indexEnd+20] = 3, 3, 16
	b[indexEnd+30], b[indexEnd+31] = 127, 3
	u32(indexEnd+40, 0xffffffff)
	for i := 8; i >= 0; i-- {
		size := ((max(1, 256>>i) + 3) / 4) * ((max(1, 256>>i) + 3) / 4) * 16 * int(layers)
		frame := []byte{0x28, 0xb5, 0x2f, 0xfd, 0xa0, 0, 0, 0, 0}
		binary.LittleEndian.PutUint32(frame[5:], uint32(size))
		for remaining := size; remaining > 0; {
			count := min(remaining, 128<<10)
			remaining -= count
			header := count<<3 | 2
			if remaining == 0 {
				header |= 1
			}
			frame = append(frame, byte(header), byte(header>>8), byte(header>>16), 0)
		}
		o := 80 + i*24
		binary.LittleEndian.PutUint64(b[o:], uint64(len(b)))
		binary.LittleEndian.PutUint64(b[o+8:], uint64(len(frame)))
		binary.LittleEndian.PutUint64(b[o+16:], uint64(size))
		b = append(b, frame...) //nolint:makezero // The initialized container header precedes the appended mip payloads.
	}
	return b
}

func TestTrioRequiresMatchingCompleteArrays(t *testing.T) {
	layout := materialLayout(t)
	descriptor, err := NewDescriptor(layout)
	if err != nil {
		t.Fatal(err)
	}
	p := Pair{
		Descriptor: descriptor,
		Albedo:     structuralArray(true, 2),
		SSBump:     structuralArray(false, 2),
		Detail:     structuralArray(false, 2),
	}
	if err := p.Validate(layout); err != nil {
		t.Fatal(err)
	}
	for _, modify := range []func(*Pair){
		func(p *Pair) { p.Detail = nil },
		func(p *Pair) { p.SSBump = structuralArray(true, 2) },
		func(p *Pair) { p.Albedo = structuralArray(false, 2) },
		func(p *Pair) { p.Detail = structuralArray(false, 1) },
	} {
		value := p
		modify(&value)
		if value.Validate(layout) == nil {
			t.Fatal("invalid trio accepted")
		}
	}
}

func TestLocalDeclarationRequiresWholeTrioAndCartridgeFeature(t *testing.T) {
	entries := []level.Entry{{Name: worldmaterial.LayoutEntry, Kind: level.EntryData}}
	for _, name := range []string{DescriptorEntry, AlbedoEntry, SSBumpEntry, DetailEntry} {
		entries = append(entries, level.Entry{Name: name, Kind: level.EntryData})
	}
	metadata := level.Metadata{MetadataKey: level.StringValue(Feature)}
	features := []string{cartridge.FeatureWorldMaterialAtlasV1, Feature, cartridge.FeatureWorldSectorsV1}
	if err := ValidateDeclaration(entries, metadata, features); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDeclaration(nil, nil, features); err != nil {
		t.Fatal("global capability forced arrays on unrelated level", err)
	}
	if ValidateDeclaration(entries, nil, features) == nil || ValidateDeclaration(entries, metadata, nil) == nil ||
		ValidateDeclaration(entries[:3], metadata, features) == nil || ValidateDeclaration(nil, metadata, features) == nil {
		t.Fatal("orphan/partial declaration accepted")
	}
	wrong := slices.Clone(entries)
	wrong[1].Kind = level.EntryTexture
	if ValidateDeclaration(wrong, metadata, features) == nil {
		t.Fatal("wrong entry kind accepted")
	}
}
