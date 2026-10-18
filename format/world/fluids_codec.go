package world

import "github.com/karty-game/karty-sdk/internal/binwire"

// KWR3 freezes the KWR1 base, then writes solid renderers (a nil sentinel when
// absent), an explicit backdrop selector (0 means absent), and fluids@1.
func writeFluidExtensions(w *binwire.Writer, d *Document) {
	s := d.SolidRenderers
	if s == nil {
		s = &SolidRenderers{}
	}
	writeWorldSolidRenderers(w, s)
	selector := uint8(0)
	if d.Environment != nil {
		switch d.Environment.Backdrop {
		case EnvironmentBackdropNone:
			selector = 1
		case EnvironmentBackdropMountains:
			selector = 2
		case EnvironmentBackdropCity:
			selector = 3
		}
	}
	w.U8(selector)
	writeWorldFluids(w, d.Fluids)
}
func scanFluidExtensions(r *binwire.Reader) error {
	scanWorldSolidRenderers(r)
	if r.U8() > 3 {
		return ErrEnvironment
	}
	scanWorldFluids(r)
	return r.Err()
}
func readFluidExtensions(r *binwire.Reader, d *Document) error {
	s := new(SolidRenderers)
	readWorldSolidRenderers(r, s)
	if s.Version != 0 || s.Prefabs != nil || s.Actors != nil {
		d.SolidRenderers = s
	}
	selector := r.U8()
	if selector != 0 {
		if d.Environment == nil {
			return ErrEnvironment
		}
		d.Environment.Backdrop = []EnvironmentBackdrop{"", EnvironmentBackdropNone, EnvironmentBackdropMountains, EnvironmentBackdropCity}[selector]
	}
	d.Fluids = new(Fluids)
	readWorldFluids(r, d.Fluids)
	return r.Err()
}
