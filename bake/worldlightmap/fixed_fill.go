package worldlightmapbake

import "github.com/karty-game/karty-sdk/format/world"

// Authored artistic fill is a fixed, non-bounced input. Baking it into the
// matching surface/probe result preserves one contribution per receiver.
func fixedFill(lighting *world.Lighting, direction vec) vec {
	if lighting == nil {
		return vec{}
	}
	cube := lighting.AmbientCube
	if cube == nil {
		return lighting.Ambient
	}
	x, y, z := cube.PositiveX, cube.PositiveY, cube.PositiveZ
	if direction.X < 0 {
		x = cube.NegativeX
	}
	if direction.Y < 0 {
		y = cube.NegativeY
	}
	if direction.Z < 0 {
		z = cube.NegativeZ
	}
	return add(add(scale(x, square(direction.X)), scale(y, square(direction.Y))), scale(z, square(direction.Z)))
}
