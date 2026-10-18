package worldvegetation

//go:generate go run ../../internal/codegen/binary ../..

import (
	"errors"
	"fmt"

	"github.com/karty-game/karty-sdk/internal/binwire"
)

// Encode validates and encodes one vegetation document as canonical typed binary.
func Encode(document Document) ([]byte, error) {
	if err := Validate(&document); err != nil {
		return nil, err
	}
	return encodeBinary(document)
}

func encodeBinary(document Document) ([]byte, error) {
	magic := BinaryMagicV5
	switch document.Version {
	case Version:
		magic = BinaryMagic
	case GeometryVersion:
		magic = BinaryMagicV2
	case NormalVersion:
		magic = BinaryMagicV3
	case TreeVersion:
		magic = BinaryMagicV4
	}
	w := binwire.NewWriter(magic, MaxEncodedSize)
	switch document.Version {
	case Version:
		legacy := toLegacy(document)
		writeWorldvegetationlegacyDocument(w, &legacy)
	case GeometryVersion:
		legacy := toLegacyGeometry(document)
		writeWorldvegetationlegacyGeometryDocument(w, &legacy)
	default:
		writeWorldvegetationDocument(w, &document)
	}
	encoded, err := w.Bytes()
	if err != nil {
		return nil, fmt.Errorf("encode vegetation: %w", ErrBounds)
	}
	return encoded, nil
}

// Decode preflights the complete version-specific payload before allocating
// typed records. KWV1 and KWV2 retain their original field layouts.
func Decode(encoded []byte) (Document, error) {
	if len(encoded) == 0 || len(encoded) > MaxEncodedSize {
		return Document{}, ErrBounds
	}
	magic := BinaryMagicV5
	version := PalmVersion
	if len(encoded) >= 4 {
		switch string(encoded[:4]) {
		case BinaryMagic:
			magic = BinaryMagic
			version = Version
		case BinaryMagicV2:
			magic = BinaryMagicV2
			version = GeometryVersion
		case BinaryMagicV3:
			magic = BinaryMagicV3
			version = NormalVersion
		case BinaryMagicV4:
			magic = BinaryMagicV4
			version = TreeVersion
		}
	}
	scan := binwire.NewReader(encoded, magic, MaxEncodedSize)
	switch version {
	case Version:
		scanWorldvegetationlegacyDocument(scan)
	case GeometryVersion:
		scanWorldvegetationlegacyGeometryDocument(scan)
	default:
		scanWorldvegetationDocument(scan)
	}
	if err := scan.Done(); err != nil {
		return Document{}, decodeError(err)
	}
	reader := binwire.NewReader(encoded, magic, MaxEncodedSize)
	var document Document
	switch version {
	case Version:
		var old legacyDocument
		readWorldvegetationlegacyDocument(reader, &old)
		document = fromLegacy(old)
	case GeometryVersion:
		var old legacyGeometryDocument
		readWorldvegetationlegacyGeometryDocument(reader, &old)
		document = fromLegacyGeometry(old)
	default:
		readWorldvegetationDocument(reader, &document)
	}
	if document.Version != version {
		return Document{}, ErrVersion
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
