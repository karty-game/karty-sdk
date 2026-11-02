// Package reflectionplan places saved static cubemaps independently of a GPU
// backend. Its room grouping and clearance search preserve existing captures.
package reflectionplan

import (
	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldreflection"
)

// Plan creates metadata for an admitted world. Capture supplies each probe's
// image before encoding the resulting public KRC1 bundle.
func Plan(document *world.Document, digest string) (worldreflection.Bundle, error) {
	if err := world.Validate(document); err != nil {
		return worldreflection.Bundle{}, err
	}

	groups, volumes := reflectionVolumes(document)

	bundle := worldreflection.Bundle{Producer: worldreflection.Producer, InputSHA256: digest, Sectors: make([]int, len(document.Sectors))}
	for index := range bundle.Sectors {
		id := groups[index]
		if id >= 0 && volumes[id].High[2] <= volumes[id].Low[2] {
			id = -1
		}

		bundle.Sectors[index] = id
	}

	bundle.Solids = reflectionSolidBindings(document, &groups, &volumes)
	for index, probe := range volumes {
		if probe.High[2] > probe.Low[2] {
			probe.ID = index
			bundle.Probes = append(bundle.Probes, probe)
		}
	}

	return bundle, nil
}
