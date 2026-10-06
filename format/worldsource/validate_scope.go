package worldsource

import "fmt"

type scopeIndex struct {
	rooms     map[string]*Room
	instances map[string]*Instance
	used      map[string]struct{}
}

func validateScope(
	name string,
	rooms []Room,
	instances []Instance,
	connections []Connection,
	ports []Port,
	prefabs map[string]*Prefab,
	counts *totals,
) (*scopeIndex, error) {
	if len(rooms) > MaxRooms || len(instances) > MaxInstances || len(connections) > MaxConnections || len(ports) > MaxPorts {
		return nil, ErrBounds
	}
	index := &scopeIndex{
		rooms: make(map[string]*Room, len(rooms)), instances: make(map[string]*Instance, len(instances)),
		used: make(map[string]struct{}, len(connections)*2+len(ports)),
	}
	for roomIndex := range rooms {
		room := &rooms[roomIndex]
		if !validIdentifier(room.ID) {
			return nil, fmt.Errorf("scope %q room %d: %w", name, roomIndex, ErrIdentity)
		}
		if _, exists := index.rooms[room.ID]; exists {
			return nil, fmt.Errorf("scope %q room %q: %w", name, room.ID, ErrIdentity)
		}
		index.rooms[room.ID] = room
		if err := validateRoom(name, room, counts); err != nil {
			return nil, err
		}
	}
	counts.rooms += len(rooms)

	for instanceIndex := range instances {
		instance := &instances[instanceIndex]
		if err := validateInstance(name, instance, prefabs); err != nil {
			return nil, err
		}
		if _, exists := index.instances[instance.ID]; exists {
			return nil, fmt.Errorf("scope %q instance %q: %w", name, instance.ID, ErrIdentity)
		}
		index.instances[instance.ID] = instance
	}
	counts.instances += len(instances)

	connectionIDs := make(map[string]struct{}, len(connections))
	for connectionIndex := range connections {
		connection := &connections[connectionIndex]
		if !validIdentifier(connection.ID) {
			return nil, fmt.Errorf("scope %q connection %d: %w", name, connectionIndex, ErrIdentity)
		}
		if _, exists := connectionIDs[connection.ID]; exists {
			return nil, fmt.Errorf("scope %q connection %q: %w", name, connection.ID, ErrIdentity)
		}
		connectionIDs[connection.ID] = struct{}{}
		left, leftEdge, err := resolveEndpoint(connection.A, index, prefabs)
		if err != nil {
			return nil, fmt.Errorf("connection %q endpoint a: %w", connection.ID, err)
		}
		right, rightEdge, err := resolveEndpoint(connection.B, index, prefabs)
		if err != nil {
			return nil, fmt.Errorf("connection %q endpoint b: %w", connection.ID, err)
		}
		if left == right || connection.Direction != "" && connection.Direction != PortalBoth &&
			connection.Direction != PortalAToB && connection.Direction != PortalBToA {
			return nil, fmt.Errorf("connection %q: %w", connection.ID, ErrConnection)
		}
		outgoingA := connection.Direction == "" || connection.Direction == PortalBoth || connection.Direction == PortalAToB
		outgoingB := connection.Direction == "" || connection.Direction == PortalBoth || connection.Direction == PortalBToA
		if _, exists := index.used[left]; outgoingA && exists {
			return nil, fmt.Errorf("connection %q reuses outgoing endpoint: %w", connection.ID, ErrConnection)
		}
		if _, exists := index.used[right]; outgoingB && exists {
			return nil, fmt.Errorf("connection %q reuses outgoing endpoint: %w", connection.ID, ErrConnection)
		}
		if !connection.NonEuclidean && leftEdge != nil && rightEdge != nil &&
			(leftEdge.Start != rightEdge.End || leftEdge.End != rightEdge.Start) {
			return nil, fmt.Errorf("connection %q edges do not coincide: %w", connection.ID, ErrConnection)
		}
		if !connection.NonEuclidean && leftEdge != nil && rightEdge != nil &&
			!directConnectionHasClearance(index.rooms[connection.A.Room], index.rooms[connection.B.Room], leftEdge) {
			return nil, fmt.Errorf("connection %q has no aperture: %w", connection.ID, ErrConnection)
		}
		if outgoingA {
			index.used[left] = struct{}{}
		}
		if outgoingB {
			index.used[right] = struct{}{}
		}
	}
	counts.connections += len(connections)

	return index, nil
}

func validatePorts(scope string, ports []Port, index *scopeIndex, prefabs map[string]*Prefab, counts *totals) error {
	portIDs := make(map[string]struct{}, len(ports))
	for portIndex, port := range ports {
		if !validIdentifier(port.ID) {
			return fmt.Errorf("scope %q port %d: %w", scope, portIndex, ErrIdentity)
		}
		if _, exists := portIDs[port.ID]; exists {
			return fmt.Errorf("scope %q port %q: %w", scope, port.ID, ErrIdentity)
		}
		portIDs[port.ID] = struct{}{}
		key, _, err := resolveEndpoint(port.Endpoint, index, prefabs)
		if err != nil {
			return fmt.Errorf("scope %q port %q: %w", scope, port.ID, err)
		}
		if _, exists := index.used[key]; exists {
			return fmt.Errorf("scope %q port %q exposes connected endpoint: %w", scope, port.ID, ErrConnection)
		}
		index.used[key] = struct{}{}
	}
	counts.ports += len(ports)

	return nil
}

func resolveEndpoint(endpoint Endpoint, index *scopeIndex, prefabs map[string]*Prefab) (string, *Edge, error) {
	roomEndpoint := endpoint.Room != "" || endpoint.Edge != ""
	instanceEndpoint := endpoint.Instance != "" || endpoint.Port != ""
	if roomEndpoint == instanceEndpoint {
		return "", nil, ErrReference
	}
	if roomEndpoint {
		if !validIdentifier(endpoint.Room) || !validIdentifier(endpoint.Edge) {
			return "", nil, ErrReference
		}
		room := index.rooms[endpoint.Room]
		if room == nil {
			return "", nil, ErrReference
		}
		for edgeIndex := range room.Boundary {
			edge := &room.Boundary[edgeIndex]
			if edge.ID == endpoint.Edge {
				return "room:" + endpoint.Room + "/" + endpoint.Edge, edge, nil
			}
		}

		return "", nil, ErrReference
	}

	if !validIdentifier(endpoint.Instance) || !validIdentifier(endpoint.Port) {
		return "", nil, ErrReference
	}
	instance := index.instances[endpoint.Instance]
	if instance == nil {
		return "", nil, ErrReference
	}
	prefab := prefabs[instance.Prefab]
	for _, port := range prefab.Ports {
		if port.ID == endpoint.Port {
			return "instance:" + endpoint.Instance + "/" + endpoint.Port, nil, nil
		}
	}

	return "", nil, ErrReference
}
