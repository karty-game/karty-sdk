package worldsource

import "unicode/utf8"

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
