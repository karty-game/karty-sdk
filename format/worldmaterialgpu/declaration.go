package worldmaterialgpu

import (
	"slices"

	"github.com/karty-game/karty-sdk/format/cartridge"
	"github.com/karty-game/karty-sdk/format/level"
	"github.com/karty-game/karty-sdk/format/worldmaterial"
)

// ValidateDeclaration rejects partial, orphaned or undeclared prepared arrays.
// A cartridge may contain ordinary levels alongside levels using this feature.
// The base material layout remains required, but old QOI images are not required
// for a level selecting the prepared GPU material contract.
func ValidateDeclaration(entries []level.Entry, metadata level.Metadata, features []string) error {
	names := [4]string{DescriptorEntry, AlbedoEntry, SSBumpEntry, DetailEntry}
	var counts [4]int
	layouts := 0
	for _, entry := range entries {
		if entry.Name == worldmaterial.LayoutEntry {
			if entry.Kind != level.EntryData {
				return ErrMaterial
			}
			layouts++
		}
		for i, name := range names {
			if entry.Name == name {
				if entry.Kind != level.EntryData {
					return ErrMaterial
				}
				counts[i]++
			}
		}
	}
	marker, declared := metadata[MetadataKey]
	if !declared {
		if counts != [4]int{} {
			return ErrMaterial
		}
		return nil
	}
	value, ok := marker.AsString()
	if !ok || value != Feature || counts != [4]int{1, 1, 1, 1} || layouts != 1 ||
		!slices.Contains(features, Feature) || !slices.Contains(features, cartridge.FeatureWorldSectorsV1) ||
		(!slices.Contains(features, cartridge.FeatureWorldMaterialAtlasV1) && !slices.Contains(features, cartridge.FeatureWorldMaterialAtlasV2)) {
		return ErrMaterial
	}
	return nil
}
