package worldsource

import (
	"fmt"
	"math"
	"unicode/utf8"
)

type totals struct {
	rooms, instances, connections, ports, edges, contents int
}

type scopeIndex struct {
	rooms     map[string]*Room
	instances map[string]*Instance
	used      map[string]struct{}
}

// Validate checks the complete source graph without expanding prefabs or
// decomposing rooms. Those compiler operations retain their own post-expansion
// bounds and diagnostics.
func Validate(document *Document) error {
	if document == nil {
		return ErrReference
	}
	if document.Version != LegacyVersion && document.Version != Version {
		return ErrVersion
	}
	if document.Version == LegacyVersion && usesActorFields(document) {
		return ErrVersion
	}
	if len(document.Prefabs) > MaxPrefabs || len(document.Rooms)+len(document.Instances) == 0 {
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

	counts := totals{}
	if _, err := validateScope("world", document.Rooms, document.Instances, document.Connections, nil, prefabs, &counts); err != nil {
		return err
	}
	for _, prefab := range document.Prefabs {
		if len(prefab.Rooms)+len(prefab.Instances) == 0 {
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
	if err := validatePrefabCycles(document.Prefabs, prefabs); err != nil {
		return err
	}

	return nil
}

func usesActorFields(document *Document) bool {
	usesRooms := func(rooms []Room) bool {
		for _, room := range rooms {
			for _, content := range room.Contents {
				if content.Actor != nil {
					return true
				}
			}
		}

		return false
	}
	if usesRooms(document.Rooms) {
		return true
	}
	for _, instance := range document.Instances {
		if len(instance.Tags) > 0 {
			return true
		}
	}
	for _, prefab := range document.Prefabs {
		if usesRooms(prefab.Rooms) {
			return true
		}
		for _, instance := range prefab.Instances {
			if len(instance.Tags) > 0 {
				return true
			}
		}
	}

	return false
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
		if left == right {
			return nil, fmt.Errorf("connection %q: %w", connection.ID, ErrConnection)
		}
		if _, exists := index.used[left]; exists {
			return nil, fmt.Errorf("connection %q reuses endpoint: %w", connection.ID, ErrConnection)
		}
		if _, exists := index.used[right]; exists {
			return nil, fmt.Errorf("connection %q reuses endpoint: %w", connection.ID, ErrConnection)
		}
		if leftEdge != nil && rightEdge != nil && (leftEdge.Start != rightEdge.End || leftEdge.End != rightEdge.Start) {
			return nil, fmt.Errorf("connection %q edges do not coincide: %w", connection.ID, ErrConnection)
		}
		if leftEdge != nil && rightEdge != nil &&
			!directConnectionHasClearance(index.rooms[connection.A.Room], index.rooms[connection.B.Room], leftEdge) {
			return nil, fmt.Errorf("connection %q has no aperture: %w", connection.ID, ErrConnection)
		}
		index.used[left], index.used[right] = struct{}{}, struct{}{}
	}
	counts.connections += len(connections)

	return index, nil
}

func validateRoom(scope string, room *Room, counts *totals) error {
	if len(room.Boundary) < 3 || len(room.Boundary) > MaxEdgesPerRoom ||
		len(room.Contents) > MaxContents ||
		!validPlane(room.Floor) || !validPlane(room.Ceiling) ||
		!validIdentifier(room.FloorMaterial) || !validIdentifier(room.CeilingMaterial) {
		return fmt.Errorf("scope %q room %q: %w", scope, room.ID, ErrGeometry)
	}
	edgeIDs := make(map[string]struct{}, len(room.Boundary))
	area := 0.0
	for edgeIndex, edge := range room.Boundary {
		next := room.Boundary[(edgeIndex+1)%len(room.Boundary)]
		if !validIdentifier(edge.ID) || !validIdentifier(edge.Material) || !validVec2(edge.Start) || !validVec2(edge.End) ||
			edge.End != next.Start || distanceSquared(edge.Start, edge.End) < MinEdgeLength*MinEdgeLength {
			return fmt.Errorf("scope %q room %q edge %d: %w", scope, room.ID, edgeIndex, ErrGeometry)
		}
		if _, exists := edgeIDs[edge.ID]; exists {
			return fmt.Errorf("scope %q room %q edge %q: %w", scope, room.ID, edge.ID, ErrIdentity)
		}
		edgeIDs[edge.ID] = struct{}{}
		area += edge.Start.X*edge.End.Y - edge.End.X*edge.Start.Y
		floor, ceiling := planeHeight(room.Floor, edge.Start), planeHeight(room.Ceiling, edge.Start)
		if !finite(floor) || !finite(ceiling) || ceiling-floor < MinClearance {
			return fmt.Errorf("scope %q room %q: %w", scope, room.ID, ErrGeometry)
		}
	}
	if area <= geometryEpsilon || boundarySelfIntersects(room.Boundary) {
		return fmt.Errorf("scope %q room %q: %w", scope, room.ID, ErrGeometry)
	}
	counts.edges += len(room.Boundary)

	contentIDs := make(map[string]struct{}, len(room.Contents))
	for contentIndex := range room.Contents {
		content := &room.Contents[contentIndex]
		if !validIdentifier(content.ID) || !validIdentifier(content.Kind) || !validVec3(content.Position) {
			return fmt.Errorf("scope %q room %q content %d: %w", scope, room.ID, contentIndex, ErrContent)
		}
		if content.Actor != nil && !validActor(content.Actor) {
			return fmt.Errorf("scope %q room %q content %q: %w", scope, room.ID, content.ID, ErrContent)
		}
		if _, exists := contentIDs[content.ID]; exists {
			return fmt.Errorf("scope %q room %q content %q: %w", scope, room.ID, content.ID, ErrIdentity)
		}
		contentIDs[content.ID] = struct{}{}
		point := Vec2{X: content.Position.X, Y: content.Position.Y}
		if !containsPoint(room.Boundary, point) || content.Position.Z < planeHeight(room.Floor, point) ||
			content.Position.Z > planeHeight(room.Ceiling, point) {
			return fmt.Errorf("scope %q room %q content %q: %w", scope, room.ID, content.ID, ErrContent)
		}
	}
	counts.contents += len(room.Contents)

	return nil
}

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

func validActor(actor *Actor) bool {
	if actor == nil || !finite(actor.YawDegrees) || !finite(actor.PitchDegrees) || !finite(actor.RollDegrees) ||
		!validVec3(actor.Scale) || actor.Scale.X < 0 || actor.Scale.Y < 0 || actor.Scale.Z < 0 || !validTags(actor.Tags) {
		return false
	}
	if actor.Sprite == nil {
		return true
	}
	sprite := actor.Sprite
	return validIdentifier(sprite.Texture) &&
		(sprite.Facing == "camera-facing" || sprite.Facing == "upright" || sprite.Facing == "cross" || sprite.Facing == "fixed") &&
		(sprite.Alpha == "cutout" || sprite.Alpha == "blend") && finite(sprite.Width) && sprite.Width > 0 &&
		finite(sprite.Height) && sprite.Height > 0 && finite(sprite.OriginX) && sprite.OriginX >= 0 && sprite.OriginX <= 1 &&
		finite(sprite.OriginY) && sprite.OriginY >= 0 && sprite.OriginY <= 1
}

func validTags(tags []string) bool {
	if len(tags) > MaxTags {
		return false
	}
	for _, tag := range tags {
		if len(tag) == 0 || len(tag) > MaxTagBytes || !utf8.ValidString(tag) {
			return false
		}
	}

	return true
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

func boundarySelfIntersects(edges []Edge) bool {
	for left := range edges {
		for right := left + 1; right < len(edges); right++ {
			if right == left+1 || left == 0 && right == len(edges)-1 {
				continue
			}
			if segmentsIntersect(edges[left].Start, edges[left].End, edges[right].Start, edges[right].End) {
				return true
			}
		}

	}

	return false
}

func segmentsIntersect(a, b, c, d Vec2) bool {
	abC, abD := cross(a, b, c), cross(a, b, d)
	cdA, cdB := cross(c, d, a), cross(c, d, b)
	if oppositeSigns(abC, abD) && oppositeSigns(cdA, cdB) {
		return true
	}

	return math.Abs(abC) <= geometryEpsilon && pointOnSegment(a, b, c) ||
		math.Abs(abD) <= geometryEpsilon && pointOnSegment(a, b, d) ||
		math.Abs(cdA) <= geometryEpsilon && pointOnSegment(c, d, a) ||
		math.Abs(cdB) <= geometryEpsilon && pointOnSegment(c, d, b)
}

func oppositeSigns(left, right float64) bool {
	return left < -geometryEpsilon && right > geometryEpsilon ||
		left > geometryEpsilon && right < -geometryEpsilon
}

func pointOnSegment(start, end, point Vec2) bool {
	return point.X >= min(start.X, end.X)-geometryEpsilon && point.X <= max(start.X, end.X)+geometryEpsilon &&
		point.Y >= min(start.Y, end.Y)-geometryEpsilon && point.Y <= max(start.Y, end.Y)+geometryEpsilon
}

func containsPoint(edges []Edge, point Vec2) bool {
	inside := false
	for _, edge := range edges {
		if math.Abs(cross(edge.Start, edge.End, point)) <= geometryEpsilon &&
			point.X >= min(edge.Start.X, edge.End.X)-geometryEpsilon && point.X <= max(edge.Start.X, edge.End.X)+geometryEpsilon &&
			point.Y >= min(edge.Start.Y, edge.End.Y)-geometryEpsilon && point.Y <= max(edge.Start.Y, edge.End.Y)+geometryEpsilon {
			return true
		}
		if (edge.Start.Y > point.Y) != (edge.End.Y > point.Y) &&
			point.X < (edge.End.X-edge.Start.X)*(point.Y-edge.Start.Y)/(edge.End.Y-edge.Start.Y)+edge.Start.X {
			inside = !inside
		}
	}

	return inside
}

func validIdentifier(value string) bool {
	return len(value) > 0 && len(value) <= MaxIdentifierBytes && utf8.ValidString(value)
}

func validVec2(value Vec2) bool {
	return finite(value.X) && finite(value.Y)
}

func validVec3(value Vec3) bool {
	return finite(value.X) && finite(value.Y) && finite(value.Z)
}

func validPlane(value Plane) bool {
	return finite(value.A) && finite(value.B) && finite(value.C)
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && math.Abs(value) <= MaxCoordinate
}

func distanceSquared(left, right Vec2) float64 {
	dx, dy := right.X-left.X, right.Y-left.Y

	return dx*dx + dy*dy
}

func cross(start, end, point Vec2) float64 {
	return (end.X-start.X)*(point.Y-start.Y) - (end.Y-start.Y)*(point.X-start.X)
}

func planeHeight(plane Plane, point Vec2) float64 {
	return plane.A*point.X + plane.B*point.Y + plane.C
}

func directConnectionHasClearance(left, right *Room, edge *Edge) bool {
	for _, point := range [...]Vec2{edge.Start, edge.End} {
		floor := max(planeHeight(left.Floor, point), planeHeight(right.Floor, point))
		ceiling := min(planeHeight(left.Ceiling, point), planeHeight(right.Ceiling, point))
		if !finite(floor) || !finite(ceiling) || ceiling-floor < MinClearance {
			return false
		}
	}

	return true
}
