package worldlightmap

import "github.com/karty-game/karty-sdk/internal/binwire"

// EncodeSurfaceSelection exposes the bounded typed face selector to mesh
// envelopes. Its bytes participate in both layout and mesh content identities.
func EncodeSurfaceSelection(selection SurfaceSelection) ([]byte, error) {
	w := binwire.NewWriter("KLS2", MaxEncodedSize)
	writeWorldlightmapSurfaceSelection(w, &selection)
	data, err := w.Bytes()
	if err != nil {
		return nil, ErrLayout
	}
	return data, nil
}
func DecodeSurfaceSelection(data []byte) (SurfaceSelection, error) {
	scan := binwire.NewReader(data, "KLS2", MaxEncodedSize)
	scanWorldlightmapSurfaceSelection(scan)
	if scan.Done() != nil {
		return SurfaceSelection{}, ErrLayout
	}
	r := binwire.NewReader(data, "KLS2", MaxEncodedSize)
	var selection SurfaceSelection
	readWorldlightmapSurfaceSelection(r, &selection)
	if r.Done() != nil {
		return SurfaceSelection{}, ErrLayout
	}
	return selection, nil
}
func encodeSelectedLayout(layout Layout) ([]byte, error) {
	selection := layout.Selection
	layout.Selection = nil
	base, err := encodeLegacyLayout(layout)
	if err != nil {
		return nil, err
	}
	faces, err := EncodeSurfaceSelection(*selection)
	if err != nil {
		return nil, err
	}
	w := binwire.NewWriter("KLM3", MaxEncodedSize)
	w.Count(len(base), false, MaxEncodedSize)
	w.Raw(base)
	w.Count(len(faces), false, MaxEncodedSize)
	w.Raw(faces)
	data, err := w.Bytes()
	if err != nil {
		return nil, ErrLayout
	}
	return data, nil
}
func decodeSelectedLayout(data []byte) (Layout, error) {
	r := binwire.NewReader(data, "KLM3", MaxEncodedSize)
	n, _ := r.Count(MaxEncodedSize, 1, 0)
	base := r.Raw(n)
	n, _ = r.Count(MaxEncodedSize, 1, 0)
	faces := r.Raw(n)
	if r.Done() != nil {
		return Layout{}, ErrLayout
	}
	layout, err := decodeLegacyLayout(base)
	if err != nil {
		return Layout{}, err
	}
	selection, err := DecodeSurfaceSelection(faces)
	if err != nil {
		return Layout{}, err
	}
	layout.Selection = &selection
	if !validLayoutVersion(&layout) {
		return Layout{}, ErrLayout
	}
	return layout, nil
}

// Keep digest admission scan-only: hashing a prepared atlas must not allocate
// another complete chart/binding graph merely to check bounded field encoding.
func scanLayoutBinary(data []byte) error {
	if len(data) >= 4 && string(data[:4]) == "KLM3" {
		r := binwire.NewReader(data, "KLM3", MaxEncodedSize)
		n, _ := r.Count(MaxEncodedSize, 1, 0)
		base := r.Raw(n)
		n, _ = r.Count(MaxEncodedSize, 1, 0)
		faces := r.Raw(n)
		if r.Done() != nil {
			return ErrLayout
		}
		scan := binwire.NewReader(base, BinaryMagic, MaxEncodedSize)
		scanWorldlightmapLayout(scan)
		if scan.Done() != nil {
			return ErrLayout
		}
		scan = binwire.NewReader(faces, "KLS2", MaxEncodedSize)
		scanWorldlightmapSurfaceSelection(scan)
		if scan.Done() != nil {
			return ErrLayout
		}
		return nil
	}
	scan := binwire.NewReader(data, BinaryMagic, MaxEncodedSize)
	scanWorldlightmapLayout(scan)
	if scan.Done() != nil {
		return ErrLayout
	}
	return nil
}
