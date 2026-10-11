package actions

import (
	"bytes"
	"encoding/binary"
	"math"
	"reflect"
	"testing"

	"github.com/karty-game/karty-sdk/format/level"
)

func actionFixture() Document {
	return Document{
		Version: Version,
		Sequences: []Sequence{
			{
				Name:     "enter",
				OnRepeat: "restart",
				Steps: []Step{
					{
						Condition: "door.ready",
						Args:      level.Metadata{"key": level.BoolValue(true)},
						Then: []Step{
							{
								Action: "door.open",
								Args: level.Metadata{
									"target": level.ObjectValue(level.Metadata{"actor": level.StringValue("tower/door")}),
									"speed":  level.FloatValue(2.5),
								},
								OnFailure: []Step{{WaitFrames: 1}},
							},
						},
						Else: []Step{{WaitFrames: 3}},
					},
				},
			},
		},
	}
}
func TestBinaryActionsRoundtripAndCompleteBatch(t *testing.T) {
	document := actionFixture()
	data, err := Encode(document)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(data)
	if err != nil || !reflect.DeepEqual(document, decoded) {
		t.Fatal(decoded, err)
	}
	if !bytes.HasPrefix(data, []byte("KTYA")) {
		t.Fatal("not binary")
	}
	for size := range data {
		if _, err := Decode(data[:size]); err == nil {
			t.Fatalf("accepted truncation %d", size)
		}
	}
	for _, invalid := range [][]byte{[]byte(`{"version":1,"sequences":[]}`), append(bytes.Clone(data), 0)} {
		if _, err := Decode(invalid); err == nil {
			t.Fatal("accepted JSON/trailing data")
		}
	}
	for _, mutate := range []func([]byte){func(d []byte) { d[4] = 1 }, func(d []byte) { d[6] = 1 }, func(d []byte) { binary.LittleEndian.PutUint32(d[16:20], math.MaxUint32) }, func(d []byte) { d[len(d)-1] = 255 }} {
		invalid := bytes.Clone(data)
		mutate(invalid)
		if _, err := Decode(invalid); err == nil {
			t.Fatal("accepted malformed count/last branch")
		}
	}
}
func TestBinaryActionsSemanticBounds(t *testing.T) {
	for _, value := range []level.Value{level.NullValue(), level.ArrayValue(nil), level.FloatValue(math.NaN()), level.ObjectValue(level.Metadata{"actor": level.StringValue(""), "extra": level.BoolValue(true)})} {
		document := actionFixture()
		document.Sequences[0].Steps[0].Args["key"] = value
		if _, err := Encode(document); err == nil {
			t.Fatal("accepted invalid argument")
		}
	}
	deep := []Step{{WaitFrames: 1}}
	for range MaxDepth {
		deep = []Step{{Condition: "ready", Args: level.Metadata{}, Then: deep}}
	}
	if _, err := Encode(Document{Version: Version, Sequences: []Sequence{{Name: "deep", Steps: deep}}}); err == nil {
		t.Fatal("accepted excess depth")
	}
}
func FuzzBinaryActionsDecode(f *testing.F) {
	data, _ := Encode(actionFixture())
	f.Add(data)
	f.Fuzz(func(t *testing.T, data []byte) {
		document, err := Decode(data)
		if err != nil {
			return
		}
		encoded, err := Encode(document)
		if err != nil || !bytes.Equal(data, encoded) {
			t.Fatal("noncanonical actions", err)
		}
	})
}
