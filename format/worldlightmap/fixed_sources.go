package worldlightmap

import "github.com/karty-game/karty-sdk/format/world"

func validateFixedSources(lighting FixedLighting, layout Layout, document *world.Document) error {
	eligible, err := StaticLightIDs(document)
	if err != nil {
		return err
	}
	selected := []string(nil)
	if layout.RuntimeBake != nil {
		if layout.RuntimeBake.Encoding != DirectRNMEncoding {
			return ErrLayout
		}
		selected = layout.RuntimeBake.LightIDs
	}
	expected := len(selected)
	environment := world.ResolvedEnvironment{}
	if document.Environment != nil {
		environment, err = world.ResolveEnvironment(document.Environment)
		if err != nil {
			return err
		}
	}
	if environment.SunIntensity > 0 {
		expected++
	}
	if len(lighting.Lights) != expected {
		return ErrLayout
	}
	for index, id := range selected {
		found := false
		for _, candidate := range eligible {
			if candidate == id {
				found = true
				break
			}
		}
		if !found {
			return ErrLayout
		}
		found = false
		for _, source := range document.Lighting.Lights {
			if source.ID != id {
				continue
			}
			light := lighting.Lights[index]
			if light.Kind != "point" || light.ID != id || light.Position != source.Position || light.Color != source.Color ||
				light.Radius != source.Radius {
				return ErrLayout
			}
			found = true
		}
		if !found {
			return ErrLayout
		}
	}
	if environment.SunIntensity > 0 {
		light := lighting.Lights[len(selected)]
		color := scale(environment.SunColor, environment.SunIntensity*(1-environment.CloudAmount))
		if light.ID != "@sun" || light.Kind != "sun" || light.Direction != environment.SunDirection || light.Color != color {
			return ErrLayout
		}
	}
	return nil
}
