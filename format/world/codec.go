package world

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Encode validates and deterministically encodes one compiled world.
func Encode(document Document) ([]byte, error) {
	if err := Validate(&document); err != nil {
		return nil, err
	}

	encoded, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("encode world: %w", ErrSyntax)
	}
	if len(encoded) == 0 || len(encoded) > MaxEncodedSize {
		return nil, ErrSize
	}

	return encoded, nil
}

// Decode requires the exact canonical representation produced by Encode. This
// rejects duplicate/unknown fields, alternate number spellings and trailing data.
func Decode(encoded []byte) (Document, error) {
	if len(encoded) == 0 || len(encoded) > MaxEncodedSize {
		return Document{}, ErrSize
	}

	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var document Document
	if err := decoder.Decode(&document); err != nil {
		return Document{}, fmt.Errorf("decode world: %w", ErrSyntax)
	}
	if err := requireJSONEnd(decoder); err != nil {
		return Document{}, err
	}
	if err := Validate(&document); err != nil {
		return Document{}, err
	}

	canonical, err := json.Marshal(document)
	if err != nil {
		return Document{}, fmt.Errorf("encode canonical world: %w", ErrSyntax)
	}
	if !bytes.Equal(encoded, canonical) {
		return Document{}, ErrCanonical
	}

	return document, nil
}

func requireJSONEnd(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return ErrSyntax
	}

	return nil
}
