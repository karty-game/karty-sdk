package worldsource

import "fmt"

const MinUVScale = 0.001

type UVMode string

const (
	UVTriplanar UVMode = "triplanar"
	UVPlanar    UVMode = "planar"
	UVWrap      UVMode = "wrap"
)

type UVAnchor string

const (
	UVWorld  UVAnchor = "world"
	UVTop    UVAnchor = "top"
	UVBottom UVAnchor = "bottom"
)

// UVSettings contains editor/compiler projection controls available in source v5.
// Empty Mode/Anchor and nil numeric fields inherit root -> room -> edge.
// Defaults are triplanar/world, scale (1,1), offset (0,0), rotation 0 degrees.
// Scale uses world units per repeat; Offset uses texture repeats. Rotation's
// pointer preserves an explicit zero override. Hosts receive only baked planes.
type UVSettings struct {
	Mode            UVMode   `json:"mode,omitempty"             yaml:"mode,omitempty"`
	Anchor          UVAnchor `json:"anchor,omitempty"           yaml:"anchor,omitempty"`
	Scale           *Vec2    `json:"scale,omitempty"            yaml:"scale,omitempty"`
	Offset          *Vec2    `json:"offset,omitempty"           yaml:"offset,omitempty"`
	RotationDegrees *float64 `json:"rotation_degrees,omitempty" yaml:"rotation_degrees,omitempty"`
}

// ValidateUVSettings checks typed authored controls without resolving inheritance.
func ValidateUVSettings(settings *UVSettings) error {
	if settings == nil {
		return ErrUVMapping
	}
	if settings.Mode != "" && settings.Mode != UVTriplanar && settings.Mode != UVPlanar && settings.Mode != UVWrap {
		return ErrUVMapping
	}
	if settings.Anchor != "" && settings.Anchor != UVWorld && settings.Anchor != UVTop && settings.Anchor != UVBottom {
		return ErrUVMapping
	}
	if settings.Scale != nil && (!validVec2(*settings.Scale) || settings.Scale.X < MinUVScale || settings.Scale.Y < MinUVScale) {
		return ErrUVMapping
	}
	if settings.Offset != nil && !validVec2(*settings.Offset) {
		return ErrUVMapping
	}
	if settings.RotationDegrees != nil && !finite(*settings.RotationDegrees) {
		return ErrUVMapping
	}
	return nil
}

func validateDocumentUV(document *Document) error {
	if err := validateVersionedUV(document.UV, document.Version, false); err != nil {
		return fmt.Errorf("world UV: %w", err)
	}
	if err := validateRoomsUV(document.Rooms, document.Version); err != nil {
		return err
	}
	for _, prefab := range document.Prefabs {
		if err := validateRoomsUV(prefab.Rooms, document.Version); err != nil {
			return fmt.Errorf("prefab %q: %w", prefab.ID, err)
		}
	}
	return nil
}

func validateRoomsUV(rooms []Room, version uint16) error {
	for _, room := range rooms {
		for _, surface := range []*UVSettings{room.FloorUV, room.CeilingUV} {
			if err := validateVersionedUV(surface, version, true); err != nil {
				return fmt.Errorf("room %q floor/ceiling UV: %w", room.ID, err)
			}
		}
		if err := validateVersionedUV(room.WallUV, version, false); err != nil {
			return fmt.Errorf("room %q wall UV: %w", room.ID, err)
		}
		for _, edge := range room.Boundary {
			if err := validateVersionedUV(edge.UV, version, false); err != nil {
				return fmt.Errorf("room %q edge %q UV: %w", room.ID, edge.ID, err)
			}
		}
	}
	return nil
}

func validateVersionedUV(settings *UVSettings, version uint16, horizontal bool) error {
	if settings == nil {
		return nil
	}
	if version < MappingVersion {
		return ErrVersion
	}
	if err := ValidateUVSettings(settings); err != nil {
		return err
	}
	if horizontal && (settings.Mode == UVWrap || settings.Anchor == UVTop || settings.Anchor == UVBottom) {
		return ErrUVMapping
	}
	return nil
}
