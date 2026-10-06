package worldsource_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"os"
	"testing"

	"github.com/karty-game/karty-sdk/format/worldsource"
)

func TestValidateAcceptsConcaveRoomAndPrefabPort(t *testing.T) {
	t.Parallel()

	document := validSource()
	if err := worldsource.Validate(&document); err != nil {
		t.Fatal(err)
	}
}

func TestCanonicalSourceFixture(t *testing.T) {
	t.Parallel()

	encoded, err := os.ReadFile("testdata/concave-prefab.world.yaml")
	if err != nil {
		t.Fatal(err)
	}
	// The JSON-compatible YAML fixture keeps its editor directive outside the
	// format payload. Public formats intentionally do not depend on a YAML parser.
	header, encoded, found := bytes.Cut(encoded, []byte("\n"))
	if !found || string(header) != "# $schema: ../../../.karty/schemas/worldsource.base.schema.json" {
		t.Fatal("world fixture must reference its generated editor schema")
	}
	var document worldsource.Document
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		t.Fatal(err)
	}
	if err := worldsource.Validate(&document); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejectsInvalidSourceGraphs(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		mutate func(*worldsource.Document)
		want   error
	}{
		"version": {func(document *worldsource.Document) { document.Version++ }, worldsource.ErrVersion},
		"duplicate room": {func(document *worldsource.Document) {
			document.Rooms = append(document.Rooms, document.Rooms[0])
		}, worldsource.ErrIdentity},
		"broken boundary": {func(document *worldsource.Document) {
			document.Rooms[0].Boundary[0].End.X = 5
		}, worldsource.ErrGeometry},
		"clockwise": {func(document *worldsource.Document) {
			reverseBoundary(document.Rooms[0].Boundary)
		}, worldsource.ErrGeometry},
		"self intersection": {func(document *worldsource.Document) {
			document.Rooms[0].Boundary = bowTieBoundary()
		}, worldsource.ErrGeometry},
		"nan plane": {func(document *worldsource.Document) {
			document.Rooms[0].Floor.A = math.NaN()
		}, worldsource.ErrGeometry},
		"outside content": {func(document *worldsource.Document) {
			document.Rooms[0].Contents[0].Position.X = 5
			document.Rooms[0].Contents[0].Position.Y = 5
		}, worldsource.ErrContent},
		"unknown prefab": {func(document *worldsource.Document) {
			document.Instances[0].Prefab = "missing"
		}, worldsource.ErrReference},
		"unknown port": {func(document *worldsource.Document) {
			document.Connections[0].B.Port = "missing"
		}, worldsource.ErrReference},
		"reused endpoint": {func(document *worldsource.Document) {
			duplicate := document.Connections[0]
			duplicate.ID = "duplicate"
			document.Connections = append(document.Connections, duplicate)
		}, worldsource.ErrConnection},
		"connected exposed port": {func(document *worldsource.Document) {
			prefab := &document.Prefabs[0]
			prefab.Connections = []worldsource.Connection{{
				ID: "inside",
				A:  prefab.Ports[0].Endpoint,
				B:  worldsource.Endpoint{Room: "alcove", Edge: "east"},
			}}
		}, worldsource.ErrConnection},
		"prefab cycle": {func(document *worldsource.Document) {
			document.Prefabs[0].Instances = []worldsource.Instance{{ID: "recursive", Prefab: "alcove"}}
		}, worldsource.ErrPrefabCycle},
		"duplicate material override": {func(document *worldsource.Document) {
			document.Instances[0].Materials = []worldsource.MaterialOverride{
				{From: "stone", To: "moss"}, {From: "stone", To: "brick"},
			}
		}, worldsource.ErrIdentity},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			document := validSource()
			test.mutate(&document)
			if err := worldsource.Validate(&document); !errors.Is(err, test.want) {
				t.Fatalf("Validate() error = %v, want %v", err, test.want)
			}
		})
	}
}

func validSource() worldsource.Document {
	return worldsource.Document{
		Version: worldsource.Version,
		Rooms: []worldsource.Room{{
			ID: "hall",
			Boundary: []worldsource.Edge{
				{ID: "south", Start: point(0, 0), End: point(6, 0), Material: "stone"},
				{ID: "door", Start: point(6, 0), End: point(6, 2), Material: "stone"},
				{ID: "inset-south", Start: point(6, 2), End: point(2, 2), Material: "stone"},
				{ID: "inset-east", Start: point(2, 2), End: point(2, 6), Material: "stone"},
				{ID: "north", Start: point(2, 6), End: point(0, 6), Material: "stone"},
				{ID: "west", Start: point(0, 6), End: point(0, 0), Material: "stone"},
			},
			Floor: worldsource.Plane{A: .01}, Ceiling: worldsource.Plane{B: -.01, C: 4},
			FloorMaterial: "floor", CeilingMaterial: "ceiling",
			Contents: []worldsource.Content{{
				ID: "spawn", Kind: "spawn", Position: worldsource.Vec3{X: 1, Y: 1, Z: 1},
			}},
		}},
		Prefabs: []worldsource.Prefab{{
			ID: "alcove",
			Rooms: []worldsource.Room{{
				ID: "alcove",
				Boundary: []worldsource.Edge{
					{ID: "south", Start: point(0, 0), End: point(2, 0), Material: "stone"},
					{ID: "east", Start: point(2, 0), End: point(2, 2), Material: "stone"},
					{ID: "north", Start: point(2, 2), End: point(0, 2), Material: "stone"},
					{ID: "entrance", Start: point(0, 2), End: point(0, 0), Material: "stone"},
				},
				Floor: worldsource.Plane{}, Ceiling: worldsource.Plane{C: 4},
				FloorMaterial: "floor", CeilingMaterial: "ceiling",
			}},
			Ports: []worldsource.Port{{
				ID: "entrance", Endpoint: worldsource.Endpoint{Room: "alcove", Edge: "entrance"},
			}},
		}},
		Instances: []worldsource.Instance{{
			ID: "alcove-1", Prefab: "alcove",
			Transform: worldsource.Transform{Translation: worldsource.Vec3{X: 6}, Scale: 1},
		}},
		Connections: []worldsource.Connection{{
			ID: "hall-to-alcove",
			A:  worldsource.Endpoint{Room: "hall", Edge: "door"},
			B:  worldsource.Endpoint{Instance: "alcove-1", Port: "entrance"},
		}},
	}
}

func point(x, y float64) worldsource.Vec2 {
	return worldsource.Vec2{X: x, Y: y}
}

func reverseBoundary(edges []worldsource.Edge) {
	for left, right := 0, len(edges)-1; left < right; left, right = left+1, right-1 {
		edges[left], edges[right] = edges[right], edges[left]
	}
	for index := range edges {
		edges[index].Start, edges[index].End = edges[index].End, edges[index].Start
	}
}

func bowTieBoundary() []worldsource.Edge {
	return []worldsource.Edge{
		{ID: "a", Start: point(0, 0), End: point(2, 2), Material: "stone"},
		{ID: "b", Start: point(2, 2), End: point(0, 2), Material: "stone"},
		{ID: "c", Start: point(0, 2), End: point(2, 0), Material: "stone"},
		{ID: "d", Start: point(2, 0), End: point(0, 0), Material: "stone"},
	}
}

func TestValidateDirectedConnectionsAndVersionGate(t *testing.T) {
	document := validSource()
	document.Connections[0].Direction = worldsource.PortalAToB
	// An endpoint used as an incoming target may independently own an outgoing
	// link. This pair expresses reciprocal travel as two directed graph edges.
	document.Connections = append(document.Connections, worldsource.Connection{
		ID:        "alcove-to-hall",
		A:         worldsource.Endpoint{Instance: "alcove-1", Port: "entrance"},
		B:         worldsource.Endpoint{Room: "hall", Edge: "door"},
		Direction: worldsource.PortalAToB,
	})
	if err := worldsource.Validate(&document); err != nil {
		t.Fatalf("directed graph: %v", err)
	}

	document.Version = worldsource.ActorVersion
	if err := worldsource.Validate(&document); !errors.Is(err, worldsource.ErrVersion) {
		t.Fatalf("legacy directed connection error = %v", err)
	}
}
