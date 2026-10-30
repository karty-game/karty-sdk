package worldlightmap

import "math"

// Compact charts carry their coverage explicitly, including all-zero RNM.
// An absent image is legal only with the complete compact schema payload.
func validFixedImageStructure(lighting FixedLighting) bool {
	if lighting.Schema == FixedSchema {
		return len(lighting.ConstantRNM) == 0 && lighting.ImageBytes > 0 &&
			lighting.ImageBytes <= MaxPrebakeImageSize && lowerSHA256(lighting.ImageSHA256)
	}
	if lighting.Schema != FixedCompactSchema || lighting.Producer != FixedSpatialProducer ||
		lighting.ImageBytes != 0 || lighting.ImageSHA256 != "" || lighting.Width != 0 || lighting.Height != 0 ||
		len(lighting.ConstantRNM) < 1 || len(lighting.ConstantRNM) > MaxCharts {
		return false
	}
	for index, chart := range lighting.ConstantRNM {
		if chart.Chart != index {
			return false
		}
		for _, value := range chart.Coefficients {
			if !finite(float64(value)) || value < 0 || value > float32(lighting.RGBMRange) ||
				value == 0 && math.Signbit(float64(value)) {
				return false
			}
		}
	}
	return true
}
