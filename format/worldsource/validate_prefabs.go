package worldsource

import "fmt"

func validateInstance(scope string, instance *Instance, prefabs map[string]*Prefab) error {
	if !validIdentifier(instance.ID) || !validIdentifier(instance.Prefab) || prefabs[instance.Prefab] == nil ||
		!validVec3(instance.Transform.Translation) || !finite(instance.Transform.YawDegrees) ||
		!finite(instance.Transform.Scale) || instance.Transform.Scale < 0 ||
		len(instance.Materials) > MaxMaterialOverrides {
		return fmt.Errorf("scope %q instance %q: %w", scope, instance.ID, ErrReference)
	}
	if !validTags(instance.Tags) {
		return fmt.Errorf("scope %q instance %q tags: %w", scope, instance.ID, ErrReference)
	}
	overrides := make(map[string]struct{}, len(instance.Materials))
	for _, override := range instance.Materials {
		if !validIdentifier(override.From) || !validIdentifier(override.To) {
			return fmt.Errorf("scope %q instance %q: %w", scope, instance.ID, ErrReference)
		}
		if _, exists := overrides[override.From]; exists {
			return fmt.Errorf("scope %q instance %q duplicate material: %w", scope, instance.ID, ErrIdentity)
		}
		overrides[override.From] = struct{}{}
	}

	return nil
}

func validatePrefabCycles(definitions []Prefab, prefabs map[string]*Prefab) error {
	state := make(map[string]uint8, len(definitions))
	var visit func(string) error
	visit = func(id string) error {
		switch state[id] {
		case 1:
			return fmt.Errorf("prefab %q: %w", id, ErrPrefabCycle)
		case 2:
			return nil
		}
		state[id] = 1
		for _, instance := range prefabs[id].Instances {
			if err := visit(instance.Prefab); err != nil {
				return err
			}
		}
		state[id] = 2

		return nil
	}

	for _, prefab := range definitions {
		if err := visit(prefab.ID); err != nil {
			return err
		}
	}

	return nil
}
