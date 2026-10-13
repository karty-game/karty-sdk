package worldsky

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
)

func (image Image) inspect() error {
	return image.expand(nil)
}

// Unpack verifies and expands one compiled image before GPU upload. It reuses
// destination capacity; contents are unspecified if stream validation fails.
func (image Image) Unpack(destination []byte) ([]byte, error) {
	if err := image.validateDimensions(); err != nil {
		return nil, err
	}
	size := image.Width * image.Height * 4
	if cap(destination) < size {
		destination = make([]byte, size)
	} else {
		destination = destination[:size]
	}
	if err := image.expand(destination); err != nil {
		return nil, err
	}
	return destination, nil
}

// A nil destination validates using fixed scratch. Multistream(false) and the
// byte reader keep concatenated members and trailing input observable.
func (image Image) expand(destination []byte) error {
	input := bytes.NewReader(image.Packed)
	reader, err := gzip.NewReader(input)
	if err != nil {
		return fmt.Errorf("%w: header: %w", ErrImage, err)
	}
	defer reader.Close()
	reader.Multistream(false)
	size := image.Width * image.Height * 4
	if destination != nil {
		_, err = io.ReadFull(reader, destination)
	} else {
		var scratch [32 * 1024]byte
		remaining := size
		for remaining > 0 && err == nil {
			count := min(remaining, len(scratch))
			_, err = io.ReadFull(reader, scratch[:count])
			remaining -= count
		}
	}
	if err != nil {
		return fmt.Errorf("%w: pixels: %w", ErrImage, err)
	}
	var extra [1]byte
	count, err := io.ReadFull(reader, extra[:])
	if count != 0 {
		return fmt.Errorf("%w: oversized pixels", ErrImage)
	}
	if !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: checksum or trailer: %w", ErrImage, err)
	}
	if input.Len() != 0 {
		return fmt.Errorf("%w: trailing payload", ErrImage)
	}
	return nil
}
