package world

import (
	"fmt"
	"github.com/karty-game/karty-sdk/format/cartridge"
)

const (
	StaticSolidsVersion = uint32(1)
	FeatureStaticSolids = cartridge.FeatureWorldStaticSolidsV1
	MaxStaticSolids     = 1024
	MaxSolidVertices    = 64
)

// StaticSolids contains independently placed opaque volumes. They never alter
// the sector/portal graph. Hosts must advertise FeatureStaticSolids.
type StaticSolids struct {
	Version uint32  `json:"version"`
	Items   []Solid `json:"items"`
}

// Solid is a strictly convex CCW extrusion between two elevation planes.
// Collision blocks movement but never makes Top a walkable floor. Optional
// mappings are shared by the indicated surface group; SideUV is not per-edge.
type Solid struct {
	ID             string     `json:"id"`
	Footprint      []Vec2     `json:"footprint"`
	Bottom         Plane      `json:"bottom"`
	Top            Plane      `json:"top"`
	SideMaterial   uint32     `json:"side_material"`
	TopMaterial    uint32     `json:"top_material"`
	BottomMaterial uint32     `json:"bottom_material"`
	SideUV         *SurfaceUV `json:"side_uv,omitempty"`
	TopUV          *SurfaceUV `json:"top_uv,omitempty"`
	BottomUV       *SurfaceUV `json:"bottom_uv,omitempty"`
	Collision      bool       `json:"collision,omitempty"`
}

// ValidateStaticSolids validates every item, including the last, before mounting.
// Solids may overlap each other and sectors; sector validation is unchanged.
func ValidateStaticSolids(solids *StaticSolids) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: %w", ErrStaticSolids, err)
		}
	}()
	if solids == nil {
		return ErrSyntax
	}
	if solids.Version != StaticSolidsVersion {
		return ErrVersion
	}
	if len(solids.Items) > MaxStaticSolids {
		return ErrBounds
	}
	identities := make(map[string]struct{}, len(solids.Items))
	for index := range solids.Items {
		solid := &solids.Items[index]
		if !validIdentifier(solid.ID) {
			return fmt.Errorf("solid %d: %w", index, ErrIdentity)
		}
		if _, exists := identities[solid.ID]; exists {
			return fmt.Errorf("solid %q: %w", solid.ID, ErrIdentity)
		}
		identities[solid.ID] = struct{}{}
		if len(solid.Footprint) > MaxSolidVertices {
			return ErrBounds
		}
		if len(solid.Footprint) < 3 || !validPlane(solid.Bottom) || !validPlane(solid.Top) {
			return fmt.Errorf("solid %q: %w", solid.ID, ErrGeometry)
		}
		if solid.SideMaterial == 0 || solid.TopMaterial == 0 || solid.BottomMaterial == 0 {
			return fmt.Errorf("solid %q materials: %w", solid.ID, ErrGeometry)
		}
		for edge, start := range solid.Footprint {
			end := solid.Footprint[(edge+1)%len(solid.Footprint)]
			if !validVec2(start) || distanceSquared(start, end) < MinEdgeLength*MinEdgeLength {
				return fmt.Errorf("solid %q edge %d: %w", solid.ID, edge, ErrGeometry)
			}
			for vertex, point := range solid.Footprint {
				if vertex != edge && vertex != (edge+1)%len(solid.Footprint) && orientedDistance(start, end, point) <= geometryEpsilon {
					return fmt.Errorf("solid %q is not strictly convex CCW: %w", solid.ID, ErrGeometry)
				}
			}
			bottom, top := planeHeight(solid.Bottom, start), planeHeight(solid.Top, start)
			if !finiteBounded(bottom) || !finiteBounded(top) || top-bottom < MinClearance {
				return fmt.Errorf("solid %q has no thickness: %w", solid.ID, ErrGeometry)
			}
		}
		for _, mapping := range []*SurfaceUV{solid.SideUV, solid.TopUV, solid.BottomUV} {
			if mapping != nil {
				if err := ValidateSurfaceUV(mapping); err != nil {
					return fmt.Errorf("solid %q: %w", solid.ID, err)
				}
			}
		}
	}
	return nil
}

func validateDocumentSolids(document *Document) error {
	if document.StaticSolids == nil {
		return nil
	}
	if document.Version != Version {
		return ErrVersion
	}
	if err := ValidateStaticSolids(document.StaticSolids); err != nil {
		return err
	}
	for _, solid := range document.StaticSolids.Items {
		if document.MaterialMapping == nil && (solid.SideUV != nil || solid.TopUV != nil || solid.BottomUV != nil) {
			return ErrMaterialMapping
		}
	}
	return nil
}
