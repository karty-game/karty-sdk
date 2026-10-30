package worldlightmap

import (
	"math"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/internal/binwire"
)

// LayoutAdmission supplies accepted root array bounds for compiled lightmap
// references. Geometry digest equality and the receiver selection's equality to
// its parent chunk descriptor are direct metadata checks performed by the host.
type LayoutAdmission struct{ SectorCount, SolidCount int }

// DecodeLayoutArtifact admits compiler-authoritative layout data after exact
// whole-child catalog hash/size verification. It validates bounded records,
// arrays, rectangles, references and finite fields, without rediscovering source
// surfaces, proving chart coverage/overlap or reconstructing authored geometry.
func DecodeLayoutArtifact(encoded []byte, admission LayoutAdmission) (Layout, error) {
	layout, err := decodeLayoutBinary(encoded)
	if err != nil {
		return Layout{}, err
	}
	if err := ValidateLayoutArtifact(&layout, admission); err != nil {
		return Layout{}, err
	}
	return layout, nil
}

// ValidateLayoutArtifact checks structural layout constraints only. Full
// authored-source proof remains available through Validate for build/cache use.
// Callers establish artifact integrity separately; the supplied counts are not
// evidence that an artifact was produced correctly. Inputs are read-only.
func ValidateLayoutArtifact(layout *Layout, admission LayoutAdmission) error {
	if admission.SectorCount <= 0 || admission.SectorCount > world.MaxSectors || admission.SolidCount < 0 ||
		admission.SolidCount > world.MaxStaticSolids ||
		layout == nil ||
		!validLayoutVersion(layout) ||
		!lowerSHA256(layout.GeometrySHA256) ||
		(layout.Algorithm != Algorithm) != (layout.Receivers != nil) ||
		!finite(layout.TexelsPerUnit) ||
		layout.TexelsPerUnit <= 0 ||
		layout.TexelsPerUnit > 64 ||
		layout.Padding < 1 ||
		layout.Padding > 16 ||
		len(layout.Pages) != 1 ||
		len(layout.Charts) == 0 && layout.Selection == nil ||
		len(layout.Charts) > MaxCharts ||
		len(layout.Bindings) == 0 ||
		len(layout.Bindings) > world.MaxWalls+world.MaxSectors*2+world.MaxStaticSolids*(world.MaxSolidVertices+2) {
		return ErrLayout
	}
	page := layout.Pages[0]
	if page.Width != page.Height || !validPageSize(page.Width, layout.Selection != nil) {
		return ErrLayout
	}
	if layout.Receivers != nil &&
		(len(layout.Receivers.Sectors) == 0 || !orderedIndexes(layout.Receivers.Sectors, admission.SectorCount) || !orderedIndexes(layout.Receivers.Solids, admission.SolidCount)) {
		return ErrLayout
	}
	if layout.Selection != nil {
		if err := ValidateSurfaceSelection(layout.Selection, admission); err != nil {
			return err
		}
	}
	for index, chart := range layout.Charts {
		if chart.ID != index || chart.Page != 0 || !validRect(chart.Rect, page) || !validRect(chart.ReceiverRect, page) ||
			chart.ReceiverRect[0] < chart.Rect[0] || chart.ReceiverRect[1] < chart.Rect[1] || chart.ReceiverRect[2] > chart.Rect[2] || chart.ReceiverRect[3] > chart.Rect[3] {
			return ErrLayout
		}
		for _, vector := range []world.Vec3{chart.Normal, chart.Tangent, chart.Bitangent} {
			if !finite(vector.X) || !finite(vector.Y) || !finite(vector.Z) {
				return ErrLayout
			}
		}
		for _, plane := range [2][4]float64{chart.UPlane, chart.VPlane} {
			for _, value := range plane {
				if !finite(value) || math.Abs(value) > 1e15 {
					return ErrLayout
				}
			}
		}
	}
	for _, binding := range layout.Bindings {
		if binding.Chart < -1 || binding.Chart >= len(layout.Charts) || !validArtifactBinding(binding, admission) {
			return ErrLayout
		}
	}
	return validateArtifactBakeRecipe(layout.RuntimeBake)
}

func decodeLayoutBinary(encoded []byte) (Layout, error) {
	if len(encoded) >= 4 && string(encoded[:4]) == "KLM3" {
		return decodeSelectedLayout(encoded)
	}
	return decodeLegacyLayout(encoded)
}

func decodeLegacyLayout(encoded []byte) (Layout, error) {
	if len(encoded) == 0 || len(encoded) > MaxEncodedSize {
		return Layout{}, ErrLayout
	}
	scan := binwire.NewReader(encoded, BinaryMagic, MaxEncodedSize)
	scanWorldlightmapLayout(scan)
	if scan.Done() != nil {
		return Layout{}, ErrLayout
	}
	var layout Layout
	reader := binwire.NewReader(encoded, BinaryMagic, MaxEncodedSize)
	readWorldlightmapLayout(reader, &layout)
	if reader.Done() != nil {
		return Layout{}, ErrLayout
	}
	return layout, nil
}

func validArtifactBinding(binding Binding, admission LayoutAdmission) bool {
	count, edges, capSurface := 0, 0, false
	switch binding.Kind {
	case "sector-floor", "sector-ceiling":
		count, capSurface = admission.SectorCount, true
	case "sector-wall":
		count, edges = admission.SectorCount, world.MaxWallsPerSector
	case "solid-top", "solid-bottom":
		count, capSurface = admission.SolidCount, true
	case "solid-side":
		count, edges = admission.SolidCount, world.MaxSolidVertices
	default:
		return false
	}
	return binding.Index >= 0 && binding.Index < count &&
		(capSurface && binding.Edge == -1 || !capSurface && binding.Edge >= 0 && binding.Edge < edges)
}

func validateArtifactBakeRecipe(recipe *RuntimeBake) error {
	if recipe == nil {
		return nil
	}
	if recipe.ShadowSize < 32 || recipe.ShadowSize > 512 {
		return ErrLayout
	}
	switch recipe.Encoding {
	case PointVisibilityEncoding:
		if recipe.LightID == "" || len(recipe.LightID) > world.MaxIdentifierBytes || recipe.LightIDs != nil {
			return ErrLayout
		}
	case DirectRNMEncoding:
		if recipe.LightID != "" || recipe.LightIDs == nil || len(recipe.LightIDs) > MaxBakeLights {
			return ErrLayout
		}
		seen := make(map[string]struct{}, len(recipe.LightIDs))
		for _, id := range recipe.LightIDs {
			if id == "" || len(id) > world.MaxIdentifierBytes {
				return ErrLayout
			}
			if _, duplicate := seen[id]; duplicate {
				return ErrLayout
			}
			seen[id] = struct{}{}
		}
	default:
		return ErrLayout
	}
	return nil
}
