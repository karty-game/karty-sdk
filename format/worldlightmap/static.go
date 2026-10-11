package worldlightmap

import "github.com/karty-game/karty-sdk/format/world"

// StaticLightIDs returns independently owned authored-order IDs of all fixed
// point sources without a nonzero linked emission pulse. It validates the world
// first. Compiler options decide the receiver/component scope separately.
func StaticLightIDs(document *world.Document) ([]string, error) {
	if err := world.Validate(document); err != nil {
		return nil, err
	}
	return staticLightIDsValidated(document), nil
}

func pulsingLightIDs(document *world.Document) map[string]bool {
	pulsing := make(map[string]bool)
	if document.Emission != nil {
		for _, material := range document.Emission.Materials {
			if material.Pulse != nil && material.Pulse.Depth != 0 {
				for _, id := range material.Lights {
					pulsing[id] = true
				}
			}
		}
	}
	return pulsing
}

func staticLightIDsValidated(document *world.Document) []string {
	ids := make([]string, 0)
	if document.Lighting == nil {
		return ids
	}
	pulsing := pulsingLightIDs(document)
	for _, light := range document.Lighting.Lights {
		if light.Motion == nil && !pulsing[light.ID] {
			ids = append(ids, light.ID)
		}
	}
	return ids
}
