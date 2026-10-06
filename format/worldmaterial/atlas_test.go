package worldmaterial_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/karty-game/karty-sdk/codec/qoi"
	"github.com/karty-game/karty-sdk/format/asset"
	"github.com/karty-game/karty-sdk/format/cartridge"
	"github.com/karty-game/karty-sdk/format/level"
	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldmaterial"
)

func TestLayoutCanonicalRoundTripAndBounds(t *testing.T) {
	for count := 1; count <= worldmaterial.MaxMaterials; count++ {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			ids := make([]uint32, count)
			for i := range ids {
				ids[i] = ^uint32(0) - uint32(i)
			}
			layout, err := worldmaterial.NewLayout(ids)
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := worldmaterial.EncodeLayout(layout)
			if err != nil || len(encoded) > worldmaterial.MaxLayoutBytes {
				t.Fatalf("encode: %v, size %d", err, len(encoded))
			}
			decoded, err := worldmaterial.DecodeLayout(encoded)
			if err != nil || !reflect.DeepEqual(decoded, layout) {
				t.Fatalf("round trip: %v, %+v", err, decoded)
			}
			pixels := uint64(layout.Width) * uint64(layout.Height)
			if pixels > asset.MaxTexturePixels || pixels*4 > asset.MaxDecodedTextureBytes || pixels*8 > asset.MaxDecodedTextures {
				t.Fatal("layout exceeds texture/pair budget")
			}
			for _, r := range layout.Materials {
				if r.X-r.Gutter < 0 || r.Y-r.Gutter < 0 || r.X+r.Width+r.Gutter > layout.Width || r.Y+r.Height+r.Gutter > layout.Height {
					t.Fatal("gutter escapes atlas")
				}
			}
		})
	}
	ids := []uint32{99, 0, 7, ^uint32(0)}
	l, err := worldmaterial.NewLayout(ids)
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range l.Materials {
		if r.MaterialID != ids[i] {
			t.Fatal("material first-use order changed")
		}
	}
	for _, ids := range [][]uint32{nil, {}, {0, 0}, {7, 7}, make([]uint32, worldmaterial.MaxMaterials+1)} {
		if _, err := worldmaterial.NewLayout(ids); !errors.Is(err, worldmaterial.ErrAtlas) {
			t.Fatalf("accepted invalid IDs: %v", err)
		}
	}
}

func TestLayoutDimensionsAndOwnership(t *testing.T) {
	for _, size := range []struct{ count, width, height int }{
		{1, 352, 352}, {14, 4096, 352}, {15, 4096, 640}, {196, 4096, 4096},
	} {
		ids := make([]uint32, size.count)
		for i := range ids {
			ids[i] = uint32(i)
		}
		layout, err := worldmaterial.NewLayout(ids)
		if err != nil || layout.Width != size.width || layout.Height != size.height {
			t.Fatalf("count %d: layout %+v, error %v", size.count, layout, err)
		}
		ids[0] = 42
		if layout.Materials[0].MaterialID != 0 {
			t.Fatal("layout aliases caller-owned IDs")
		}
	}
}

func TestLayoutRejectsMalformedPlacements(t *testing.T) {
	mutations := map[string]func(*worldmaterial.Layout){
		"schema":               func(l *worldmaterial.Layout) { l.Schema = "karty.world-material-atlas@2" },
		"width":                func(l *worldmaterial.Layout) { l.Width++ },
		"height":               func(l *worldmaterial.Layout) { l.Height = -1 },
		"empty":                func(l *worldmaterial.Layout) { l.Materials = nil },
		"too many":             func(l *worldmaterial.Layout) { l.Materials = make([]worldmaterial.Rect, worldmaterial.MaxMaterials+1) },
		"duplicate ID":         func(l *worldmaterial.Layout) { l.Materials[1].MaterialID = l.Materials[0].MaterialID },
		"overlap":              func(l *worldmaterial.Layout) { l.Materials[1].X = l.Materials[0].X },
		"negative coordinate":  func(l *worldmaterial.Layout) { l.Materials[0].X = -1 },
		"overflow coordinate":  func(l *worldmaterial.Layout) { l.Materials[0].Y = int(^uint(0) >> 1) },
		"tile width":           func(l *worldmaterial.Layout) { l.Materials[0].Width++ },
		"tile height":          func(l *worldmaterial.Layout) { l.Materials[0].Height = 0 },
		"gutter":               func(l *worldmaterial.Layout) { l.Materials[0].Gutter = -1 },
		"reordered placements": func(l *worldmaterial.Layout) { l.Materials[0], l.Materials[1] = l.Materials[1], l.Materials[0] },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			l, _ := worldmaterial.NewLayout([]uint32{0, 42})
			mutate(&l)
			if !errors.Is(l.Validate(), worldmaterial.ErrAtlas) {
				t.Fatal("accepted layout")
			}
			if _, err := worldmaterial.EncodeLayout(l); !errors.Is(err, worldmaterial.ErrAtlas) {
				t.Fatal("encoded invalid layout")
			}
			encoded, _ := json.Marshal(l)
			if _, err := worldmaterial.DecodeLayout(encoded); !errors.Is(err, worldmaterial.ErrAtlas) {
				t.Fatal("decoded invalid layout")
			}
		})
	}
}

func TestLayoutJSONIsFrozen(t *testing.T) {
	l, _ := worldmaterial.NewLayout([]uint32{0})
	encoded, _ := worldmaterial.EncodeLayout(l)
	want := `{"schema":"karty.world-material-atlas@1","width":352,"height":352,"materials":[{"materialId":0,"x":48,"y":48,"width":256,"height":256,"gutter":16}]}`
	if string(encoded) != want {
		t.Fatalf("contract changed: %s", encoded)
	}
	for name, malformed := range map[string][]byte{
		"empty":                      nil,
		"oversize":                   bytes.Repeat([]byte(" "), worldmaterial.MaxLayoutBytes+1),
		"truncated":                  encoded[:len(encoded)-1],
		"trailing":                   append(bytes.Clone(encoded), []byte(`{}`)...),
		"whitespace":                 append(bytes.Clone(encoded), ' '),
		"unknown path":               []byte(strings.Replace(want, `"width":352`, `"path":"../../atlas.qoi","width":352`, 1)),
		"unknown placement texture":  []byte(strings.Replace(want, `"materialId":0`, `"materialId":0,"texture":"@texture/00000001"`, 1)),
		"unversioned schema":         []byte(strings.Replace(want, worldmaterial.Schema, "karty.world-material-atlas", 1)),
		"future schema":              []byte(strings.Replace(want, worldmaterial.Schema, "karty.world-material-atlas@2", 1)),
		"duplicate top field":        []byte(strings.Replace(want, `"width":352`, `"width":1,"width":352`, 1)),
		"duplicate placement field":  []byte(strings.Replace(want, `"x":48`, `"x":1,"x":48`, 1)),
		"missing zero ID":            []byte(strings.Replace(want, `"materialId":0,`, "", 1)),
		"missing schema":             []byte(strings.Replace(want, `"schema":"karty.world-material-atlas@1",`, "", 1)),
		"null layout":                []byte("null"),
		"null placement":             []byte(strings.Replace(want, want[strings.Index(want, "[{")+1:len(want)-2], "null", 1)),
		"alternate number":           []byte(strings.Replace(want, `"width":352`, `"width":352.0`, 1)),
		"exponent number":            []byte(strings.Replace(want, `"width":352`, `"width":352e0`, 1)),
		"negative ID":                []byte(strings.Replace(want, `"materialId":0`, `"materialId":-1`, 1)),
		"overflow ID":                []byte(strings.Replace(want, `"materialId":0`, `"materialId":4294967296`, 1)),
		"overflow coordinate":        []byte(strings.Replace(want, `"x":48`, `"x":18446744073709551616`, 1)),
		"null ID":                    []byte(strings.Replace(want, `"materialId":0`, `"materialId":null`, 1)),
		"string ID":                  []byte(strings.Replace(want, `"materialId":0`, `"materialId":"0"`, 1)),
		"reordered top fields":       []byte(strings.Replace(want, `"width":352,"height":352`, `"height":352,"width":352`, 1)),
		"reordered placement fields": []byte(strings.Replace(want, `"x":48,"y":48`, `"y":48,"x":48`, 1)),
		"case-folded field":          []byte(strings.Replace(want, `"materialId"`, `"MaterialId"`, 1)),
		"escaped schema":             []byte(strings.Replace(want, `"karty.`, `"\u006barty.`, 1)),
		"leading whitespace":         append([]byte{' '}, encoded...),
		"UTF-8 BOM":                  append([]byte{0xef, 0xbb, 0xbf}, encoded...),
		"null placements":            []byte(strings.Replace(want, string(encoded[bytes.IndexByte(encoded, '['):len(encoded)-1]), "null", 1)),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := worldmaterial.DecodeLayout(malformed); !errors.Is(err, worldmaterial.ErrAtlas) {
				t.Fatal("accepted noncanonical JSON")
			}
		})
	}
	for size := range len(encoded) {
		if _, err := worldmaterial.DecodeLayout(encoded[:size]); !errors.Is(err, worldmaterial.ErrAtlas) {
			t.Fatalf("accepted truncation at %d", size)
		}
	}
}

func testPair(t *testing.T, ids ...uint32) worldmaterial.Pair {
	t.Helper()
	if len(ids) == 0 {
		ids = []uint32{0}
	}
	l, err := worldmaterial.NewLayout(ids)
	if err != nil {
		t.Fatal(err)
	}
	pixels := image.NewNRGBA(image.Rect(0, 0, l.Width, l.Height))
	for o := 0; o < len(pixels.Pix); o += 4 {
		pixels.Pix[o], pixels.Pix[o+1], pixels.Pix[o+2], pixels.Pix[o+3] = 192, 192, 192, 255
	}
	albedo, _, err := qoi.Encode(pixels, qoi.Options{Channels: qoi.ChannelsRGBA, Colorspace: qoi.ColorspaceSRGB})
	if err != nil {
		t.Fatal(err)
	}
	for o := 0; o < len(pixels.Pix); o += 4 {
		pixels.Pix[o], pixels.Pix[o+1], pixels.Pix[o+2], pixels.Pix[o+3] = 128, 128, 73, 17
	}
	data, _, err := qoi.Encode(pixels, qoi.Options{Channels: qoi.ChannelsRGBA, Colorspace: qoi.ColorspaceLinear})
	if err != nil {
		t.Fatal(err)
	}
	return worldmaterial.Pair{Layout: l, Albedo: albedo, Data: data}
}

func TestPairValidation(t *testing.T) {
	pair := testPair(t)
	if err := pair.Validate(); err != nil {
		t.Fatal(err)
	}
	_, pixels, err := qoi.Decode(pair.Data)
	if err != nil || pixels.Pix[0] != 128 || pixels.Pix[1] != 128 || pixels.Pix[2] != 73 || pixels.Pix[3] != 17 {
		t.Fatal("straight data channels changed")
	}
	mutations := map[string]func(*worldmaterial.Pair){
		"invalid layout":          func(p *worldmaterial.Pair) { p.Layout.Schema = "unknown" },
		"missing albedo":          func(p *worldmaterial.Pair) { p.Albedo = nil },
		"missing data":            func(p *worldmaterial.Pair) { p.Data = nil },
		"swapped pair":            func(p *worldmaterial.Pair) { p.Albedo, p.Data = p.Data, p.Albedo },
		"albedo dimension":        func(p *worldmaterial.Pair) { binary.BigEndian.PutUint32(p.Albedo[4:8], 1) },
		"data dimension":          func(p *worldmaterial.Pair) { binary.BigEndian.PutUint32(p.Data[8:12], 1) },
		"oversize dimension":      func(p *worldmaterial.Pair) { binary.BigEndian.PutUint32(p.Data[4:8], ^uint32(0)) },
		"RGB albedo":              func(p *worldmaterial.Pair) { p.Albedo[12] = qoi.ChannelsRGB },
		"RGB data":                func(p *worldmaterial.Pair) { p.Data[12] = qoi.ChannelsRGB },
		"linear albedo":           func(p *worldmaterial.Pair) { p.Albedo[13] = qoi.ColorspaceLinear },
		"sRGB data":               func(p *worldmaterial.Pair) { p.Data[13] = qoi.ColorspaceSRGB },
		"broken albedo marker":    func(p *worldmaterial.Pair) { p.Albedo[len(p.Albedo)-1] = 0 },
		"broken data marker":      func(p *worldmaterial.Pair) { p.Data[len(p.Data)-1] = 0 },
		"truncated albedo stream": func(p *worldmaterial.Pair) { p.Albedo = append(p.Albedo[:14:14], p.Albedo[len(p.Albedo)-8:]...) },
		"truncated data stream":   func(p *worldmaterial.Pair) { p.Data = append(p.Data[:14:14], p.Data[len(p.Data)-8:]...) },
		"trailing albedo operation": func(p *worldmaterial.Pair) {
			p.Albedo = append(append(bytes.Clone(p.Albedo[:len(p.Albedo)-8]), 0xc0), p.Albedo[len(p.Albedo)-8:]...)
		},
		"trailing data operation": func(p *worldmaterial.Pair) {
			p.Data = append(append(bytes.Clone(p.Data[:len(p.Data)-8]), 0xc0), p.Data[len(p.Data)-8:]...)
		},
		"albedo run overflow": func(p *worldmaterial.Pair) { p.Albedo[len(p.Albedo)-9] = 0xfd },
		"data run overflow":   func(p *worldmaterial.Pair) { p.Data[len(p.Data)-9] = 0xfd },
		"truncated RGBA operation": func(p *worldmaterial.Pair) {
			p.Data = append(append(bytes.Clone(p.Data[:14]), 0xff, 128, 128, 73), p.Data[len(p.Data)-8:]...)
		},
		"transparent albedo": func(p *worldmaterial.Pair) {
			_, pixels, err := qoi.Decode(p.Albedo)
			if err != nil {
				t.Fatal(err)
			}
			pixels.Pix[3] = 254
			p.Albedo, _, err = qoi.Encode(pixels, qoi.Options{Channels: qoi.ChannelsRGBA, Colorspace: qoi.ColorspaceSRGB})
			if err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			p := pair
			p.Layout.Materials = append([]worldmaterial.Rect(nil), p.Layout.Materials...)
			p.Albedo, p.Data = bytes.Clone(p.Albedo), bytes.Clone(p.Data)
			mutate(&p)
			if !errors.Is(p.Validate(), worldmaterial.ErrAtlas) {
				t.Fatal("accepted invalid pair")
			}
		})
	}
}

func TestPairDataPreservesStraightChannelsAndOwnership(t *testing.T) {
	pair := testPair(t)
	_, pixels, err := qoi.Decode(pair.Data)
	if err != nil {
		t.Fatal(err)
	}
	// Include every AO byte, including zero, with RGB greater than alpha.
	for ao := 0; ao <= 255; ao++ {
		copy(pixels.Pix[ao*4:], []byte{128, 255, 73, byte(ao)})
	}
	pair.Data, _, err = qoi.Encode(pixels, qoi.Options{Channels: qoi.ChannelsRGBA, Colorspace: qoi.ColorspaceLinear})
	if err != nil {
		t.Fatal(err)
	}
	albedoBefore, dataBefore := bytes.Clone(pair.Albedo), bytes.Clone(pair.Data)
	layoutBefore, err := worldmaterial.EncodeLayout(pair.Layout)
	if err != nil {
		t.Fatal(err)
	}
	if err := pair.Validate(); err != nil {
		t.Fatal(err)
	}
	_, decoded, err := qoi.Decode(pair.Data)
	if err != nil || !bytes.Equal(decoded.Pix, pixels.Pix) {
		t.Fatal("straight material channels changed")
	}
	layoutAfter, err := worldmaterial.EncodeLayout(pair.Layout)
	if err != nil || !bytes.Equal(layoutBefore, layoutAfter) || !bytes.Equal(albedoBefore, pair.Albedo) ||
		!bytes.Equal(dataBefore, pair.Data) {
		t.Fatal("validation modified caller-owned pair")
	}
}

func TestPairLevelEnvelopeIntegration(t *testing.T) {
	ids := []uint32{99, 7, ^uint32(0), 0}
	p := testPair(t, ids...)
	layout, _ := worldmaterial.EncodeLayout(p.Layout)
	metadata, _ := json.Marshal(map[string]string{worldmaterial.MetadataKey: worldmaterial.Schema})
	compiled, err := world.Encode(world.Document{
		Version: world.Version,
		Sectors: []world.Sector{{
			ID: "room/0", SourceRoom: "room", Ceiling: world.Plane{C: 4},
			FloorMaterial: 99, CeilingMaterial: 7,
			Walls: []world.Wall{
				{Start: world.Vec2{X: 0, Y: 0}, End: world.Vec2{X: 4, Y: 0}, Portal: -1, Material: ^uint32(0)},
				{Start: world.Vec2{X: 4, Y: 0}, End: world.Vec2{X: 4, Y: 4}, Portal: -1},
				{Start: world.Vec2{X: 4, Y: 4}, End: world.Vec2{X: 0, Y: 4}, Portal: -1, Material: 7},
				{Start: world.Vec2{X: 0, Y: 4}, End: world.Vec2{X: 0, Y: 0}, Portal: -1, Material: ^uint32(0)},
			},
		}},
		Contents: []world.Content{},
	})
	if err != nil {
		t.Fatal(err)
	}
	entries := []level.SourceEntry{
		{Name: world.EntryName, Kind: level.EntryData, Data: compiled},
		{Name: worldmaterial.LayoutEntry, Kind: level.EntryData, Data: layout},
		{Name: worldmaterial.AlbedoEntry, Kind: level.EntryData, Data: p.Albedo},
		{Name: worldmaterial.DataEntry, Kind: level.EntryData, Data: p.Data},
	}
	sourceTexture := solidQOI(1, 1, [4]byte{192, 192, 192, 255}, qoi.ColorspaceSRGB)
	for _, id := range ids {
		if id != 0 {
			entries = append(entries, level.SourceEntry{
				Name: level.TextureEntryName(id), Kind: level.EntryTexture, Data: sourceTexture,
			})
		}
	}
	encoded, err := level.Encode(metadata, entries)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := level.Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(envelope.Entries) != len(entries) || !bytes.Equal(envelope.Metadata, metadata) {
		t.Fatal("level entry or metadata contract changed")
	}
	for _, entry := range envelope.Entries {
		if entry.Kind == level.EntryTexture {
			id, ok := level.TextureAssetID(entry.Name)
			if !ok || id == 0 {
				t.Fatal("source texture identity changed")
			}
		} else if entry.Kind != level.EntryData {
			t.Fatal("world or atlas entry is not level data")
		}
	}
	read := func(name string) []byte {
		for _, entry := range envelope.Entries {
			if entry.Name == name {
				b, _, ok := envelope.Read(name, 0, entry.Length)
				if ok {
					return b
				}
			}
		}
		t.Fatalf("missing entry %s", name)
		return nil
	}
	if stored := read(world.EntryName); !bytes.Equal(stored, compiled) {
		t.Fatal("compiled world bytes changed")
	} else if _, err := world.Decode(stored); err != nil {
		t.Fatal(err)
	}
	decoded, err := worldmaterial.DecodeLayout(read(worldmaterial.LayoutEntry))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, p.Layout) {
		t.Fatal("packaged placements or material identities changed")
	}
	for slot, id := range ids {
		if decoded.Materials[slot].MaterialID != id {
			t.Fatal("material ID changed to a dense slot or numeric sort order")
		}
		if id != 0 && !bytes.Equal(read(level.TextureEntryName(id)), sourceTexture) {
			t.Fatal("material/source texture reference changed")
		}
	}
	loaded := worldmaterial.Pair{Layout: decoded, Albedo: read(worldmaterial.AlbedoEntry), Data: read(worldmaterial.DataEntry)}
	if err := loaded.Validate(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(loaded.Albedo, p.Albedo) || !bytes.Equal(loaded.Data, p.Data) {
		t.Fatal("packaged QOI bytes changed")
	}
	for _, feature := range []string{cartridge.FeatureWorldMaterialAtlasV1, string(asset.CapabilityWorldMaterialAtlasV1)} {
		if feature != worldmaterial.Feature {
			t.Fatal("capability declarations disagree")
		}
	}
	module, err := level.WrapModule(encoded)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(module)
	manifest := cartridge.Manifest{
		ProjectName: "atlas", Compiler: "fixture", Width: 640, Height: 360,
		Features: []string{cartridge.FeatureTextureQOIv1, worldmaterial.Feature, world.Feature},
		Levels: []cartridge.LevelDependency{{
			Name: "atlas", Kind: "world", ContentSHA256: hex.EncodeToString(digest[:]),
			Size: uint64(len(module)), EnvelopeVersion: level.EnvelopeVersion,
		}},
	}
	manifestBytes, err := cartridge.EncodeManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	game, err := cartridge.EmbedSection([]byte("\x00asm\x01\x00\x00\x00"), cartridge.ManifestSectionName, manifestBytes)
	if err != nil {
		t.Fatal(err)
	}
	storedManifest, err := cartridge.ExtractSection(game, cartridge.ManifestSectionName)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := cartridge.DecodeManifest(storedManifest); err != nil || !reflect.DeepEqual(got, manifest) {
		t.Fatalf("atlas cartridge reference changed: %+v, %v", got, err)
	}
}

// Encode a uniform reference QOI directly so the maximum-boundary test does not
// need an additional 64 MiB source image or an encoder/decoder round trip.
//
//nolint:makezero // The initialized QOI header precedes the appended pixel operations.
func solidQOI(width, height int, pixel [4]byte, colorspace uint8) []byte {
	encoded := make([]byte, 14)
	copy(encoded, qoi.Magic)
	binary.BigEndian.PutUint32(encoded[4:8], uint32(width))
	binary.BigEndian.PutUint32(encoded[8:12], uint32(height))
	encoded[12], encoded[13] = qoi.ChannelsRGBA, colorspace
	encoded = append(encoded, 0xff, pixel[0], pixel[1], pixel[2], pixel[3])
	for remaining := width*height - 1; remaining > 0; {
		run := min(remaining, 62)
		encoded = append(encoded, 0xc0|byte(run-1))
		remaining -= run
	}
	return append(encoded, 0, 0, 0, 0, 0, 0, 0, 1)
}

func TestPairMaximumDimensionsAndHeaderPreflight(t *testing.T) {
	ids := make([]uint32, worldmaterial.MaxMaterials)
	for i := range ids {
		ids[i] = uint32(i)
	}
	layout, err := worldmaterial.NewLayout(ids)
	if err != nil {
		t.Fatal(err)
	}
	pair := worldmaterial.Pair{
		Layout: layout,
		Albedo: solidQOI(layout.Width, layout.Height, [4]byte{192, 192, 192, 255}, qoi.ColorspaceSRGB),
		Data:   solidQOI(layout.Width, layout.Height, [4]byte{128, 128, 73, 0}, qoi.ColorspaceLinear),
	}
	var decodedBytes uint64
	for _, encoded := range [][]byte{pair.Albedo, pair.Data} {
		m, err := qoi.Inspect(encoded)
		if err != nil || m.DecodedBytes != asset.MaxDecodedTextureBytes {
			t.Fatalf("maximum texture cost: %+v, %v", m, err)
		}
		decodedBytes += m.DecodedBytes
	}
	if decodedBytes != 128*1024*1024 || decodedBytes > asset.MaxDecodedTextures {
		t.Fatalf("maximum pair cost: %d", decodedBytes)
	}
	if err := pair.Validate(); err != nil {
		t.Fatalf("maximum pair rejected: %v", err)
	}

	// Reject the second header before allocating pixels for the valid first
	// texture. Compare against layout-only work, allowing its bounded ID map.
	for name, mutate := range map[string]func([]byte){
		"wrong size": func(b []byte) { binary.BigEndian.PutUint32(b[4:8], uint32(layout.Width-1)) },
		"overflow":   func(b []byte) { binary.BigEndian.PutUint32(b[4:8], ^uint32(0)) },
		"channels":   func(b []byte) { b[12] = qoi.ChannelsRGB },
		"colorspace": func(b []byte) { b[13] = qoi.ColorspaceSRGB },
	} {
		t.Run(name, func(t *testing.T) {
			invalid := pair
			invalid.Data = bytes.Clone(pair.Data)
			mutate(invalid.Data)
			if !errors.Is(invalid.Validate(), worldmaterial.ErrAtlas) {
				t.Fatal("invalid second header accepted")
			}
			baseline := testing.AllocsPerRun(3, func() { _ = layout.Validate() })
			if got := testing.AllocsPerRun(3, func() { _ = invalid.Validate() }); got > baseline {
				t.Fatalf("header rejection allocated pixels: %g allocations, layout baseline %g", got, baseline)
			}
		})
	}
}

func TestPairAlbedoOpacityAcrossAtlasRegions(t *testing.T) {
	ids := make([]uint32, worldmaterial.GridSize+1)
	for i := range ids {
		ids[i] = uint32(i)
	}
	layout, err := worldmaterial.NewLayout(ids)
	if err != nil {
		t.Fatal(err)
	}
	pair := worldmaterial.Pair{
		Layout: layout,
		Albedo: solidQOI(layout.Width, layout.Height, [4]byte{192, 192, 192, 255}, qoi.ColorspaceSRGB),
		Data:   solidQOI(layout.Width, layout.Height, [4]byte{128, 128, 73, 0}, qoi.ColorspaceLinear),
	}
	_, pixels, err := qoi.Decode(pair.Albedo)
	if err != nil {
		t.Fatal(err)
	}
	r := layout.Materials[0]
	for name, point := range map[string]image.Point{
		"interior":    {X: r.X, Y: r.Y},
		"gutter":      {X: r.X - r.Gutter, Y: r.Y},
		"corner":      {X: r.X + r.Width + r.Gutter - 1, Y: r.Y + r.Height + r.Gutter - 1},
		"margin":      {},
		"unused slot": {X: r.X + worldmaterial.CellSize, Y: r.Y + worldmaterial.CellSize},
		"last pixel":  {X: layout.Width - 1, Y: layout.Height - 1},
	} {
		t.Run(name, func(t *testing.T) {
			offset := pixels.PixOffset(point.X, point.Y) + 3
			pixels.Pix[offset] = 254
			invalid := pair
			invalid.Albedo, _, err = qoi.Encode(pixels, qoi.Options{Channels: qoi.ChannelsRGBA, Colorspace: qoi.ColorspaceSRGB})
			pixels.Pix[offset] = 255
			if err != nil {
				t.Fatal(err)
			}
			if !errors.Is(invalid.Validate(), worldmaterial.ErrAtlas) {
				t.Fatal("nonopaque atlas region accepted")
			}
		})
	}
}

func TestAtlasEntriesRequireDataKind(t *testing.T) {
	for _, name := range []string{worldmaterial.LayoutEntry, worldmaterial.AlbedoEntry, worldmaterial.DataEntry} {
		t.Run(name, func(t *testing.T) {
			entry := level.SourceEntry{Name: name, Kind: level.EntryTexture, Data: []byte{1}}
			if _, err := level.Encode([]byte(`{}`), []level.SourceEntry{entry}); !errors.Is(err, level.ErrEntry) {
				t.Fatalf("Encode() error = %v, want ErrEntry", err)
			}
			entry.Kind = level.EntryData
			encoded, err := level.Encode([]byte(`{}`), []level.SourceEntry{entry})
			if err != nil {
				t.Fatal(err)
			}
			encoded[level.HeaderSize+2] = byte(level.EntryTexture)
			if _, err := level.Decode(encoded); !errors.Is(err, level.ErrEntry) {
				t.Fatalf("Decode() error = %v, want ErrEntry", err)
			}
		})
	}
}

func FuzzDecodeLayout(f *testing.F) {
	l, _ := worldmaterial.NewLayout([]uint32{0, 42})
	encoded, _ := worldmaterial.EncodeLayout(l)
	f.Add(encoded)
	f.Add([]byte(`{}`))
	f.Fuzz(func(t *testing.T, input []byte) {
		l, err := worldmaterial.DecodeLayout(input)
		if err != nil {
			return
		}
		encoded, err := worldmaterial.EncodeLayout(l)
		if err != nil || !bytes.Equal(encoded, input) {
			t.Fatal("accepted noncanonical layout")
		}
	})
}
