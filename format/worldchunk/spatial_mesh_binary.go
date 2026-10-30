package worldchunk

import (
	"github.com/karty-game/karty-sdk/format/worldlightmap"
	"github.com/karty-game/karty-sdk/internal/binwire"
)

func encodeSelectedMesh(mesh Mesh) ([]byte, error) {
	selection := mesh.Selection
	mesh.Selection = nil
	base, err := encodeLegacyMesh(mesh)
	if err != nil {
		return nil, err
	}
	faces, err := worldlightmap.EncodeSurfaceSelection(*selection)
	if err != nil {
		return nil, err
	}
	w := binwire.NewWriter("KCM3", MaxEncodedSize)
	w.Count(len(base), false, MaxEncodedSize)
	w.Raw(base)
	w.Count(len(faces), false, MaxEncodedSize)
	w.Raw(faces)
	data, err := w.Bytes()
	if err != nil {
		return nil, ErrChunk
	}
	return data, nil
}
func decodeSelectedMesh(data []byte, mesh *Mesh) error {
	r := binwire.NewReader(data, "KCM3", MaxEncodedSize)
	n, _ := r.Count(MaxEncodedSize, 1, 0)
	base := r.Raw(n)
	n, _ = r.Count(MaxEncodedSize, 1, 0)
	faces := r.Raw(n)
	if r.Done() != nil {
		return ErrChunk
	}
	if err := decodeLegacyMesh(base, mesh); err != nil {
		return err
	}
	selection, err := worldlightmap.DecodeSurfaceSelection(faces)
	if err != nil {
		return err
	}
	mesh.Schema = SelectedMeshSchema
	mesh.Selection = &selection
	return nil
}
