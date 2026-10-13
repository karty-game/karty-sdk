package worldsky

import (
	"bytes"
	"errors"

	"github.com/karty-game/karty-sdk/internal/binwire"
)

// Encode validates every image and emits canonical static typed binary.
func Encode(document Document) ([]byte, error) {
	if err := Validate(&document); err != nil {
		return nil, err
	}
	w := binwire.NewWriter(BinaryMagic, MaxEncodedSize)
	w.U16(document.Version)
	writeSettings(w, document.Settings)
	for _, image := range [4]*Image{document.CloudSparse, document.CloudDense, document.Background, document.Horizon} {
		w.Bool(image != nil)
		if image != nil {
			w.I32(image.Width)
			w.I32(image.Height)
			w.U32(uint32(len(image.Packed)))
			w.Raw(image.Packed)
		}
	}
	encoded, err := w.Bytes()
	if err != nil {
		return nil, ErrBounds
	}
	return encoded, nil
}

// Decode preflights the entire typed record, dimensions, aggregate bounds and
// every gzip member before allocating owned packed image payloads. Decompression
// uses fixed scratch; no expanded image is published or allocated here.
func Decode(encoded []byte) (Document, error) {
	if len(encoded) == 0 || len(encoded) > MaxEncodedSize {
		return Document{}, ErrBounds
	}
	r := binwire.NewReader(encoded, BinaryMagic, MaxEncodedSize)
	document := Document{Version: r.U16(), Settings: readSettings(r)}
	var images [4]Image
	for index, target := range [4]**Image{&document.CloudSparse, &document.CloudDense, &document.Background, &document.Horizon} {
		if !r.Bool() {
			continue
		}
		image := &images[index]
		image.Width, image.Height = r.I32(), r.I32()
		size := r.U32()
		if size > MaxPackedImageBytes {
			return Document{}, ErrBounds
		}
		image.Packed = r.Raw(int(size))
		*target = image
	}
	if err := r.Done(); err != nil {
		if errors.Is(err, binwire.ErrVersion) {
			return Document{}, ErrVersion
		}
		return Document{}, ErrSyntax
	}
	if err := Validate(&document); err != nil {
		return Document{}, err
	}
	for _, image := range [4]*Image{document.CloudSparse, document.CloudDense, document.Background, document.Horizon} {
		if image != nil {
			image.Packed = bytes.Clone(image.Packed)
		}
	}
	return document, nil
}

func writeSettings(w *binwire.Writer, settings Settings) {
	for _, value := range settings.CloudUVScale {
		w.F64(value)
	}
	w.F64(settings.CloudDenseScrollScale)
	for _, value := range settings.CloudDenseOffset {
		w.F64(value)
	}
	for _, value := range settings.CloudParallax {
		w.F64(value)
	}
	w.F64(settings.HorizonUVScale)
	w.F64(settings.HorizonParallax)
	w.F64(settings.HorizonHaze)
	for _, value := range settings.HorizonEmission {
		w.F64(value)
	}
}

func readSettings(r *binwire.Reader) Settings {
	var settings Settings
	for index := range settings.CloudUVScale {
		settings.CloudUVScale[index] = r.F64()
	}
	settings.CloudDenseScrollScale = r.F64()
	for index := range settings.CloudDenseOffset {
		settings.CloudDenseOffset[index] = r.F64()
	}
	for index := range settings.CloudParallax {
		settings.CloudParallax[index] = r.F64()
	}
	settings.HorizonUVScale = r.F64()
	settings.HorizonParallax = r.F64()
	settings.HorizonHaze = r.F64()
	for index := range settings.HorizonEmission {
		settings.HorizonEmission[index] = r.F64()
	}
	return settings
}
