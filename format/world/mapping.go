package world

import (
	"fmt"
	"math"

	"github.com/karty-game/karty-sdk/format/cartridge"
)

const (
	MaterialMappingVersion = uint32(1)
	FeatureMaterialMapping = cartridge.FeatureWorldMaterialMappingV1
	MaxUVProjectionValue   = 1e12
	UVWeightTolerance      = 1e-9
)

// MaterialMapping opts a compiled v3 world into world/material-mapping@1.
// Every floor, ceiling and wall must then supply complete baked projections.
// Projection setup, authoring modes and anchors belong to the compiler.
type MaterialMapping struct {
	Version uint32 `json:"version"`
}

// SurfaceUV contains one planar/wrapped projection or three blended projections.
// Weights are constant per surface, finite, nonnegative and normalized to one
// within UVWeightTolerance. They apply to all material channels, not just color.
type SurfaceUV struct {
	Projections []UVProjection `json:"projections"`
	Weights     []float64      `json:"weights"`
}

// UVProjection is an authored-position affine mapping to texture repeats.
// Hosts evaluate these planes before clipping; no projection setup is inferred.
type UVProjection struct {
	U UVPlane `json:"u"`
	V UVPlane `json:"v"`
}

// UVPlane evaluates X*position.X+Y*position.Y+Z*position.Z+Offset.
// All coefficients and Offset are finite with absolute value at most 1e12.
type UVPlane struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Z      float64 `json:"z"`
	Offset float64 `json:"offset"`
}

// ValidateSurfaceUV validates complete bounded projection and weight lists.
func ValidateSurfaceUV(mapping *SurfaceUV) error {
	if mapping == nil || (len(mapping.Projections) != 1 && len(mapping.Projections) != 3) || len(mapping.Weights) != len(mapping.Projections) {
		return ErrMaterialMapping
	}
	sum := 0.0
	for index, projection := range mapping.Projections {
		if !validUVPlane(projection.U) || !validUVPlane(projection.V) {
			return fmt.Errorf("projection %d: %w", index, ErrMaterialMapping)
		}
		weight := mapping.Weights[index]
		if math.IsNaN(weight) || math.IsInf(weight, 0) || weight < 0 || weight > 1 {
			return fmt.Errorf("weight %d: %w", index, ErrMaterialMapping)
		}
		sum += weight
	}
	if math.Abs(sum-1) > UVWeightTolerance {
		return fmt.Errorf("projection weight sum: %w", ErrMaterialMapping)
	}
	return nil
}

func validUVPlane(plane UVPlane) bool {
	for _, value := range [4]float64{plane.X, plane.Y, plane.Z, plane.Offset} {
		if math.IsNaN(value) || math.IsInf(value, 0) || math.Abs(value) > MaxUVProjectionValue {
			return false
		}
	}
	return true
}

func validateDocumentMapping(document *Document) error {
	declared := document.MaterialMapping != nil
	if declared {
		if document.Version != Version || document.MaterialMapping.Version != MaterialMappingVersion {
			return ErrVersion
		}
	}
	for sectorIndex := range document.Sectors {
		sector := &document.Sectors[sectorIndex]
		if len(sector.Walls) > MaxWallsPerSector {
			return ErrBounds
		}
		surfaces := []*SurfaceUV{sector.FloorUV, sector.CeilingUV}
		for _, surface := range surfaces {
			if err := validateMappedSurface(surface, declared, document.Version); err != nil {
				return fmt.Errorf("sector %q floor/ceiling: %w", sector.ID, err)
			}
		}
		for wallIndex := range sector.Walls {
			if err := validateMappedSurface(sector.Walls[wallIndex].UV, declared, document.Version); err != nil {
				return fmt.Errorf("sector %q wall %d: %w", sector.ID, wallIndex, err)
			}
		}
	}
	return nil
}

func validateMappedSurface(surface *SurfaceUV, declared bool, version uint16) error {
	if surface == nil {
		if declared {
			return ErrMaterialMapping
		}
		return nil
	}
	if version != Version {
		return ErrVersion
	}
	if !declared {
		return ErrMaterialMapping
	}
	return ValidateSurfaceUV(surface)
}
