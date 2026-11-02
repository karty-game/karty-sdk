package worldlightmap

// ValidateFixedLightingMetadata admits scalar, chart and probe metadata without
// its compiler-only image. Physical-only KCC6 children use this after binding
// their exact module digest and ownership. Historical full-image APIs remain
// unchanged and still require the image provenance to match.
func ValidateFixedLightingMetadata(lighting FixedLighting, layout Layout) error {
	if err := validateFixedStructure(lighting); err != nil {
		return err
	}
	return validateFixedLayout(lighting, layout)
}
