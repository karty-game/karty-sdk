package worldsource

import "fmt"

type totals struct {
	rooms, instances, connections, ports, edges, contents int
}

// Validate checks the complete source graph without expanding prefabs or
// decomposing rooms. Those compiler operations retain their own post-expansion
// bounds and diagnostics.
func Validate(document *Document) error {
	if document == nil {
		return ErrReference
	}
	if document.Version < LegacyVersion || document.Version > Version {
		return ErrVersion
	}
	if document.Version < ActorVersion && usesActorFields(document) {
		return ErrVersion
	}
	if document.Version < PortalVersion && usesNonEuclideanConnections(document) {
		return ErrVersion
	}
	if document.Lighting != nil {
		if document.Version < LightingVersion {
			return fmt.Errorf("lighting requires source version %d: %w", LightingVersion, ErrVersion)
		}
		if err := validateLighting(document.Lighting); err != nil {
			return err
		}
	}
	if err := validateSolids(document); err != nil {
		return err
	}
	if len(document.Prefabs) > MaxPrefabs || len(document.Rooms)+len(document.Instances)+len(document.Solids)+len(document.Contents) == 0 {
		return ErrBounds
	}

	prefabs := make(map[string]*Prefab, len(document.Prefabs))
	for index := range document.Prefabs {
		prefab := &document.Prefabs[index]
		if !validIdentifier(prefab.ID) {
			return fmt.Errorf("prefab %d: %w", index, ErrIdentity)
		}
		if _, exists := prefabs[prefab.ID]; exists {
			return fmt.Errorf("prefab %q: %w", prefab.ID, ErrIdentity)
		}
		prefabs[prefab.ID] = prefab
	}

	counts := totals{contents: len(document.Contents)}
	for _, prefab := range document.Prefabs {
		counts.contents += len(prefab.Contents)
	}
	if _, err := validateScope("world", document.Rooms, document.Instances, document.Connections, nil, prefabs, &counts); err != nil {
		return err
	}
	for _, prefab := range document.Prefabs {
		if len(prefab.Rooms)+len(prefab.Instances)+len(prefab.Solids)+len(prefab.Contents) == 0 {
			return fmt.Errorf("prefab %q: %w", prefab.ID, ErrBounds)
		}
		index, err := validateScope(prefab.ID, prefab.Rooms, prefab.Instances, prefab.Connections, prefab.Ports, prefabs, &counts)
		if err != nil {
			return err
		}
		if err := validatePorts(prefab.ID, prefab.Ports, index, prefabs, &counts); err != nil {
			return err
		}
	}
	if counts.rooms > MaxRooms || counts.instances > MaxInstances || counts.connections > MaxConnections ||
		counts.ports > MaxPorts || counts.edges > MaxEdges || counts.contents > MaxContents {
		return ErrBounds
	}
	if err := validateDocumentMaterials(document); err != nil {
		return err
	}
	if err := validateDocumentUV(document); err != nil {
		return err
	}
	if err := validatePrefabCycles(document.Prefabs, prefabs); err != nil {
		return err
	}

	return nil
}

func usesNonEuclideanConnections(document *Document) bool {
	for _, connection := range document.Connections {
		if connection.NonEuclidean || connection.Direction != "" && connection.Direction != PortalBoth {
			return true
		}
	}
	for _, prefab := range document.Prefabs {
		for _, connection := range prefab.Connections {
			if connection.NonEuclidean || connection.Direction != "" && connection.Direction != PortalBoth {
				return true
			}
		}
	}

	return false
}
