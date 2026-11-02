package worldchunk

import "github.com/karty-game/karty-sdk/format/world"

// SolidOwners validates the compiled world and returns each static solid's
// first sector with positive footprint overlap, using the same ownership policy
// as Plan without overrides. If no positive area exists, exterior solids retain
// their first conservative bounds owner. This build-time query leaves the
// document unchanged and returns one entry per solid, or nil for no solids.
func SolidOwners(document *world.Document) ([]int, error) {
	if err := world.Validate(document); err != nil {
		return nil, err
	}
	return solidOwners(document)
}
