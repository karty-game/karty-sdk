package worldgpuchunk

import (
	"testing"

	"github.com/karty-game/karty-sdk/format/cartridge"
	"github.com/karty-game/karty-sdk/format/level"
)

func TestDeclarationRequiresCompleteRootOrCell(t *testing.T) {
	t.Parallel()
	features := []string{
		Feature,
		cartridge.FeatureWorldMaterialGPUV1,
		cartridge.FeatureWorldChunksV1,
		cartridge.FeatureWorldFixedLightingV1,
		cartridge.FeatureWorldLightmapsV1,
		cartridge.FeatureWorldSectorsV1,
	}
	for _, root := range []bool{false, true} {
		entries := []level.Entry{{Name: PayloadEntry, Kind: level.EntryData}, {Name: RNMEntry, Kind: level.EntryData}}
		key := CellMetadataKey
		if root {
			key = MetadataKey
			entries = []level.Entry{{Name: CatalogEntry, Kind: level.EntryData}}
		}
		metadata := level.Metadata{key: level.StringValue(Schema)}
		if err := ValidateDeclaration(entries, metadata, features); err != nil {
			t.Fatal(err)
		}
		if ValidateDeclaration(entries, nil, features) == nil {
			t.Fatal("orphan reserved data admitted")
		}
		if ValidateDeclaration(entries, metadata, nil) == nil {
			t.Fatal("undeclared feature admitted")
		}
		entries = append(entries, entries[0])
		if ValidateDeclaration(entries, metadata, features) == nil {
			t.Fatal("duplicate entry admitted")
		}
	}
}
