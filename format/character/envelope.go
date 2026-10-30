package character

import "math"

const (
	maximumHierarchyScale       = 1e6
	minimumHierarchyScale       = 1e-6
	maximumHierarchyTranslation = 1e9
)

type envelope struct {
	minimum, maximum float64
	translation      [3]float64
}

// Rotation preserves Euclidean length. Products of the smallest/largest scale
// magnitudes therefore bound hierarchy transforms without enumerating every
// combination of independent animation clocks or multiplying float matrices.
func validateHierarchyEnvelope(model *Model, bank Bank, names map[string]int) error {
	local := make([]envelope, len(model.Nodes))
	for index, node := range model.Nodes {
		local[index].minimum = math.Inf(1)
		for axis := range 3 {
			magnitude := math.Abs(float64(node.Scale[axis]))
			local[index].minimum = min(local[index].minimum, magnitude)
			local[index].maximum = max(local[index].maximum, magnitude)
			local[index].translation[axis] = math.Abs(float64(node.Translation[axis]))
		}
	}
	for _, clip := range bank.Clips {
		for _, track := range clip.Tracks {
			index := names[track.Node]
			if track.Path == Rotation {
				continue
			}
			for at, value := range track.Values {
				if track.Path == Translation {
					local[index].translation[at%3] = max(local[index].translation[at%3], math.Abs(float64(value)))
					continue
				}
				// Blending a reflected bind scale into a positive animated scale crosses
				// a singular transform. Reflected nodes without scale tracks remain valid.
				if model.Nodes[index].Scale[at%3] < 0 {
					return ErrInvalid
				}
				local[index].minimum = min(local[index].minimum, float64(value))
				local[index].maximum = max(local[index].maximum, float64(value))
			}
		}
	}
	for index := range model.Nodes {
		minimum, maximum, translation := float64(1), float64(1), float64(0)
		for current := index; current >= 0; current = model.Nodes[current].Parent {
			bound := local[current]
			distance := math.Sqrt(
				bound.translation[0]*bound.translation[0] + bound.translation[1]*bound.translation[1] + bound.translation[2]*bound.translation[2],
			)
			translation = bound.maximum*translation + distance
			minimum *= bound.minimum
			maximum *= bound.maximum
			if minimum < minimumHierarchyScale || maximum > maximumHierarchyScale || translation > maximumHierarchyTranslation {
				return ErrInvalid
			}
		}
	}
	return nil
}
