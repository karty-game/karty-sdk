package worldvegetation

// These records preserve the released KWV1 field layout. Regenerate their
// bounded codecs with the same owning generator as newer versions.
type legacyDocument struct {
	Version uint16
	Presets []legacyPreset
	Layers  []Layer
}
type legacyPreset struct {
	Name          string
	Texture       uint32
	Width         Range
	Height        Range
	WindAmplitude float64
	WindFrequency float64
	DrawDistance  float64
}

func toLegacy(d Document) legacyDocument {
	result := legacyDocument{Version: d.Version, Layers: d.Layers}
	if d.Presets != nil {
		result.Presets = make([]legacyPreset, len(d.Presets))
	}
	for i, p := range d.Presets {
		result.Presets[i] = legacyPreset{p.Name, p.Texture, p.Width, p.Height, p.WindAmplitude, p.WindFrequency, p.DrawDistance}
	}
	return result
}
func fromLegacy(d legacyDocument) Document {
	result := Document{Version: d.Version, Layers: d.Layers}
	if d.Presets != nil {
		result.Presets = make([]Preset, len(d.Presets))
	}
	for i, p := range d.Presets {
		result.Presets[i] = Preset{
			Name:          p.Name,
			Texture:       p.Texture,
			Width:         p.Width,
			Height:        p.Height,
			WindAmplitude: p.WindAmplitude,
			WindFrequency: p.WindFrequency,
			DrawDistance:  p.DrawDistance,
		}
	}
	return result
}

// These records retain the KWV2 layout before optional normal maps.
type legacyGeometryDocument struct {
	Version uint16
	Presets []legacyGeometryPreset
	Layers  []Layer
}
type legacyGeometryPreset struct {
	Name          string
	Texture       uint32
	Geometry      uint8
	Width         Range
	Height        Range
	WindAmplitude float64
	WindFrequency float64
	DrawDistance  float64
}

func toLegacyGeometry(d Document) legacyGeometryDocument {
	result := legacyGeometryDocument{Version: d.Version, Layers: d.Layers}
	if d.Presets != nil {
		result.Presets = make([]legacyGeometryPreset, len(d.Presets))
	}
	for i, p := range d.Presets {
		result.Presets[i] = legacyGeometryPreset{
			p.Name,
			p.Texture,
			p.Geometry,
			p.Width,
			p.Height,
			p.WindAmplitude,
			p.WindFrequency,
			p.DrawDistance,
		}
	}
	return result
}
func fromLegacyGeometry(d legacyGeometryDocument) Document {
	result := Document{Version: d.Version, Layers: d.Layers}
	if d.Presets != nil {
		result.Presets = make([]Preset, len(d.Presets))
	}
	for i, p := range d.Presets {
		result.Presets[i] = Preset{
			Name:          p.Name,
			Texture:       p.Texture,
			Geometry:      p.Geometry,
			Width:         p.Width,
			Height:        p.Height,
			WindAmplitude: p.WindAmplitude,
			WindFrequency: p.WindFrequency,
			DrawDistance:  p.DrawDistance,
		}
	}
	return result
}
