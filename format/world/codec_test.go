package world_test

import (
	"bytes"
	"errors"
	"math"
	"os"
	"slices"
	"testing"

	"github.com/karty-game/karty-sdk/format/cartridge"
	"github.com/karty-game/karty-sdk/format/level"
	"github.com/karty-game/karty-sdk/format/world"
)

func TestEncodeDecodeCanonicalWorld(t *testing.T) {
	t.Parallel()

	document := validWorld()
	encoded, err := world.Encode(document)
	if err != nil {
		t.Fatal(err)
	}

	golden, err := os.ReadFile("testdata/two-room.world.json")
	if err != nil {
		t.Fatal(err)
	}
	golden = bytes.TrimSuffix(golden, []byte{'\n'})
	if !bytes.Equal(encoded, golden) {
		t.Fatalf("wire format changed:\n got %s\nwant %s", encoded, golden)
	}

	decoded, err := world.Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.EqualFunc(decoded.Sectors, document.Sectors, func(left, right world.Sector) bool {
		return left.ID == right.ID && slices.Equal(left.Walls, right.Walls)
	}) || decoded.Contents[0] != document.Contents[0] {
		t.Fatalf("decoded document differs: %+v", decoded)
	}
}

func TestDecodeRejectsNonCanonicalAndMalformedPayloads(t *testing.T) {
	t.Parallel()

	encoded := encodeValid(t)
	for name, payload := range map[string][]byte{
		"empty":         nil,
		"trailing":      append(slices.Clone(encoded), '\n'),
		"leading space": append([]byte{' '}, encoded...),
		"unknown field": bytes.Replace(encoded, []byte(`"version":1`), []byte(`"unknown":0,"version":1`), 1),
		"wrong version": bytes.Replace(encoded, []byte(`"version":1`), []byte(`"version":3`), 1),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := world.Decode(payload); err == nil {
				t.Fatal("Decode() accepted invalid payload")
			}
		})
	}
}

func TestDecodeRejectsEveryTruncation(t *testing.T) {
	t.Parallel()

	encoded := encodeValid(t)
	for size := range len(encoded) {
		if _, err := world.Decode(encoded[:size]); err == nil {
			t.Fatalf("Decode() accepted truncation at %d", size)
		}
	}
}

func TestCompiledWorldFitsLevelEnvelope(t *testing.T) {
	t.Parallel()

	payload := encodeValid(t)
	envelope, err := level.Encode([]byte(`{"schema":"karty.level@1"}`), []level.SourceEntry{{
		Name: world.EntryName, Kind: level.EntryData, Data: payload,
	}})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := level.Decode(envelope)
	if err != nil {
		t.Fatal(err)
	}
	stored, total, found := decoded.Read(world.EntryName, 0, uint32(len(payload)))
	if !found || total != uint32(len(payload)) || !bytes.Equal(stored, payload) {
		t.Fatal("compiled world did not round-trip through the level envelope")
	}
}

func TestWorldCapabilityIsAcceptedByManifest(t *testing.T) {
	t.Parallel()

	_, err := cartridge.EncodeManifest(cartridge.Manifest{
		ProjectName: "world-test", Compiler: "test", Width: 320, Height: 180,
		Features: []string{world.Feature},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejectsInvalidCompiledWorlds(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		mutate func(*world.Document)
		want   error
	}{
		"version": {func(document *world.Document) { document.Version = world.Version + 1 }, world.ErrVersion},
		"duplicate sector": {func(document *world.Document) {
			document.Sectors[1].ID = document.Sectors[0].ID
		}, world.ErrIdentity},
		"nan plane": {func(document *world.Document) {
			document.Sectors[0].Floor.A = math.NaN()
		}, world.ErrGeometry},
		"broken boundary": {func(document *world.Document) {
			document.Sectors[0].Walls[0].End.X = 3
		}, world.ErrGeometry},
		"clockwise": {func(document *world.Document) {
			walls := document.Sectors[0].Walls
			for left, right := 0, len(walls)-1; left < right; left, right = left+1, right-1 {
				walls[left], walls[right] = walls[right], walls[left]
			}
			for index := range walls {
				walls[index].Start, walls[index].End = walls[index].End, walls[index].Start
			}
		}, world.ErrGeometry},
		"missing reverse": {func(document *world.Document) {
			document.Sectors[1].Walls[3].Portal = -1
		}, world.ErrPortal},
		"portal crossover": {func(document *world.Document) {
			document.Sectors[1].Floor = world.Plane{B: -.2, C: .5}
		}, world.ErrPortal},
		"content sector": {func(document *world.Document) {
			document.Contents[0].Sector = ^uint32(0)
		}, world.ErrContent},
		"content outside": {func(document *world.Document) {
			document.Contents[0].Position.X = 20
		}, world.ErrContent},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			document := validWorld()
			test.mutate(&document)
			if err := world.Validate(&document); !errors.Is(err, test.want) {
				t.Fatalf("Validate() error = %v, want %v", err, test.want)
			}
		})
	}
}

func FuzzDecode(f *testing.F) {
	f.Add(encodeValid(f))
	f.Add([]byte(`{"version":1}`))
	f.Add([]byte("not json"))
	f.Fuzz(func(t *testing.T, encoded []byte) {
		document, err := world.Decode(encoded)
		if err != nil {
			return
		}

		reencoded, err := world.Encode(document)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(encoded, reencoded) {
			t.Fatal("successful decode was not canonical")
		}
	})
}

func validWorld() world.Document {
	return world.Document{
		Version: world.LegacyVersion,
		Sectors: []world.Sector{
			{
				ID: "room-a/0", SourceRoom: "room-a",
				Walls: []world.Wall{
					{Start: world.Vec2{X: 0, Y: 0}, End: world.Vec2{X: 4, Y: 0}, Portal: -1, Material: 3, SourceEdge: "south"},
					{Start: world.Vec2{X: 4, Y: 0}, End: world.Vec2{X: 4, Y: 4}, Portal: 1, SourceEdge: "door"},
					{Start: world.Vec2{X: 4, Y: 4}, End: world.Vec2{X: 0, Y: 4}, Portal: -1, Material: 3, SourceEdge: "north"},
					{Start: world.Vec2{X: 0, Y: 4}, End: world.Vec2{X: 0, Y: 0}, Portal: -1, Material: 3, SourceEdge: "west"},
				},
				Floor: world.Plane{A: .01, B: .02}, Ceiling: world.Plane{A: -.01, C: 4},
				FloorMaterial: 1, CeilingMaterial: 2,
			},
			{
				ID: "room-b/0", SourceRoom: "room-b", Instance: "prefab-1",
				Walls: []world.Wall{
					{Start: world.Vec2{X: 4, Y: 0}, End: world.Vec2{X: 8, Y: 0}, Portal: -1, Material: 4, SourceEdge: "south"},
					{Start: world.Vec2{X: 8, Y: 0}, End: world.Vec2{X: 8, Y: 4}, Portal: -1, Material: 4, SourceEdge: "east"},
					{Start: world.Vec2{X: 8, Y: 4}, End: world.Vec2{X: 4, Y: 4}, Portal: -1, Material: 4, SourceEdge: "north"},
					{Start: world.Vec2{X: 4, Y: 4}, End: world.Vec2{X: 4, Y: 0}, Portal: 0, SourceEdge: "door"},
				},
				Floor: world.Plane{A: .01, B: .02, C: .2}, Ceiling: world.Plane{A: -.01, C: 4.2},
				FloorMaterial: 1, CeilingMaterial: 2,
			},
		},
		Contents: []world.Content{{
			ID: "prefab-1/crate", SourceID: "crate", Instance: "prefab-1", Kind: "prop", Sector: 1,
			Position: world.Vec3{X: 6, Y: 2, Z: 1},
		}},
	}
}

type testHelper interface {
	Helper()
	Fatal(args ...any)
}

func encodeValid(t testHelper) []byte {
	t.Helper()
	encoded, err := world.Encode(validWorld())
	if err != nil {
		t.Fatal(err)
	}

	return encoded
}
