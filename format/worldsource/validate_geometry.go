package worldsource

import (
	"fmt"
	"math"
)

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
