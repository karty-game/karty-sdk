package actions

import (
	"bytes"
	"os"
	"testing"
)

func TestEditorSchemaConsumerSnapshots(t *testing.T) {
	for _, path := range []string{"../../../karty-engine/core/contracts/actions-v1.generated.json", "../../../karty-cli/internal/actionbuild/testdata/actions-v1.generated.json"} {
		snapshot, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(snapshot, Schema) {
			t.Fatalf("editor schema drift: %s", path)
		}
	}
}
