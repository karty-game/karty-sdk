package world

import "github.com/karty-game/karty-sdk/internal/binwire"

// KWR4 freezes the base and appends explicit, bounded solid/fluid sentinels and appearance.
func writeAppearanceExtensions(w *binwire.Writer, d *Document) {
	s := d.SolidRenderers
	if s == nil {
		s = &SolidRenderers{}
	}
	writeWorldSolidRenderers(w, s)
	f := d.Fluids
	if f == nil {
		f = &Fluids{}
	}
	writeWorldFluids(w, f)
	writeWorldEnvironmentAppearance(w, d.Environment.Appearance)
}
func scanAppearanceExtensions(r *binwire.Reader) error {
	scanWorldSolidRenderers(r)
	scanWorldFluids(r)
	scanWorldEnvironmentAppearance(r)
	return r.Err()
}
func readAppearanceExtensions(r *binwire.Reader, d *Document) error {
	if d.Environment == nil || d.Environment.Version != ExplicitEnvironmentVersion {
		return ErrEnvironment
	}
	s := new(SolidRenderers)
	readWorldSolidRenderers(r, s)
	if s.Version != 0 || s.Prefabs != nil || s.Actors != nil {
		d.SolidRenderers = s
	}
	f := new(Fluids)
	readWorldFluids(r, f)
	if f.Version != 0 || f.Items != nil {
		d.Fluids = f
	}
	d.Environment.Appearance = new(EnvironmentAppearance)
	readWorldEnvironmentAppearance(r, d.Environment.Appearance)
	return r.Err()
}
