package level_test

import (
	"crypto/sha256"
	"testing"

	"github.com/karty-game/karty-sdk/format/level"
)

func TestPassiveModuleIdentityMatchesWholeFile(t *testing.T) {
	t.Parallel()
	for _, size := range []int{0, 127, 128, 16383, 16384, 65535, 65536, 1 << 20} {
		var entries []level.SourceEntry
		if size > 0 {
			entries = []level.SourceEntry{{Name: "map", Kind: level.EntryData, Data: make([]byte, size)}}
		}
		envelope, err := level.Encode(metadataBytes(t, level.Metadata{"schema": level.StringValue("test@1")}), entries)
		if err != nil {
			t.Fatal(err)
		}
		module, err := level.WrapModule(envelope)
		if err != nil {
			t.Fatal(err)
		}
		digest, length, err := level.PassiveModuleIdentity(envelope)
		if err != nil || digest != sha256.Sum256(module) || length != uint64(len(module)) {
			t.Fatalf("whole-file identity differs at payload length %d: %v", size, err)
		}
		decoded, err := level.DecodePassiveModule(module)
		if err != nil || len(decoded.Raw) != len(envelope) {
			t.Fatalf("canonical module rejected: %v", err)
		}
		if size == 0 {
			continue
		}
		module[len(module)-1] ^= 1
		changed, _, err := level.PassiveModuleIdentity(module[len(module)-len(envelope):])
		if err != nil || changed == digest {
			t.Fatalf("payload alteration was not included in identity: %v", err)
		}
	}
}

func TestPassiveModuleIdentityRejectsInvalidEnvelope(t *testing.T) {
	t.Parallel()
	for _, invalid := range [][]byte{nil, []byte("KTYL"), make([]byte, level.HeaderSize)} {
		digest, size, err := level.PassiveModuleIdentity(invalid)
		if err == nil || size != 0 || digest != [sha256.Size]byte{} {
			t.Fatal("invalid envelope returned an identity")
		}
	}
}
