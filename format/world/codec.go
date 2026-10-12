package world

//go:generate go run ../../internal/codegen/binary ../..

import (
	"errors"
	"fmt"

	"github.com/karty-game/karty-sdk/internal/binwire"
)

const BinaryMagic = "KWR1"
const SolidBinaryMagic = "KWR2"

// Encode validates and encodes one compiled world as canonical typed binary.
func Encode(document Document) ([]byte, error) {
	if err := Validate(&document); err != nil {
		return nil, err
	}
	return encodeBinary(document)
}

func encodeBinary(document Document) ([]byte, error) {
	magic := BinaryMagic
	if document.SolidRenderers != nil {
		magic = SolidBinaryMagic
	}
	w := binwire.NewWriter(magic, MaxEncodedSize)
	writeWorldDocument(w, &document)
	if document.SolidRenderers != nil {
		writeWorldSolidRenderers(w, document.SolidRenderers)
	}
	encoded, err := w.Bytes()
	if err != nil {
		return nil, fmt.Errorf("encode world: %w", ErrSize)
	}
	return encoded, nil
}

// Decode rejects JSON, unsupported binary versions, trailing bytes and malformed
// records. A complete allocation-free scan bounds all typed allocations first.
func Decode(encoded []byte) (Document, error) {
	if len(encoded) == 0 || len(encoded) > MaxEncodedSize {
		return Document{}, ErrSize
	}
	magic := BinaryMagic
	if len(encoded) >= 4 && string(encoded[:4]) == SolidBinaryMagic {
		magic = SolidBinaryMagic
	}
	scan := binwire.NewReader(encoded, magic, MaxEncodedSize)
	scanWorldDocument(scan)
	if magic == SolidBinaryMagic {
		scanWorldSolidRenderers(scan)
	}
	if err := scan.Done(); err != nil {
		return Document{}, decodeError(err)
	}
	reader := binwire.NewReader(encoded, magic, MaxEncodedSize)
	var document Document
	readWorldDocument(reader, &document)
	if magic == SolidBinaryMagic {
		document.SolidRenderers = new(SolidRenderers)
		readWorldSolidRenderers(reader, document.SolidRenderers)
	}
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
