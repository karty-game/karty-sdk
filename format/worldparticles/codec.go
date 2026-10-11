package worldparticles

//go:generate go run ../../internal/codegen/binary ../..

import (
	"errors"
	"fmt"

	"github.com/karty-game/karty-sdk/internal/binwire"
)

// Encode validates and encodes one particle document as canonical typed binary.
func Encode(document Document) ([]byte, error) {
	if err := Validate(&document); err != nil {
		return nil, err
	}
	return encodeBinary(document)
}

func encodeBinary(document Document) ([]byte, error) {
	w := binwire.NewWriter(BinaryMagic, MaxEncodedSize)
	writeWorldparticlesDocument(w, &document)
	encoded, err := w.Bytes()
	if err != nil {
		return nil, fmt.Errorf("encode particles: %w", ErrBounds)
	}
	return encoded, nil
}

// Decode rejects JSON, unsupported binary versions, trailing bytes and malformed
// records. A complete allocation-free scan bounds all typed allocations first.
func Decode(encoded []byte) (Document, error) {
	if len(encoded) == 0 || len(encoded) > MaxEncodedSize {
		return Document{}, ErrBounds
	}
	scan := binwire.NewReader(encoded, BinaryMagic, MaxEncodedSize)
	scanWorldparticlesDocument(scan)
	if err := scan.Done(); err != nil {
		return Document{}, decodeError(err)
	}
	reader := binwire.NewReader(encoded, BinaryMagic, MaxEncodedSize)
	var document Document
	readWorldparticlesDocument(reader, &document)
	if err := reader.Done(); err != nil {
		return Document{}, decodeError(err)
	}
	if err := Validate(&document); err != nil {
		return Document{}, err
	}
	return document, nil
}

func decodeError(err error) error {
	if errors.Is(err, binwire.ErrVersion) {
		return ErrVersion
	}
	if errors.Is(err, binwire.ErrCount) {
		return ErrBounds
	}
	return ErrSyntax
}
