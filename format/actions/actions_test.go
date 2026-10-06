package actions

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestActionsRoundTripAndBounds(t *testing.T) {
	encoded := []byte(
		`{"version":1,"sequences":[{"name":"enter","onRepeat":"restart","steps":[{"condition":"door.ready","args":{"key":true},"then":[{"action":"door.open","args":{"target":{"actor":"tower/door"},"speed":2.5},"onFailure":[{"waitFrames":1}]}],"else":[{"waitFrames":3}]}]}]}`,
	)
	document, err := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	roundtrip, err := Encode(document)
	if err != nil {
		t.Fatal(err)
	}
	again, err := Decode(roundtrip)
	if err != nil || again.Sequences[0].Steps[0].Then[0].Action != "door.open" {
		t.Fatal(again, err)
	}
	for _, invalid := range []string{`{"version":2,"sequences":[]}`, `{"version":1,"sequences":[],"extra":1}`, `{"version":1,"sequences":[{"name":"a","steps":[{"waitFrames":0}]}]}`, `{"version":1,"sequences":[{"name":"a","steps":[{"action":"open","waitFrames":1}]}]}`, `{"version":1,"sequences":[{"name":"a","steps":[{"action":"open","args":{"target":{"actor":"a","extra":1}}}]}]}`, `{"version":1,"sequences":[{"name":"a","steps":[]},{"name":"a","steps":[]}]}`, string(bytes.Repeat([]byte(" "), MaxBytes+1)), string([]byte{255})} {
		if _, err := Decode([]byte(invalid)); err == nil {
			t.Fatal("invalid document accepted", invalid)
		}
	}
	deep := `{"waitFrames":1}`
	for range MaxDepth + 1 {
		deep = `{"condition":"ready","args":{},"then":[` + deep + `]}`
	}
	if _, err := Decode([]byte(`{"version":1,"sequences":[{"name":"a","steps":[` + deep + `]}]}`)); err == nil {
		t.Fatal("deep branches accepted")
	}
}
func TestPublicSchemaSnapshots(t *testing.T) {
	// Sibling snapshots are optional developer artifacts; standalone public builds
	// neither need private source nor fetch it. When present, drift is a failure.
	for _, path := range []string{"../../../karty-engine/core/contracts/actions-v1.generated.json", "../../../karty-cli/internal/actionbuild/testdata/actions-v1.generated.json"} {
		snapshot, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(snapshot, Schema) {
			t.Fatalf("schema drift in %s; regenerate from format/actions/schema.json", path)
		}
	}
	if !strings.Contains(string(Schema), "onRepeat") || !strings.Contains(string(Schema), "waitFrames") {
		t.Fatal("incomplete public schema")
	}
}
func FuzzDecode(f *testing.F) {
	f.Add([]byte(`{"version":1,"sequences":[]}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		document, err := Decode(data)
		if err != nil {
			return
		}
		encoded, err := Encode(document)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Decode(encoded); err != nil {
			t.Fatal(err)
		}
	})
}

func TestCaseSensitiveFields(t *testing.T) {
	for _, source := range []string{
		`{"Version":1,"sequences":[]}`,
		`{"version":1,"Sequences":[]}`,
		`{"version":1,"sequences":[{"Name":"a","steps":[]}]}`,
		`{"version":1,"sequences":[{"name":"a","Steps":[]}]}`,
		`{"version":1,"sequences":[{"name":"a","onrepeat":"ignore","steps":[]}]}`,
	} {
		if _, err := Decode([]byte(source)); err == nil {
			t.Fatal("case-insensitive fields accepted", source)
		}
	}
}
