package worldgpuchunk

import (
	"slices"
	"strings"

	"github.com/karty-game/karty-sdk/format/cartridge"
	"github.com/karty-game/karty-sdk/format/level"
)

// ValidateDeclaration requires complete, explicitly selected root or cell data.
// Reserved entries cannot silently select a legacy renderer or partial asset.
func ValidateDeclaration(entries []level.Entry, metadata level.Metadata, features []string) error {
	counts := [3]int{}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name, "@world/gpu-chunks/") && entry.Name != CatalogEntry && entry.Name != PayloadEntry &&
			entry.Name != RNMEntry {
			return ErrChunk
		}
		for index, name := range [3]string{CatalogEntry, PayloadEntry, RNMEntry} {
			if entry.Name == name {
				if entry.Kind != level.EntryData {
					return ErrChunk
				}
				counts[index]++
			}
		}
	}
	root, hasRoot := metadata[MetadataKey]
	cell, hasCell := metadata[CellMetadataKey]
	if !hasRoot && !hasCell {
		if counts != [3]int{} {
			return ErrChunk
		}
		return nil
	}
	if hasRoot == hasCell || !slices.Contains(features, Feature) || !slices.Contains(features, cartridge.FeatureWorldMaterialGPUV1) {
		return ErrChunk
	}
	for _, required := range []string{cartridge.FeatureWorldSectorsV1} {
		if !slices.Contains(features, required) {
			return ErrChunk
		}
	}
	marker := root
	expected := [3]int{1, 0, 0}
	if hasCell {
		marker = cell
		expected = [3]int{0, 1, 1}
	}
	value, ok := marker.AsString()
	if !ok || value != Schema || counts != expected {
		return ErrChunk
	}
	return nil
}
