package world

import (
	"fmt"
	"math"
	"unicode/utf8"
)

// Validate checks a complete compiled document before a host prepares private
// indexes or publishes a mounted resource.
func Validate(document *Document) error {
	if document == nil {
		return ErrSyntax
	}
	if document.Version < LegacyVersion || document.Version > Version {
		return ErrVersion
	}
	if document.Lighting != nil {
		if document.Version != Version {
			return fmt.Errorf("lighting requires compiled world version %d: %w", Version, ErrVersion)
		}
		if err := ValidateLighting(document.Lighting); err != nil {
			return err
		}
	}
	if err := validateDocumentSolids(document); err != nil {
		return err
	}
	if len(document.Sectors) == 0 || len(document.Sectors) > MaxSectors || len(document.Contents) > MaxContents {
		return ErrBounds
	}

	if err := validateDocumentMapping(document); err != nil {
		return err
	}

	identities := make(map[string]struct{}, len(document.Sectors))
	walls := 0
	for sectorIndex := range document.Sectors {
		sector := &document.Sectors[sectorIndex]
		if err := validateSector(sectorIndex, sector, identities); err != nil {
			return err
		}
		walls += len(sector.Walls)
		if walls > MaxWalls {
			return ErrBounds
		}
	}
	if err := validatePortals(document.Sectors, document.Version); err != nil {
		return err
	}

	if err := validateDocumentLayers(document); err != nil {
		return err
	}
	contentIdentities := make(map[string]struct{}, len(document.Contents))
	for index := range document.Contents {
		if err := validateContent(index, &document.Contents[index], document.Sectors, contentIdentities, document.Version); err != nil {
			return err
		}
	}

	if err := ValidateEmission(document); err != nil {
		return err
	}
	return ValidateAnimations(document)
}

func validateSector(index int, sector *Sector, identities map[string]struct{}) error {
	if !validIdentifier(sector.ID) || !validIdentifier(sector.SourceRoom) ||
		sector.Instance != "" && !validIdentifier(sector.Instance) {
		return fmt.Errorf("sector %d: %w", index, ErrIdentity)
	}
	if _, exists := identities[sector.ID]; exists {
		return fmt.Errorf("sector %q: %w", sector.ID, ErrIdentity)
	}
	identities[sector.ID] = struct{}{}

	if len(sector.Walls) < 3 || len(sector.Walls) > MaxWallsPerSector ||
		!validPlane(sector.Floor) || !validPlane(sector.Ceiling) {
		return fmt.Errorf("sector %q: %w", sector.ID, ErrGeometry)
	}
	for wallIndex, wall := range sector.Walls {
		next := sector.Walls[(wallIndex+1)%len(sector.Walls)]
		if !validVec2(wall.Start) || !validVec2(wall.End) || wall.End != next.Start ||
			wall.SourceEdge != "" && !validIdentifier(wall.SourceEdge) ||
			distanceSquared(wall.Start, wall.End) < MinEdgeLength*MinEdgeLength {
			return fmt.Errorf("sector %q wall %d: %w", sector.ID, wallIndex, ErrGeometry)
		}
		if wall.Portal < -1 {
			return fmt.Errorf("sector %q wall %d: %w", sector.ID, wallIndex, ErrPortal)
		}
		for pointIndex, pointWall := range sector.Walls {
			if pointIndex == wallIndex || pointIndex == (wallIndex+1)%len(sector.Walls) {
				continue
			}
			if orientedDistance(wall.Start, wall.End, pointWall.Start) <= geometryEpsilon {
				return fmt.Errorf("sector %q is not strictly convex CCW: %w", sector.ID, ErrGeometry)
			}
		}
		floor, ceiling := planeHeight(sector.Floor, wall.Start), planeHeight(sector.Ceiling, wall.Start)
		if !finiteBounded(floor) || !finiteBounded(ceiling) || ceiling-floor < MinClearance {
			return fmt.Errorf("sector %q has no clearance: %w", sector.ID, ErrGeometry)
		}
	}

	return nil
}

func validatePortals(sectors []Sector, version uint16) error {
	for sectorIndex := range sectors {
		sector := &sectors[sectorIndex]
		for wallIndex, wall := range sector.Walls {
			if wall.Portal < 0 {
				if wall.PortalWall != 0 {
					return fmt.Errorf("sector %q wall %d: %w", sector.ID, wallIndex, ErrPortal)
				}
				continue
			}
			neighborIndex := int(wall.Portal)
			if neighborIndex >= len(sectors) || version < Version && neighborIndex == sectorIndex {
				return fmt.Errorf("sector %q wall %d: %w", sector.ID, wallIndex, ErrPortal)
			}
			neighbor := &sectors[neighborIndex]
			if version >= Version {
				if wall.PortalWall == 0 || int(wall.PortalWall) > len(neighbor.Walls) ||
					!equalPortalLength(wall, neighbor.Walls[int(wall.PortalWall)-1]) {
					return fmt.Errorf("sector %q wall %d: %w", sector.ID, wallIndex, ErrPortal)
				}
				continue
			}
			if wall.PortalWall != 0 {
				return fmt.Errorf("sector %q wall %d: %w", sector.ID, wallIndex, ErrPortal)
			}
			reverse := -1
			for candidateIndex, candidate := range neighbor.Walls {
				if int(candidate.Portal) == sectorIndex && candidate.Start == wall.End && candidate.End == wall.Start {
					if reverse >= 0 {
						return fmt.Errorf("sector %q wall %d has duplicate reverse: %w", sector.ID, wallIndex, ErrPortal)
					}
					reverse = candidateIndex
				}
			}
			if reverse < 0 || !portalHasClearance(sector, neighbor, wall.Start) ||
				!portalHasClearance(sector, neighbor, wall.End) || planesCrossOnEdge(sector.Floor, neighbor.Floor, wall) ||
				planesCrossOnEdge(sector.Ceiling, neighbor.Ceiling, wall) {
				return fmt.Errorf("sector %q wall %d: %w", sector.ID, wallIndex, ErrPortal)
			}
		}
	}

	return nil
}

func equalPortalLength(left, right Wall) bool {
	leftLength := distanceSquared(left.Start, left.End)
	rightLength := distanceSquared(right.Start, right.End)
	tolerance := geometryEpsilon * max(1, leftLength, rightLength)

	return math.Abs(leftLength-rightLength) <= tolerance
}

func validateContent(index int, content *Content, sectors []Sector, identities map[string]struct{}, version uint16) error {
	if !validIdentifier(content.ID) || !validIdentifier(content.SourceID) || !validKind(content.Kind) ||
		content.Instance != "" && !validIdentifier(content.Instance) || content.Sector >= uint32(len(sectors)) ||
		!validVec3(content.Position) {
		return fmt.Errorf("content %d: %w", index, ErrContent)
	}
	if _, exists := identities[content.ID]; exists {
		return fmt.Errorf("content %q: %w", content.ID, ErrIdentity)
	}
	identities[content.ID] = struct{}{}
	if version < ActorVersion && content.Actor != nil {
		return fmt.Errorf("content %q actor requires version %d: %w", content.ID, ActorVersion, ErrVersion)
	}
	if content.Actor != nil {
		if err := validateActor(content.Actor); err != nil {
			return fmt.Errorf("content %q actor: %w", content.ID, err)
		}
	}

	sector := &sectors[content.Sector]
	point := Vec2{X: content.Position.X, Y: content.Position.Y}
	if !containsPoint(sector, point) || content.Position.Z < planeHeight(sector.Floor, point) ||
		content.Position.Z > planeHeight(sector.Ceiling, point) {
		return fmt.Errorf("content %q: %w", content.ID, ErrContent)
	}

	return nil
}

func validateActor(actor *Actor) error {
	if !finiteBounded(actor.Yaw) || !finiteBounded(actor.Pitch) || !finiteBounded(actor.Roll) ||
		!validVec3(actor.Scale) || actor.Scale.X <= 0 || actor.Scale.Y <= 0 || actor.Scale.Z <= 0 ||
		len(actor.Tags) > MaxTags {
		return ErrContent
	}
	previous := ""
	for _, tag := range actor.Tags {
		if len(tag) == 0 || len(tag) > MaxTagBytes || !utf8.ValidString(tag) || tag <= previous {
			return ErrContent
		}
		previous = tag
	}
	if actor.Sprite == nil {
		return nil
	}
	sprite := actor.Sprite
	if sprite.AssetID == 0 ||
		(sprite.Facing != SpriteCameraFacing && sprite.Facing != SpriteUpright &&
			sprite.Facing != SpriteCross && sprite.Facing != SpriteFixed) ||
		(sprite.Alpha != SpriteCutout && sprite.Alpha != SpriteBlend) ||
		!finiteBounded(sprite.Width) || sprite.Width <= 0 || !finiteBounded(sprite.Height) || sprite.Height <= 0 ||
		!finiteBounded(sprite.OriginX) || sprite.OriginX < 0 || sprite.OriginX > 1 ||
		!finiteBounded(sprite.OriginY) || sprite.OriginY < 0 || sprite.OriginY > 1 {
		return ErrContent
	}

	return nil
}

func validIdentifier(value string) bool {
	return len(value) > 0 && len(value) <= MaxIdentifierBytes && utf8.ValidString(value)
}

func validKind(value string) bool {
	return len(value) > 0 && len(value) <= MaxKindBytes && utf8.ValidString(value)
}

func validVec2(value Vec2) bool {
	return finiteBounded(value.X) && finiteBounded(value.Y)
}

func validVec3(value Vec3) bool {
	return finiteBounded(value.X) && finiteBounded(value.Y) && finiteBounded(value.Z)
}

func validPlane(value Plane) bool {
	return finiteBounded(value.A) && finiteBounded(value.B) && finiteBounded(value.C)
}

func finiteBounded(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && math.Abs(value) <= MaxCoordinate
}

func distanceSquared(left, right Vec2) float64 {
	dx, dy := right.X-left.X, right.Y-left.Y

	return dx*dx + dy*dy
}

func orientedDistance(start, end, point Vec2) float64 {
	dx, dy := end.X-start.X, end.Y-start.Y
	cross := dx*(point.Y-start.Y) - dy*(point.X-start.X)
	length := math.Hypot(dx, dy)
	if length == 0 {
		return 0
	}

	return cross / length
}

func planeHeight(plane Plane, point Vec2) float64 {
	return plane.A*point.X + plane.B*point.Y + plane.C
}

func portalHasClearance(left, right *Sector, point Vec2) bool {
	floor := max(planeHeight(left.Floor, point), planeHeight(right.Floor, point))
	ceiling := min(planeHeight(left.Ceiling, point), planeHeight(right.Ceiling, point))

	return ceiling-floor >= MinClearance
}

func planesCrossOnEdge(left, right Plane, wall Wall) bool {
	start := planeHeight(left, wall.Start) - planeHeight(right, wall.Start)
	end := planeHeight(left, wall.End) - planeHeight(right, wall.End)

	return start < -geometryEpsilon && end > geometryEpsilon || start > geometryEpsilon && end < -geometryEpsilon
}

func containsPoint(sector *Sector, point Vec2) bool {
	for _, wall := range sector.Walls {
		if orientedDistance(wall.Start, wall.End, point) < -geometryEpsilon {
			return false
		}
	}

	return true
}
