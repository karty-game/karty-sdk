package worldchunk

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldterrain"
)

// ValidateSpatialTerrain is an offline source-coverage proof. Runtime admits
// immutable children by root artifact integrity and compares this fingerprint
// with each terrain lighting payload; it need not retain full terrain source.
func ValidateSpatialTerrain(catalog *Catalog, terrain *worldterrain.Document, document *world.Document) error {
	if catalog == nil || catalog.Schema != SpatialSchema {
		return ErrChunk
	}
	expected := make([]uint16, 0)
	for _, owner := range catalog.Surfaces {
		if owner.Kind == "terrain" {
			expected = append(expected, uint16(owner.Binding.Index))
		}
	}
	if terrain == nil {
		if len(expected) != 0 || catalog.TerrainSHA256 != "" {
			return ErrChunk
		}
		return nil
	}
	if len(expected) == 0 || !slices.Equal(expected, terrain.ExteriorSectors) {
		return fmt.Errorf("terrain replacement ownership differs: %w", ErrChunk)
	}
	if err := worldterrain.ValidateForWorld(terrain, document); err != nil {
		return err
	}
	encoded, err := worldterrain.Encode(*terrain)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(encoded)
	if hex.EncodeToString(sum[:]) != catalog.TerrainSHA256 {
		return fmt.Errorf("terrain replacement fingerprint differs: %w", ErrChunk)
	}
	return nil
}
