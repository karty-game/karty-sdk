package worldlightmap

import (
	"math"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldterrain"
)

func ValidateFixedTerrainBinding(lighting FixedLighting, terrain *worldterrain.Document) error {
	if terrain == nil {
		if len(lighting.Terrain) != 0 {
			return ErrLayout
		}
		return nil
	}
	encoded, err := worldterrain.Encode(*terrain)
	if err != nil {
		return err
	}
	if digest(encoded) != lighting.TerrainReceiverSHA256 || len(terrain.Patches) != len(lighting.Terrain) {
		return ErrLayout
	}
	for index, patch := range terrain.Patches {
		if lighting.Terrain[index].Patch != index ||
			len(lighting.Terrain[index].Coefficients) != 9*(int(patch.Columns)+1)*(int(patch.Rows)+1) {
			return ErrLayout
		}
	}
	return nil
}

// TerrainLightingBasis is the shared producer/consumer RNM terrain frame.
func TerrainLightingBasis(normal world.Vec3) (world.Vec3, world.Vec3) {
	normal = unit(normal)
	axis := world.Vec3{Z: 1}
	if math.Abs(normal.Z) > .9 {
		axis = world.Vec3{Y: 1}
	}
	tangent := unit(cross(axis, normal))
	return tangent, cross(normal, tangent)
}
